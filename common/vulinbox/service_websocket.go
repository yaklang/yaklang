package vulinbox

import (
	_ "embed"
	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"time"
)

//go:embed ws_jquery.min.js
var jquery214 []byte

//go:embed html/ws_websocket_index.html
var wsIndexHtml []byte

func (s *VulinServer) registerWebsocket() {
	r := s.router
	wsGroup := r.Name("Websocket 仿真测试").Subrouter()
	var upgrader = websocket.Upgrader{}
	wsHandlerFactory := func(compress int) func(writer http.ResponseWriter, request *http.Request) {
		return func(writer http.ResponseWriter, request *http.Request) {
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

			go func() {
				for {
					_, message, err := ws.ReadMessage()
					if err != nil {
						log.Errorf("websocket read message failed: %s", err)
						return
					}
					log.Infof("websocket recv message: %s", message)
				}
			}()

			for {
				err = ws.WriteMessage(websocket.TextMessage, []byte("hello yaklang.io / yaklang.com, time now: "+time.Now().String()))
				if err != nil {
					log.Errorf("websocket write message failed: %s", err)
					return
				}
				time.Sleep(time.Second)
			}
		}
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
	}
	for _, v := range wsRoutes {
		addRouteWithVulInfo(wsGroup, v)
	}
}
