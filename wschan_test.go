package wschan

import (
	"io"
	"time"
	"bytes"
	"testing"
	"net/http"
	"net/http/httptest"
	"github.com/gorilla/websocket"
)

// --- utils ---

type echoHandler testing.T
func (t *echoHandler) ServeHTTP(ans http.ResponseWriter, req *http.Request) {
	ws, err := websocket.Upgrade(ans, req, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		t.Logf("bad handshake: %v", err)
		http.Error(ans, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		t.Logf("upgrade error: %v", err)
		return
	}
	defer ws.Close()
	for {
		mt, p, err := ws.ReadMessage()
		if err != nil {
			if err != io.EOF {
				t.Logf("ReadMessage: %v", err)
			}
			return
		}
		if err := ws.WriteMessage(mt, p); err != nil {
			t.Logf("WriteMessage: %v", err)
			return
		}
	}
}

func http_to_ws(u string) string {
	return "ws" + u[len("http"):]
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
	srv := httptest.NewServer((*echoHandler)(t))
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
	srv := httptest.NewServer((*echoHandler)(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	<-ws.Status  // CONNECTING
	<-ws.Status  // CONNECTED

	// server unexpectedly closes connection
	srv.CloseClientConnections()

	// wait for a re-connection
	for _, state := range []byte{DISCONNECTED, CONNECTING, CONNECTED} {
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
	srv.Close()
}
func TestServerDisappear(t *testing.T) {
	srv := httptest.NewServer((*echoHandler)(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	<-ws.Status  // CONNECTING
	<-ws.Status  // CONNECTED

	// server unexpectedly disappear
	srv.CloseClientConnections()
	srv.Close()

	// expect a re-connection attempt and then waiting
	for _, state := range []byte{DISCONNECTED, CONNECTING, DISCONNECTED, WAITING} {
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
	srv := httptest.NewServer((*echoHandler)(t))
	defer srv.Close()

	URL := http_to_ws(srv.URL)
	t.Logf("Server URL: %v", URL)

	ws := New(URL, http.Header{})

	<-ws.Status  // CONNECTING
	<-ws.Status  // CONNECTED

	// send a message
	orig := []byte("Test Message")
	ws.Output <- orig

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
