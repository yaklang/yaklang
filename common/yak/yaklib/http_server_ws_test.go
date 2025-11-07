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
			_httpServerOptWsRouteHandler("/ws", func(conn *WebSocketConn) {
				defer conn.Close()
				log.Info("WebSocket connection established")
				log.Infof("Raw request size: %d bytes", len(conn.GetRawRequest()))

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
	err := utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		t.Fatalf("wait connect error: %s", err)
	}

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
			_httpServerOptWsRouteHandler("/ws/echo", func(conn *WebSocketConn) {
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
			_httpServerOptWsRouteHandler("/ws/upper", func(conn *WebSocketConn) {
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
	err := utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		t.Fatalf("wait connect error: %s", err)
	}

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
			_httpServerOptWsRouteHandler("/ws/*", func(conn *WebSocketConn) {
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
	err := utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		t.Fatalf("wait connect error: %s", err)
	}

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

func TestWebSocketGetRawRequest(t *testing.T) {
	// 获取随机端口
	port := utils.GetRandomAvailableTCPPort()
	host := "127.0.0.1"

	// 创建上下文用于控制服务器生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var capturedRawRequest []byte

	// 启动 HTTP 服务器，测试 GetRawRequest
	go func() {
		err := _httpServe(host, port,
			_httpServerOptContext(ctx),
			_httpServerOptWsRouteHandler("/ws/test", func(conn *WebSocketConn) {
				defer conn.Close()
				// 获取原始请求
				capturedRawRequest = conn.GetRawRequest()
				log.Infof("Captured raw request: %d bytes", len(capturedRawRequest))

				// 发送确认消息
				conn.WriteMessage(websocket.TextMessage, []byte("OK"))
			}),
		)
		if err != nil {
			log.Errorf("http serve error: %s", err)
		}
	}()

	// 等待服务器启动
	err := utils.WaitConnect(utils.HostPort(host, port), 3)
	if err != nil {
		t.Fatalf("wait connect error: %s", err)
	}

	// 测试 WebSocket 连接，添加自定义 header
	customHeader := map[string][]string{
		"X-Custom-Header":  {"test-value"},
		"X-Request-ID":     {"12345"},
		"User-Agent":       {"YakLang-Test-Client/1.0"},
		"X-Test-Timestamp": {fmt.Sprintf("%d", time.Now().Unix())},
	}

	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, port), Path: "/ws/test"}
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(u.String(), customHeader)
	if err != nil {
		t.Fatalf("dial error: %s", err)
	}
	defer conn.Close()

	// 读取确认消息
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message error: %s", err)
	}

	if string(message) != "OK" {
		t.Fatalf("expected 'OK', got '%s'", string(message))
	}

	// 等待一下确保 capturedRawRequest 已经被设置
	err = utils.WaitConnect(utils.HostPort(host, port), 0.1)
	if err == nil {
		// 连接正常，继续
	}

	// 验证原始请求已被捕获
	if len(capturedRawRequest) == 0 {
		t.Fatal("raw request should not be empty")
	}

	// 验证原始请求包含预期的内容
	rawRequestStr := string(capturedRawRequest)
	log.Infof("Raw request content:\n%s", rawRequestStr)

	// 验证请求行
	if !contains(rawRequestStr, "GET /ws/test") {
		t.Fatalf("raw request should contain 'GET /ws/test', got: %s", rawRequestStr)
	}

	// 验证 WebSocket 升级相关的 header
	if !contains(rawRequestStr, "Upgrade: websocket") && !contains(rawRequestStr, "Upgrade: Websocket") {
		t.Fatalf("raw request should contain 'Upgrade: websocket', got: %s", rawRequestStr)
	}

	if !contains(rawRequestStr, "Connection: Upgrade") && !contains(rawRequestStr, "Connection: upgrade") {
		t.Fatalf("raw request should contain 'Connection: Upgrade', got: %s", rawRequestStr)
	}

	// 验证自定义 header (注意：HTTP header 会被规范化，如 X-Request-ID 会变成 X-Request-Id)
	if !contains(rawRequestStr, "X-Custom-Header: test-value") {
		t.Fatalf("raw request should contain custom header 'X-Custom-Header: test-value', got: %s", rawRequestStr)
	}

	// HTTP 规范化后可能是 X-Request-Id 而不是 X-Request-ID
	if !contains(rawRequestStr, "X-Request-Id: 12345") && !contains(rawRequestStr, "X-Request-ID: 12345") {
		t.Fatalf("raw request should contain custom header 'X-Request-Id: 12345', got: %s", rawRequestStr)
	}

	if !contains(rawRequestStr, "User-Agent: YakLang-Test-Client/1.0") {
		t.Fatalf("raw request should contain custom header 'User-Agent: YakLang-Test-Client/1.0', got: %s", rawRequestStr)
	}

	if !contains(rawRequestStr, "X-Test-Timestamp:") {
		t.Fatalf("raw request should contain custom header 'X-Test-Timestamp', got: %s", rawRequestStr)
	}

	log.Infof("test passed, raw request captured successfully (%d bytes) with all custom headers", len(capturedRawRequest))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
