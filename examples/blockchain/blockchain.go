package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/ws-machine"
)

func main() {
    wsm := machine.New("ws://ws.blockchain.info/inv", http.Header{})
	wsm.Command <- machine.USE_TEXT  // blockchain.info does not support bin messages

    for {
        select {
        case st := <-wsm.Status:
            fmt.Println("STATE:", st.State)
			if st.Error != nil {
				fmt.Println("ERROR:", st.Error)
			}
            if st.State == machine.CONNECTED {
				fmt.Println("OP: set_tx_mini")
                wsm.Output <- []byte(`{"op":"set_tx_mini"}`)
				fmt.Println("OP: unconfirmed_sub")
                wsm.Output <- []byte(`{"op":"unconfirmed_sub"}`)
				fmt.Println("OP: blocks_sub")
                wsm.Output <- []byte(`{"op":"blocks_sub"}`)
            }
        case msg := <-wsm.Input:
            fmt.Println(string(msg))
        }
    }
}
