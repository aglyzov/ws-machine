package main

import (
    "fmt"
    "net/http"
    "github.com/aglyzov/wschan"
)

func main() {
    URL := "wss://ws.pusherapp.com/app/de504dc5763aeef9ff52?protocol=6&client=js&version=2.1.2&flash=false"
    ws := wschan.New(URL, http.Header{})

    for {
        select {
        case st := <-ws.Status:
            fmt.Println(st)
            if st.State == wschan.CONNECTED {
                ws.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"order_book"}}`)
                ws.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"live_trades"}}`)
                ws.Output <- []byte(`{"event":"pusher:subscribe","data":{"channel":"live_orders"}}`)
            }
        case msg := <-ws.Input:
            fmt.Println(string(msg))
        }
    }
}
