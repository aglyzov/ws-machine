WSChan (Go language)
====================

**WSChan** allows you to interact with a client Websocket connection via 4 channels:
**Input**, **Output**, **Status** and **Command**.

Essentially it is a state machine that creates a [gorilla/websocket](http://github.com/gorilla/websocket) connection,
keeps it alive with PING messages, re-connects if necessary and lets you
send/receive messages via non-blocking channels (as opposed to the blocking
ReadMessage/WriteMessage interface).

Thus you can integrate your Websocket IO into a single select loop with timers,
other channels, you name it.

There are four possible states: **CONNECTING**, **CONNECTED**, **DISCONNECTED**
and **WAITING**.
The state machine switches to the WAITING state when a connection attempt fails.
After 34 seconds it will try again switching to the CONNECTING state.

**Input** and **Output** channels transmit **[]byte** sequences.

**Status** channel transmits **Status structs** which consist of two fields: **{State byte, Error error}**

In order to shutdown the state machine one either closes the **Command** channel
or sends **wschan.QUIT** command.

Example
-------
```go
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
```

