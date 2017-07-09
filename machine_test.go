package machine

import (
	"io"
	"os"
	"time"
	"bytes"
	"testing"
	"net/http"
	"net/http/httptest"

	"github.com/gorilla/websocket"
	"github.com/aglyzov/log15"
)

// --- utils ---

type Echoer struct {
	test	*testing.T
}

var upgrader = websocket.Upgrader{} // use default options

func NewEchoer(t *testing.T) *Echoer {
	return & Echoer {
		test   : t,
	}
}

func (e *Echoer) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(ans, req, nil)
	if err != nil {
		e.test.Errorf("HTTP upgrade error: %v", err)
		return
	}
	defer ws.Close()
	for {
		mt, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				e.test.Logf("Echoer: ReadMessage error: %v", err)
			}
			return
		}
		if bytes.Equal(p, []byte("/CLOSE")) {
			e.test.Logf("Echoer: closing connection by client request")
			return
		}
		if err := ws.WriteMessage(mt, p); err != nil {
			e.test.Logf("Echoer: WriteMessage error: %v", err)
			return
		}
	}
}

func http_to_ws(u string) string {
	return "ws" + u[len("http"):]
}

// --- setup ---
func TestMain(m *testing.M) {
	var LogHandler = log15.StreamHandler(os.Stderr, log15.TerminalFormat())

	if _, ok := os.LookupEnv("DEBUG"); ! ok {
		LogHandler = log15.LvlFilterHandler(log15.LvlInfo, LogHandler)
	}

	Log.SetHandler(LogHandler)

	os.Exit(m.Run())
}

// --- test cases ---

func TestBadURL(t *testing.T) {
	ws := New("ws://websocket.bad.url/", http.Header{})

	st := <-ws.Status
	if st.State != CONNECTING {
		t.Errorf("st.State is %v, expected CONNECTING (%v). st.Error is %v", st.State, CONNECTING, st.Error)
	}
	st = <-ws.Status
	if st.State != DISCONNECTED {
		t.Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, DISCONNECTED, st.Error)
	}
	st = <-ws.Status
	if st.State != WAITING {
		t.Errorf("st.State is %v, expected WAITING (%v). st.Error is %v", st.State, WAITING, st.Error)
	}

	ws.Command <- QUIT

	st = <-ws.Status
	if st.State != DISCONNECTED {
		t.Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, DISCONNECTED, st.Error)
	}
}
func TestConnect(t *testing.T) {
	srv := httptest.NewServer(NewEchoer(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	st := <-ws.Status
	if st.State != CONNECTING {
		t.Errorf("st.State is %v, expected CONNECTING (%v). st.Error is %v", st.State, CONNECTING, st.Error)
	}
	st = <-ws.Status
	if st.State != CONNECTED {
		t.Errorf("st.State is %v, expected CONNECTED (%v). st.Error is %v", st.State, CONNECTED, st.Error)
	}

	// cleanup
	ws.Command <- QUIT

	st = <-ws.Status
	if st.State != DISCONNECTED {
		t.Errorf("st.State is %v, expected DISCONNECTED (%v). st.Error is %v", st.State, DISCONNECTED, st.Error)
	}

	srv.Close()
}
func TestReconnect(t *testing.T) {
	srv := httptest.NewServer(NewEchoer(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	if st := <-ws.Status; st.State != CONNECTING {
		t.Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-ws.Status; st.State != CONNECTED {
		t.Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// server unexpectedly closes our connection
	ws.Output <- []byte("/CLOSE")

	// wait for a re-connection
	for _, state := range []State{DISCONNECTED, CONNECTING, CONNECTED} {
		select {
		case st := <-ws.Status:
			t.Log(state)
			if st.State != state {
				t.Errorf("st.State is %v, expected %v. st.Error is %v", st.State, state, st.Error)
			}
		case <-time.After(200*time.Millisecond):
			t.Errorf("WSChan has not changed state in time, expected %v", state)
		}
	}

	// cleanup
	ws.Command <- QUIT
	<-ws.Status  // DISCONNECTED
}
func TestServerDisappear(t *testing.T) {
	srv := httptest.NewServer(NewEchoer(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	if st := <-ws.Status; st.State != CONNECTING {
		t.Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-ws.Status; st.State != CONNECTED {
		t.Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// server unexpectedly disappear
	ws.Output <- []byte("/CLOSE")
	srv.Listener.Close()
	srv.Close()

	// expect a re-connection attempt and then waiting
	for _, state := range []State{DISCONNECTED, CONNECTING, DISCONNECTED, WAITING} {
		select {
		case st := <-ws.Status:
			t.Log(state)
			if st.State != state {
				t.Errorf("st.State is %v, expected %v. st.Error is %v", st.State, state, st.Error)
			}
		case <-time.After(200*time.Millisecond):
			t.Errorf("WSChan has not changed state in time, expected %v", state)
		}
	}

	// cleanup
	ws.Command <- QUIT
	<-ws.Status  // DISCONNECTED
}
func TestEcho(t *testing.T) {
	srv := httptest.NewServer(NewEchoer(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	// send a message right away
	orig := []byte("Test Message")
	ws.Output <- orig

	if st := <-ws.Status; st.State != CONNECTING {
		t.Errorf("st.State is %v, extected CONNECTING", st.State)
	}
	if st := <-ws.Status; st.State != CONNECTED {
		t.Errorf("st.State is %v, extected CONNECTED", st.State)
	}

	// wait for an answer
	select {
	case msg, ok := <-ws.Input:
		if !ok {
			t.Error("ws.Input unexpectedly closed")
		} else if !bytes.Equal(msg, orig) {
			t.Errorf("echo message mismatch: %v != %v", msg, orig)
		}
	case <-time.After(100*time.Millisecond):
		t.Error("Timeout when waiting for an echo message")
	}

	// cleanup
	ws.Command <- QUIT

	<-ws.Status  // DISCONNECTED
	srv.Close()
}
