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
