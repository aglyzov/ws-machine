WS-Machine
==========

**WS-Machine** is a finite state machine for client websocket connections for [Go](http://golang.org).
A caller just needs to provide a websocket URL and after that the state machine
takes care of the connection. I.e. it connects to the server, reads/writes
websocket messages, keeps the connection alive with pings, reconnects when the
connection closes, waits when a connection attempt fails, etc.

Moreover it is fully asynchronous, the caller communicates with a machine via
4 Go channels:

- `Input` is used to receive `[]byte` messages from a websocket server;
- `Output` is for sending `[]byte` messages to a websocket server;
- `Status` lets the user know when the machine state changes;
- `Command` allows the user to control the state machine.

Every machine has 4 states:

- `CONNECTING` is when it attempts to connect to a remote server;
- `CONNECTED` means the connection has been established;
- `DISCONNECTED` should be obvious;
- `WAITING` triggers after a failed attempt to connect when the machine makes a pause before the next retry.

Because everything is done via Go channels it is now possible to integrate
multiple websockets with timers, other channes and network connections
in a single `select` loop. Thus avoiding possible dead-locks, complex logic and
making the code simple, readable, clear and easy to modify/support. As they say **Make websockets great again!**

Under the hood the library uses [gorilla/websocket](http://github.com/gorilla/websocket) to handle raw websocket connections.

In order to shutdown a running FSM a user should either close the `Command` channel
or send the `machine.QUIT` command.

By default new FSM use the *binary message format* to communicate with a server. Some
websocket server implementations do not support those. In such cases one might switch the machine
to the *text message format* by sending the `machine.USE_TEXT` command.

Example
-------
```go
// A stateful client for ws://echo.websocket.org
package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/ws-machine"
)

func main() {
	wsm := machine.New("ws://echo.websocket.org", http.Header{})
	fmt.Println("URL:  ", wsm.URL)

	loop:
	for {
		select {
		case st, _ := <-wsm.Status:
			fmt.Println("STATE:", st.State)
			if st.Error != nil {
				fmt.Println(st.Error)
			}
			switch st.State {
			case machine.CONNECTED:
				msg := "test message"
				wsm.Output <- []byte(msg)
				fmt.Println("SENT: ", msg)
			case machine.DISCONNECTED:
				break loop
			}
		case msg, ok := <-wsm.Input:
			if ok {
				fmt.Println("RECV: ", string(msg))
				wsm.Command <- machine.QUIT
			}
		}
	}
}
```

See more [examples](http://github.com/aglyzov/wschan/tree/master/examples).

