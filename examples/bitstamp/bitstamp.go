package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/ws-machine"
)

func main() {
    URL := "wss://ws.pusherapp.com/app/de504dc5763aeef9ff52?protocol=6&client=js&version=2.1.2&flash=false"
    wsm := machine.New(URL, http.Header{})

    for {
        select {
        case st := <-wsm.Status:
            fmt.Println("STATE:", st.State)
			if st.Error != nil {
				fmt.Println("ERROR:", st.Error)
			}
            if st.State == machine.CONNECTED {
				fmt.Println("SUBSCRIBE: live_trades")
                wsm.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"live_trades"}}`)
				//fmt.Println("SUBSCRIBE: live_orders")
                //ws.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"live_orders"}}`)
				//fmt.Println("SUBSCRIBE: order_book")
                //ws.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"order_book"}}`)
            }
        case msg := <-wsm.Input:
            fmt.Println(string(msg))
        }
    }
}
