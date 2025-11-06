package yaklib

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func TestWebSocketRouteHandler(t *testing.T) {
	// 获取随机端口
	port := utils.GetRandomAvailableTCPPort()
	host := "127.0.0.1"

	// 创建上下文用于控制服务器生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 HTTP 服务器，带有 WebSocket 路由
	go func() {
		err := _httpServe(host, port,
			_httpServerOptContext(ctx),
			_httpServerOptWsRouteHandler("/ws", func(conn *websocket.Conn) {
				defer conn.Close()
				log.Info("WebSocket connection established")

				for {
					// 读取消息
					messageType, message, err := conn.ReadMessage()
					if err != nil {
						log.Errorf("read message error: %s", err)
						break
					}

					log.Infof("received message: %s", string(message))

					// 回显消息
					reply := fmt.Sprintf("Server echo: %s", string(message))
					err = conn.WriteMessage(messageType, []byte(reply))
					if err != nil {
						log.Errorf("write message error: %s", err)
						break
					}
				}
			}),
		)
		if err != nil {
			log.Errorf("http serve error: %s", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(time.Second)

	// 测试 WebSocket 连接
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, port), Path: "/ws"}
	log.Infof("connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial error: %s", err)
	}
	defer conn.Close()

	// 发送测试消息
	testMessage := "Hello WebSocket"
	err = conn.WriteMessage(websocket.TextMessage, []byte(testMessage))
	if err != nil {
		t.Fatalf("write message error: %s", err)
	}

	// 读取回复
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message error: %s", err)
	}

	expectedReply := fmt.Sprintf("Server echo: %s", testMessage)
	if string(message) != expectedReply {
		t.Fatalf("expected reply '%s', got '%s'", expectedReply, string(message))
	}

	log.Infof("test passed, received: %s", string(message))
}

func TestWebSocketMultipleRoutes(t *testing.T) {
	// 获取随机端口
	port := utils.GetRandomAvailableTCPPort()
	host := "127.0.0.1"

	// 创建上下文用于控制服务器生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 HTTP 服务器，带有多个 WebSocket 路由
	go func() {
		err := _httpServe(host, port,
			_httpServerOptContext(ctx),
			// 第一个 WebSocket 路由：echo
			_httpServerOptWsRouteHandler("/ws/echo", func(conn *websocket.Conn) {
				defer conn.Close()
				for {
					messageType, message, err := conn.ReadMessage()
					if err != nil {
						break
					}
					conn.WriteMessage(messageType, message)
				}
			}),
			// 第二个 WebSocket 路由：uppercase
			_httpServerOptWsRouteHandler("/ws/upper", func(conn *websocket.Conn) {
				defer conn.Close()
				for {
					messageType, message, err := conn.ReadMessage()
					if err != nil {
						break
					}
					conn.WriteMessage(messageType, []byte(fmt.Sprintf("UPPER: %s", string(message))))
				}
			}),
		)
		if err != nil {
			log.Errorf("http serve error: %s", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(time.Second)

	// 测试第一个路由
	t.Run("echo route", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, port), Path: "/ws/echo"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("dial error: %s", err)
		}
		defer conn.Close()

		testMessage := "test message"
		conn.WriteMessage(websocket.TextMessage, []byte(testMessage))

		_, message, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read error: %s", err)
		}

		if string(message) != testMessage {
			t.Fatalf("expected '%s', got '%s'", testMessage, string(message))
		}
	})

	// 测试第二个路由
	t.Run("upper route", func(t *testing.T) {
		u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, port), Path: "/ws/upper"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("dial error: %s", err)
		}
		defer conn.Close()

		testMessage := "test message"
		conn.WriteMessage(websocket.TextMessage, []byte(testMessage))

		_, message, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read error: %s", err)
		}

		expected := fmt.Sprintf("UPPER: %s", testMessage)
		if string(message) != expected {
			t.Fatalf("expected '%s', got '%s'", expected, string(message))
		}
	})
}

func TestWebSocketGlobPattern(t *testing.T) {
	// 获取随机端口
	port := utils.GetRandomAvailableTCPPort()
	host := "127.0.0.1"

	// 创建上下文用于控制服务器生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 HTTP 服务器，使用 glob 模式
	go func() {
		err := _httpServe(host, port,
			_httpServerOptContext(ctx),
			_httpServerOptWsRouteHandler("/ws/*", func(conn *websocket.Conn) {
				defer conn.Close()
				for {
					messageType, message, err := conn.ReadMessage()
					if err != nil {
						break
					}
					reply := fmt.Sprintf("Glob matched: %s", string(message))
					conn.WriteMessage(messageType, []byte(reply))
				}
			}),
		)
		if err != nil {
			log.Errorf("http serve error: %s", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(time.Second)

	// 测试不同的路径都能匹配
	testPaths := []string{"/ws/test1", "/ws/test2", "/ws/abc"}
	for _, path := range testPaths {
		t.Run(path, func(t *testing.T) {
			u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, port), Path: path}
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				t.Fatalf("dial error: %s", err)
			}
			defer conn.Close()

			testMessage := "hello"
			conn.WriteMessage(websocket.TextMessage, []byte(testMessage))

			_, message, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read error: %s", err)
			}

			expected := fmt.Sprintf("Glob matched: %s", testMessage)
			if string(message) != expected {
				t.Fatalf("expected '%s', got '%s'", expected, string(message))
			}
		})
	}
}
