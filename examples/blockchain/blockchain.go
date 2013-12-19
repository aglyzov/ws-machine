package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/wschan"
)

func main() {
    ws := wschan.New("ws://ws.blockchain.info/inv", http.Header{})
	ws.Command <- wschan.USE_TEXT  // blockchain does not support bin messages

    for {
        select {
        case st := <-ws.Status:
            fmt.Println("State:", st)
            if st.State == wschan.CONNECTED {
                ws.Output <- []byte(`{"op":"set_tx_mini"}`)
                ws.Output <- []byte(`{"op":"unconfirmed_sub"}`)
                ws.Output <- []byte(`{"op":"blocks_sub"}`)
            }
        case msg := <-ws.Input:
            fmt.Println(string(msg))
        }
    }
}
