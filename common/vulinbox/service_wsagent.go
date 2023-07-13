package vulinbox

import (
	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"time"
)

func (r *VulinServer) registerWSAgent() {
	router := r.router
	var agentFeedbackHandler func([]byte) error
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			reqRaw, _ := utils.HttpDumpWithBody(request, true)
			if len(reqRaw) > 0 {
				if agentFeedbackHandler != nil {
					err := agentFeedbackHandler(reqRaw)
					if err != nil {
						agentFeedbackHandler = nil
					}
				}
			}
			handler.ServeHTTP(writer, request)
		})
	})
	// wsAgent
	var upgrader = websocket.Upgrader{}
	router.HandleFunc("/_/ws", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `text/html`)
		unsafeTemplateRender(writer, request, `
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


`, map[string]any{
			"host": request.Host,
		})
	})
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
		if agentFeedbackHandler != nil {
			writer.Write([]byte(`agent is connected by other user`))
			writer.WriteHeader(502)
			return
		}

		responseHeader := make(http.Header)
		conn, err := upgrader.Upgrade(writer, request, responseHeader)
		if err != nil {
			return
		}
		agentFeedbackHandler = func(bytes []byte) error {
			err := conn.WriteJSON(map[string]any{
				"type":              "request",
				"request":           string(bytes),
				"timestamp":         time.Now().Second(),
				"timestamp_verbose": time.Now().String(),
			})
			if err != nil {
				conn.Close()
				return err
			}
			return nil
		}
		for {
			conn.WriteJSON(map[string]any{"type": "ping"})
			time.Sleep(time.Second)
		}
	})
}
