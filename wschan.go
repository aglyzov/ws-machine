package wschan

import (
	"fmt"
	"time"
	"sync"
	"errors"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/aglyzov/log15"
)

var Log = log15.New("pkg", "wschan")

type State   byte
type Command byte

type (
	WSChan struct {
		Input	<-chan []byte
		Output	chan<- []byte
		Status	<-chan Status
		Command	chan<- Command
	}
	Status struct {
		State	State
		Error	error
	}
)

const (
	// states
	DISCONNECTED State = iota
	CONNECTING
	CONNECTED
	WAITING
)
const (
	// commands
	QUIT       Command = 16 + iota
	PING
	USE_TEXT
	USE_BINARY
)

func (s State) String() string {
	switch s {
	case DISCONNECTED:  return "DISCONNECTED"
	case CONNECTING:    return "CONNECTING"
	case CONNECTED:     return "CONNECTED"
	case WAITING:       return "WAITING"
	}
	return fmt.Sprintf("UNKNOWN STATUS %v", s)
}

func (c Command) String() string {
	switch c {
	case QUIT:        return "QUIT"
	case PING:        return "PING"
	case USE_TEXT:    return "USE_TEXT"
	case USE_BINARY:  return "USE_BINARY"
	}
	return fmt.Sprintf("UNKNOWN COMMAND %v", c)
}

func New(url string, headers http.Header) *WSChan {
	inp_ch := make(chan []byte,  8)
	out_ch := make(chan []byte,  8)
	sts_ch := make(chan Status,  2)
	cmd_ch := make(chan Command, 2)

	con_return_ch := make(chan *websocket.Conn, 1)
	con_cancel_ch := make(chan bool,     1)

	r_error_ch    := make(chan error,    1)
	w_error_ch    := make(chan error,    1)
	w_control_ch  := make(chan Command,  1)

	io_event_ch   := make(chan bool,     2)

	var wg sync.WaitGroup

	connect := func() {
		wg.Add(1)
		defer wg.Done()

		Log.Debug("connect has started")

		for {
			sts_ch <- Status{State:CONNECTING}
			dialer := websocket.Dialer{HandshakeTimeout: 5*time.Second}
			conn, _, err := dialer.Dial(url, headers)
			if err == nil {
				conn.SetPongHandler(func(string) error {io_event_ch <- true; return nil})
				con_return_ch <- conn
				sts_ch <- Status{State:CONNECTED}
				return
			} else {
				Log.Debug("connect error", "err", err)
				sts_ch <- Status{DISCONNECTED, err}
			}

			sts_ch <- Status{State:WAITING}
			select {
			case <- time.After(34*time.Second):
			case <- con_cancel_ch:
				sts_ch <- Status{DISCONNECTED, errors.New("cancelled")}
				return
			}
		}
	}
	keep_alive := func() {
		wg.Add(1)
		defer wg.Done()

		Log.Debug("keep_alive has started")

		dur   := 34 * time.Second
		timer := time.NewTimer(dur)
		timer.Stop()

		loop:
		for {
			select {
			case _, ok := <-io_event_ch:
				if ok {
					timer.Reset(dur)
				} else {
					timer.Stop()
					break loop
				}
			case <-timer.C:
				timer.Reset(dur)
				// non-blocking PING request
				select {
				case w_control_ch <- PING:
				default:
				}
			}
		}
	}
	read := func(conn *websocket.Conn) {
		wg.Add(1)
		defer wg.Done()

		Log.Debug("read has started")

		for {
			if _, msg, err := conn.ReadMessage(); err == nil {
				Log.Debug("received message", "msg", string(msg))
				io_event_ch <- true
				inp_ch <- msg
			} else {
				Log.Debug("read error", "err", err)
				r_error_ch <- err
				break
			}
		}
	}
	write := func(conn *websocket.Conn, msg_type int) {
		wg.Add(1)
		defer wg.Done()

		Log.Debug("write has started")

		loop:
		for {
			select {
			case msg, ok := <-out_ch:
				if ok {
					io_event_ch <- true
					if err := conn.SetWriteDeadline(time.Now().Add(3*time.Second)); err != nil {
						w_error_ch <- err
						break loop
					}
					if err := conn.WriteMessage(msg_type, msg); err != nil {
						w_error_ch <- err
						break loop
					}
					conn.SetWriteDeadline(time.Time{})  // reset write deadline
				} else {
					Log.Debug("write error", "err", "out_ch closed")
					w_error_ch <- errors.New("out_ch closed")
					break loop
				}
			case cmd, ok := <-w_control_ch:
				if !ok {
					w_error_ch <- errors.New("w_control_ch closed")
					break loop
				} else {
					switch cmd {
					case QUIT:
						Log.Debug("write received QUIT command")
						w_error_ch <- errors.New("cancelled")
						break loop
					case PING:
						if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(3*time.Second)); err != nil {
							Log.Debug("ping error", "err", err)
							w_error_ch <- errors.New("cancelled")
							break loop
						}
					case USE_TEXT:
						msg_type = websocket.TextMessage
					case USE_BINARY:
						msg_type = websocket.BinaryMessage
					}
				}
			}
		}
	}

	go func() {
		// local state
		var conn *websocket.Conn
		reading	 := false
		writing  := false
		msg_type := websocket.BinaryMessage  // use Binary messages by default

		defer func() {
			Log.Debug("cleanup has started")
			if conn != nil {conn.Close()}  // this also makes reader to exit

			// close local output channels
			close(con_cancel_ch)  // this makes connect    to exit
			close(w_control_ch)   // this makes write      to exit
			close(io_event_ch)    // this makes keep_alive to exit

			// drain input channels
			<-time.After(50*time.Millisecond)  // small pause to let things react

			drain_loop:
			for {
				select {
				case _, ok := <-out_ch:
					if !ok {out_ch = nil}
				case _, ok := <-cmd_ch:
					if !ok {inp_ch = nil}
				case conn, ok := <-con_return_ch:
					if conn != nil {conn.Close()}
					if !ok {con_return_ch = nil}
				case _, ok := <-r_error_ch:
					if !ok {r_error_ch = nil}
				case _, ok := <-w_error_ch:
					if !ok {w_error_ch = nil}
				default:
					break drain_loop
				}
			}

			// wait for all goroutines to stop
			wg.Wait()

			// close output channels
			close(inp_ch)
			close(sts_ch)
		}()

		Log.Debug("main loop has started")

		go connect()
		go keep_alive()

		main_loop:
		for {
			select {
			case conn = <-con_return_ch:
				if conn == nil {
					break main_loop
				}
				Log.Debug("connected", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())
				reading = true
				writing = true
				go read(conn)
				go write(conn, msg_type)

			case err := <-r_error_ch:
				reading = false
				if writing {
					// write goroutine is still active
					Log.Debug("read error -> stopping write")
					w_control_ch <- QUIT  // ask write to exit
					sts_ch <- Status{DISCONNECTED, err}
				} else {
					// both read and write goroutines have exited
					Log.Debug("read error -> starting connect()")
					if conn != nil {
						conn.Close()
						conn = nil
					}
					go connect()
				}
			case err := <-w_error_ch:
				// write goroutine has exited
				writing = false
				if reading {
					// read goroutine is still active
					Log.Debug("write error -> stopping read")
					if conn != nil {
						conn.Close()  // this also makes read to exit
						conn = nil
					}
					sts_ch <- Status{DISCONNECTED, err}
				} else {
					// both read and write goroutines have exited
					Log.Debug("write error -> starting connect()")
					go connect()
				}
			case cmd, ok := <-cmd_ch:
				if ok {
					Log.Debug("received command", "cmd", cmd)
				}
				switch {
				case !ok || cmd == QUIT:
					if reading || writing || conn != nil {sts_ch <- Status{DISCONNECTED, nil}}
					break main_loop  // defer should clean everything up
				case cmd == PING:
					if conn != nil && writing {w_control_ch <- cmd}
				case cmd == USE_TEXT:
					msg_type = websocket.TextMessage
					if writing {w_control_ch <- cmd}
				case cmd == USE_BINARY:
					msg_type = websocket.BinaryMessage
					if writing {w_control_ch <- cmd}
				default:
					panic(fmt.Sprintf("unsupported command: %v", cmd))
				}
			}
		}
	}()

	return &WSChan{inp_ch, out_ch, sts_ch, cmd_ch}
}
