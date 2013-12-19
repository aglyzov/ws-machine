package wschan

import (
	"fmt"
	"time"
	"errors"
	"net/http"
	ws "github.com/gorilla/websocket"
)

type (
	WSChan struct {
		Input	<-chan []byte
		Output	chan<- []byte
		Status	<-chan Status
		Command	chan<- byte
	}
	Status struct {
		State	byte
		Error	error
	}
)

const (
	// states
	DISCONNECTED byte = iota
	CONNECTING
	CONNECTED
	WAITING
)
const (
	// commands
	QUIT       byte = 16 + iota
	PING
	USE_TEXT
	USE_BINARY
)

func New(url string, headers http.Header) *WSChan {
	inp_ch := make(chan []byte, 8)
	out_ch := make(chan []byte, 8)
	sts_ch := make(chan Status, 2)
	cmd_ch := make(chan byte,   2)

	con_return_ch := make(chan *ws.Conn, 1)
	con_cancel_ch := make(chan bool,     1)

	r_error_ch    := make(chan error, 1)
	w_error_ch    := make(chan error, 1)
	w_control_ch  := make(chan byte,  1)

	io_event_ch   := make(chan bool, 2)

	connect := func() {
		for {
			sts_ch <- Status{State:CONNECTING}
			dialer := ws.Dialer{HandshakeTimeout: 5*time.Second}
			conn, _, err := dialer.Dial(url, headers)
			if err == nil {
				conn.SetPongHandler(func(string) error {io_event_ch <- true; return nil})
				con_return_ch <- conn
				sts_ch <- Status{State:CONNECTED}
				return
			} else {
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
	read := func(conn *ws.Conn) {
		for {
			if _, msg, err := conn.ReadMessage(); err == nil {
				io_event_ch <- true
				inp_ch <- msg
			} else {
				r_error_ch <- err
				break
			}
		}
	}
	write := func(conn *ws.Conn, msg_type int) {
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
						w_error_ch <- errors.New("cancelled")
						break loop
					case PING:
						if err := conn.WriteControl(ws.PingMessage, []byte{}, time.Now().Add(3*time.Second)); err != nil {
							w_error_ch <- errors.New("cancelled")
							break loop
						}
					case USE_TEXT:
						msg_type = ws.TextMessage
					case USE_BINARY:
						msg_type = ws.BinaryMessage
					}
				}
			}
		}
	}

	go func() {
		// local state
		var conn *ws.Conn
		reading	 := false
		writing  := false
		msg_type := ws.BinaryMessage  // use Binary messages by default

		defer func() {
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

			// close output channels
			close(inp_ch)
			close(sts_ch)
		}()

		go connect()
		go keep_alive()

		main_loop:
		for {
			select {
			case conn = <-con_return_ch:
				if conn == nil {
					break main_loop
				}
				reading = true; go read(conn)
				writing = true; go write(conn, msg_type)

			case err := <-r_error_ch:
				reading = false
				if writing {
					// write goroutine is still active
					sts_ch <- Status{DISCONNECTED, err}
					w_control_ch <- QUIT  // ask write to exit
				} else {
					// both read and write goroutines have exited
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
					sts_ch <- Status{DISCONNECTED, err}
					if conn != nil {
						conn.Close()  // this also makes read to exit
						conn = nil
					}
				} else {
					// both read and write goroutines have exited
					go connect()
				}
			case cmd, ok := <-cmd_ch:
				switch {
				case !ok || cmd == QUIT:
					if reading || writing || conn != nil {sts_ch <- Status{DISCONNECTED, nil}}
					break main_loop  // defer should clean everything up
				case cmd == PING:
					if conn != nil && writing {w_control_ch <- cmd}
				case cmd == USE_TEXT:
					msg_type = ws.TextMessage
					if writing {w_control_ch <- cmd}
				case cmd == USE_BINARY:
					msg_type = ws.BinaryMessage
					if writing {w_control_ch <- cmd}
				default:
					panic(fmt.Sprintf("unsupported command: %v", cmd))
				}
			}
		}
	}()

	return &WSChan{inp_ch, out_ch, sts_ch, cmd_ch}
}
