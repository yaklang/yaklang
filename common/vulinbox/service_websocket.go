package vulinbox

import (
	_ "embed"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed static/js/ws_jquery.min.js
var jquery214 []byte

//go:embed html/ws_websocket_index.html
var wsIndexHtml []byte

const (
	websocketServerFirstMessage = "yak-ws-server-first"
	websocketPingPayload        = "yak-ws-ping"
	websocketPongMessage        = "yak-ws-pong-ok"
)

func websocketEchoLoop(conn *websocket.Conn, decorate bool) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket read message failed: %v", err)
			}
			return
		}
		log.Printf("websocket recv message type=%d size=%d", messageType, len(message))

		if decorate {
			messageType = websocket.TextMessage
			message = []byte(fmt.Sprintf(`Recive websocket message:%s, time:%s`, message, time.Now().String()))
		}
		if err := conn.WriteMessage(messageType, message); err != nil {
			log.Printf("websocket write message failed: %v", err)
			return
		}
	}
}

func websocketScenarioUpgrader(enableCompression bool, subprotocols ...string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: enableCompression,
		Subprotocols:      subprotocols,
	}
}

func (s *VulinServer) registerWebsocket() {
	r := s.router
	wsGroup := r.Name("Websocket 仿真测试").Subrouter()
	wsHandlerFactory := func(compress int) func(writer http.ResponseWriter, request *http.Request) {
		return func(writer http.ResponseWriter, request *http.Request) {
			upgrader := websocketScenarioUpgrader(compress > 0)
			ws, err := upgrader.Upgrade(writer, request, http.Header(map[string][]string{
				"Your-WebsiteDomainHook": {"yaklang.io"},
			}))
			if err != nil {
				log.Errorf("websocket upgrade failed: %s", err)
				return
			}
			defer ws.Close()

			if compress > 0 {
				ws.EnableWriteCompression(true)
				ws.SetCompressionLevel(compress)
			}
			websocketEchoLoop(ws, true)
		}
	}

	deterministicEchoHandler := func(enableCompression bool) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			upgrader := websocketScenarioUpgrader(enableCompression)
			conn, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				log.Errorf("websocket deterministic echo upgrade failed: %s", err)
				return
			}
			defer conn.Close()
			if enableCompression {
				conn.EnableWriteCompression(true)
			}
			websocketEchoLoop(conn, false)
		}
	}

	firstFrameHandler := func(writer http.ResponseWriter, request *http.Request) {
		upgrader := websocketScenarioUpgrader(false)
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Errorf("websocket first-frame upgrade failed: %s", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(websocketServerFirstMessage)); err != nil {
			log.Errorf("websocket first-frame write failed: %s", err)
			return
		}
		websocketEchoLoop(conn, false)
	}

	idleHandler := func(writer http.ResponseWriter, request *http.Request) {
		upgrader := websocketScenarioUpgrader(false)
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Errorf("websocket idle upgrade failed: %s", err)
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}

	pingHandler := func(writer http.ResponseWriter, request *http.Request) {
		upgrader := websocketScenarioUpgrader(false)
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Errorf("websocket ping upgrade failed: %s", err)
			return
		}
		defer conn.Close()

		pong := make(chan struct{}, 1)
		readDone := make(chan struct{})
		conn.SetPongHandler(func(data string) error {
			if data == websocketPingPayload {
				select {
				case pong <- struct{}{}:
				default:
				}
			}
			return nil
		})
		go func() {
			defer close(readDone)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		deadline := time.Now().Add(3 * time.Second)
		if err := conn.WriteControl(websocket.PingMessage, []byte(websocketPingPayload), deadline); err != nil {
			log.Errorf("websocket ping write failed: %s", err)
			return
		}
		select {
		case <-pong:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(websocketPongMessage)); err != nil {
				log.Errorf("websocket pong confirmation write failed: %s", err)
			}
		case <-readDone:
		case <-time.After(3 * time.Second):
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "pong timeout"),
				time.Now().Add(time.Second),
			)
		}
	}

	closeHandler := func(writer http.ResponseWriter, request *http.Request) {
		code := websocket.CloseNormalClosure
		if rawCode := request.URL.Query().Get("code"); rawCode != "" {
			if parsed, err := strconv.Atoi(rawCode); err == nil {
				code = parsed
			}
		}
		reason := request.URL.Query().Get("reason")
		upgrader := websocketScenarioUpgrader(false)
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Errorf("websocket close upgrade failed: %s", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason),
			time.Now().Add(time.Second),
		); err != nil {
			log.Errorf("websocket close frame write failed: %s", err)
		}
	}

	subprotocolHandler := func(writer http.ResponseWriter, request *http.Request) {
		upgrader := websocketScenarioUpgrader(false, "yak-ws-v1", "yak-ws-v2")
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Errorf("websocket subprotocol upgrade failed: %s", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(conn.Subprotocol())); err != nil {
			log.Errorf("websocket subprotocol write failed: %s", err)
			return
		}
		websocketEchoLoop(conn, false)
	}

	delayedHandshakeHandler := func(writer http.ResponseWriter, request *http.Request) {
		delay, _ := strconv.Atoi(request.URL.Query().Get("delay_ms"))
		if delay < 0 {
			delay = 0
		}
		if delay > 5000 {
			delay = 5000
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
		firstFrameHandler(writer, request)
	}

	wsRoutes := []*VulInfo{
		{
			Path:  "/websocket/",
			Title: "Websocket基础案例",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/html")
				writer.Write(wsIndexHtml)
			},
			RiskDetected: true,
		},
		{
			Path: "/websocket/jquery.min.js",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/javascript")
				writer.Write(jquery214)
			},
		},
		{
			Path:    "/websocket/ws",
			Handler: wsHandlerFactory(0),
		},
		{
			Path:    "/websocket/ws/compression",
			Handler: wsHandlerFactory(3),
		},
		{
			Path:    "/websocket/ws/echo",
			Handler: deterministicEchoHandler(false),
		},
		{
			Path:    "/websocket/ws/echo/compression",
			Handler: deterministicEchoHandler(true),
		},
		{
			Path:    "/websocket/ws/first-frame",
			Handler: firstFrameHandler,
		},
		{
			Path:    "/websocket/ws/idle",
			Handler: idleHandler,
		},
		{
			Path:    "/websocket/ws/ping",
			Handler: pingHandler,
		},
		{
			Path:    "/websocket/ws/close",
			Handler: closeHandler,
		},
		{
			Path:    "/websocket/ws/subprotocol",
			Handler: subprotocolHandler,
		},
		{
			Path:    "/websocket/ws/delayed-handshake",
			Handler: delayedHandshakeHandler,
		},
	}
	for _, v := range wsRoutes {
		addRouteWithVulInfo(wsGroup, v)
	}
}
