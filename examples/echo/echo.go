package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/wschan"
)

func main() {
	ws := wschan.New("ws://echo.websocket.org", http.Header{})

	loop:
	for {
		select {
		case st, ok := <-ws.Status:
			if ok && st.State == wschan.CONNECTED {
				fmt.Println("CONNECTED")
				ws.Output <- []byte("test message")
			}
		case msg, ok := <-ws.Input:
			if ok {
				fmt.Println(string(msg))
				ws.Command <- wschan.QUIT
			} else {
				break loop
			}
		}
	}
}
