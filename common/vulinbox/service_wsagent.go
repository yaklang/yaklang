package vulinbox

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinboxagentproto"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"sync"
)

type EvFunc func([]byte) (any, error)

type wsAgent struct {
	conn   *websocket.Conn
	wChan  chan any
	ctx    context.Context
	cancel context.CancelFunc

	events map[string]EvFunc
}

func (a *wsAgent) init(conn *websocket.Conn) {
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.conn = conn
	a.events = make(map[string]EvFunc)
	a.wChan = make(chan any, 1000)
}

func (a *wsAgent) listenLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}
		_, m, err := a.conn.ReadMessage()
		if err != nil {
			a.cancel()
			return
		}
		go a.messageMux(m)
	}
}

func (a *wsAgent) messageMux(data []byte) {
	ap := utils.MustUnmarshalJson[vulinboxagentproto.AgentProtocol](data)
	if ap == nil || ap.Action == "" || a.events[ap.Action] == nil {
		return
	}
	rec, err := a.events[ap.Action](data)
	if err != nil {
		a.TrySend(vulinboxagentproto.NewAckAction(ap.ActionId, "error", err))
		return
	}
	a.TrySend(vulinboxagentproto.NewAckAction(ap.ActionId, "ok", rec))
}

func (a *wsAgent) sendLoop() {
	for v := range a.wChan {
		err := a.conn.WriteJSON(v)
		if err != nil {
			log.Debugf("ws conn from: %v closed", a.conn.LocalAddr())
			a.cancel()
			return
		}
	}
}

func (a *wsAgent) run() {
	go a.listenLoop()
	go a.sendLoop()
	<-a.ctx.Done()
	close(a.wChan)
}

func (a *wsAgent) TrySend(v any) {
	select {
	case a.wChan <- v:
	default:
		// channel closed or full, drop it
	}
}

func (a *wsAgent) Register(action string, handler EvFunc) {
	a.events[action] = handler
}

func (r *VulinServer) registerWSAgent() {
	router := r.router
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

		r.wsAgent.init(conn)
		r.registerWSAgentEvent()
		r.wsAgent.run()
	})
}

func (r *VulinServer) registerWSAgentEvent() {
	r.wsAgent.Register("ping", handlePing)
	r.wsAgent.Register("udp", handleUDP)
	r.wsAgent.Register("subscribe", r.handleSubscribe)
	r.wsAgent.Register("unsubscribe", r.handleUnsubscribe)
}

func (r *VulinServer) handleSubscribe(a []byte) (any, error) {
	subscribe := utils.MustUnmarshalJson[vulinboxagentproto.SubscribeAction](a)
	if subscribe == nil {
		return nil, nil
	}
	if subscribe.Type != "suricata" {
		return nil, nil
	}

	var appendRules []*rule.Rule
	for _, v := range subscribe.Rules {
		rules, err := rule.Parse(v)
		if err != nil {
			return nil, err
		}
		appendRules = append(appendRules, rules...)
	}
	r.matcher.AddRule(appendRules...)
	r.matcher.SetCallback(func(data []byte) {
		r.wsAgent.TrySend(vulinboxagentproto.NewDataBackAction("suricata", codec.EncodeBase64(data)))
	})
	go r.matcher.RunSingle()
	return nil, nil
}

func (r *VulinServer) handleUnsubscribe(a []byte) (any, error) {
	unsubscribe := utils.MustUnmarshalJson[vulinboxagentproto.UnsubscribeAction](a)
	if unsubscribe == nil {
		return nil, nil
	}
	if unsubscribe.Type != "suricata" {
		return nil, nil
	}

	var removeRules []*rule.Rule
	for _, v := range unsubscribe.Rules {
		rules, err := rule.Parse(v)
		if err != nil {
			return nil, err
		}
		removeRules = append(removeRules, rules...)
	}
	r.matcher.RemoveRule(removeRules...)
	return nil, nil
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
