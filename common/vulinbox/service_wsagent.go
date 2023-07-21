package vulinbox

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"sync"
	"time"
)

func (r *VulinServer) registerWSAgent() {
	router := r.router
	var wsAgentWrite = r.agentFeedbackChan
	// wsAgent
	var upgrader = websocket.Upgrader{}
	router.HandleFunc("/_/ws", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `text/html`)
		unsafeTemplateRender(writer, request, wspage, map[string]any{
			"host": request.Host,
		})
	})
	var wsAgentMux = new(sync.Mutex)

	router.HandleFunc("/_/ws/agent", func(writer http.ResponseWriter, request *http.Request) {
		/*
			GET /_/ws/agent HTTP/1.1
			Host: 127.0.0.1:8787
			Connection: Upgrade
			Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
			Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
			Sec-WebSocket-Protocol: pbbp2
			Sec-WebSocket-Version: 13
			Upgrade: websocket
			User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36
		*/

		if !wsAgentMux.TryLock() {
			writer.Write([]byte(`agent is connected by other user`))
			writer.WriteHeader(502)
			return
		} else {
			log.Info("start to enter ws agent lock")
		}

		defer wsAgentMux.Unlock()
		defer func() {
			if err := recover(); err != nil {
				spew.Dump(err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		log.Infof("start to upgrade to ws agent: %s", request.RemoteAddr)
		responseHeader := make(http.Header)
		conn, err := upgrader.Upgrade(writer, request, responseHeader)
		if err != nil {
			log.Error(err)
			return
		}
		defer func() {
			conn.Close()
		}()

		var wr sync.Mutex
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()
			for {
				_, m, err := conn.ReadMessage()
				if err != nil {
					return
				}
				go MessageMux(m, func(ack *AckAction) {
					wsAgentWrite <- ack
				})
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case feedback := <-wsAgentWrite:
				wr.Lock()
				err := conn.WriteJSON(feedback)
				wr.Unlock()
				if err != nil {
					log.Debugf("ws conn from: %v closed", conn.LocalAddr())
					cancel()
					return
				}
			case <-time.After(time.Second):
				log.Debugf("ws conn start to send: %v ping", conn.LocalAddr())
				wr.Lock()
				err := conn.WriteJSON(newPingAction())
				wr.Unlock()
				if err != nil {
					log.Debugf("ws conn from: %v closed", conn.LocalAddr())
					cancel()
					return
				}
				continue
			}
		}
	})
}

const wspage = `
<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	<h1>WebSocket Agent WS CONNECT WITH</h1>
<pre>
GET /_/ws/agent HTTP/1.1
Host: {{ .host }}
Connection: Upgrade
Sec-WebSocket-Key: kpFli2X1YeW53YainWGFzA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: FeedbackStreamer/1.0
</pre>
</div>

</body>
</html>


`
