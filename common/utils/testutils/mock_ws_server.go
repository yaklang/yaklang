package testutils

import (
	"context"
	websocket2 "github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/websocket"
	"net"
	"net/http"
	"time"
)

type handleTCPFunc func(ctx context.Context, lis net.Listener, conn net.Conn)

func DebugMockWs(handler func(conn *websocket.Conn)) (string, int) {
	addr := utils.GetRandomLocalAddr()

	go func() {
		server := &websocket.Server{
			Handler: websocket.Handler(handler),
			Handshake: func(config *websocket.Config, req *http.Request) error {
				// 不执行任何 Origin 检查
				return nil
			},
		}
		if err := http.ListenAndServe(addr, server); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	host, port, _ := utils.ParseStringToHostPort(addr)
	return host, port
}

func DebugMockEchoWs(point string) (string, int) {
	addr := utils.GetRandomLocalAddr()
	time.Sleep(time.Millisecond * 300)
	host, port, _ := utils.ParseStringToHostPort(addr)

	upgrader := websocket2.Upgrader{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		EnableCompression: true, // 启用压缩
	}

	http.HandleFunc("/"+point, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()

		for {
			mt, message, err := conn.ReadMessage()
			if err != nil && message == nil {
				// 检查WebSocket是否正常关闭
				if websocket2.IsCloseError(err, websocket2.CloseNormalClosure, websocket2.CloseGoingAway) {
					log.Infof("Websocket closed normally: %v", err)
				} else {
					log.Errorf("read: %v", err)
				}
				return
			}
			serverMessage := []byte("server: " + string(message))
			if err := conn.WriteMessage(mt, serverMessage); err != nil {
				log.Errorf("write: %v", err)
				return
			}
		}
	})

	server := &http.Server{Addr: addr}

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	err := utils.WaitConnect(addr, 3)
	if err != nil {
		panic(err)
	}

	return host, port
}
