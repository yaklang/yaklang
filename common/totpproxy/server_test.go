package totpproxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// 测试用的 TOTP 密钥
const testTOTPSecret = "test-totpproxy-secret"

// TestServerConfig_Validate 测试配置验证
func TestServerConfig_Validate(t *testing.T) {
	t.Run("MissingListenAddr", func(t *testing.T) {
		server := NewServer(
			WithTargetAddr("127.0.0.1:8080"),
			WithTOTPSecret("secret"),
		)
		err := server.Start()
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrMissingListenAddr) || strings.Contains(err.Error(), "listenAddr"))
		t.Logf("Expected error: %v", err)
	})

	t.Run("MissingTargetAddr", func(t *testing.T) {
		server := NewServer(
			WithListenAddr("127.0.0.1:0"),
			WithTOTPSecret("secret"),
		)
		err := server.Start()
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrMissingTargetAddr) || strings.Contains(err.Error(), "targetAddr"))
		t.Logf("Expected error: %v", err)
	})

	t.Run("MissingTOTPSecret", func(t *testing.T) {
		server := NewServer(
			WithListenAddr("127.0.0.1:0"),
			WithTargetAddr("127.0.0.1:8080"),
		)
		err := server.Start()
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrMissingTOTPSecret) || strings.Contains(err.Error(), "totpSecret"))
		t.Logf("Expected error: %v", err)
	})

	t.Run("AllConfigProvided", func(t *testing.T) {
		config := NewDefaultServerConfig()
		config.ListenAddr = "127.0.0.1:0"
		config.TargetAddr = "127.0.0.1:8080"
		config.TOTPSecret = "secret"
		err := config.Validate()
		require.NoError(t, err)
	})
}

// mockBackend 模拟后端服务
func mockBackend(t *testing.T, listenAddr string) (net.Listener, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","message":"hello from backend"}`))
	})

	mux.HandleFunc("/worker_get_status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"model_names":["test-model"],"speed":1,"queue_length":0}`))
	})

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	go func() {
		http.Serve(listener, mux)
	}()

	t.Logf("Mock backend started at %s", listenAddr)
	return listener, nil
}

// mockSSEBackend 模拟 SSE 流式响应后端
func mockSSEBackend(t *testing.T, listenAddr string) (net.Listener, error) {
	mux := http.NewServeMux()

	// SSE 流式响应端点
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		// 检查是否请求流式响应
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		// 模拟 OpenAI 风格的流式响应
		events := []string{
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":" "}}]}`,
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"World"}}]}`,
			`data: {"id":"chatcmpl-1","choices":[{"delta":{"content":"!"}}]}`,
			`data: [DONE]`,
		}

		for _, event := range events {
			fmt.Fprintf(w, "%s\n\n", event)
			flusher.Flush()
			time.Sleep(100 * time.Millisecond) // 模拟延迟
		}
	})

	// 普通 JSON 响应
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	})

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	go func() {
		http.Serve(listener, mux)
	}()

	t.Logf("Mock SSE backend started at %s", listenAddr)
	return listener, nil
}

// mockChunkedBackend 模拟 chunked 分块传输后端
func mockChunkedBackend(t *testing.T, listenAddr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()

				// 读取请求
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}

				// 发送 chunked 响应
				response := "HTTP/1.1 200 OK\r\n" +
					"Content-Type: application/json\r\n" +
					"Transfer-Encoding: chunked\r\n" +
					"Connection: keep-alive\r\n\r\n"
				c.Write([]byte(response))

				// 分块发送数据
				chunks := []string{
					`{"chunk":1}`,
					`{"chunk":2}`,
					`{"chunk":3}`,
				}

				for _, chunk := range chunks {
					// chunked 格式: 长度(十六进制)\r\n数据\r\n
					fmt.Fprintf(c, "%x\r\n%s\r\n", len(chunk), chunk)
					time.Sleep(50 * time.Millisecond)
				}

				// 结束标记
				c.Write([]byte("0\r\n\r\n"))
			}(conn)
		}
	}()

	t.Logf("Mock chunked backend started at %s", listenAddr)
	return listener, nil
}

func TestServer_BasicProxy(t *testing.T) {
	// 启动模拟后端
	backendListener, err := mockBackend(t, "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	// 启动代理服务器 - 必须显式设置 TOTPSecret
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret), // 必须显式设置
		WithDebug(true),
		WithAllowedPaths([]string{"/api/", "/worker_"}),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)
	require.True(t, server.IsRunning())

	t.Run("RejectWithoutTOTP", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/test", proxyAddr), nil)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("Without TOTP: status=%d, body=%s", resp.StatusCode, string(body))
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("SuccessWithValidTOTP_UsingTwofaWithTwoFa", func(t *testing.T) {
		// 使用 twofa.WithTwoFa() 进行交叉验证
		rsp, _, err := poc.DoGET(
			fmt.Sprintf("http://%s/api/test", proxyAddr),
			twofa.WithTwoFa(testTOTPSecret), // 使用 twofa 包的方法
			poc.WithTimeout(5),
		)
		require.NoError(t, err)

		t.Logf("With twofa.WithTwoFa: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "200"))
		require.True(t, strings.Contains(string(rsp.RawPacket), "hello from backend"))
	})

	t.Run("SuccessWithValidTOTP_UsingGetTOTPCode", func(t *testing.T) {
		// 使用 totpproxy.GetTOTPCode() 验证
		totpCode := GetTOTPCode(testTOTPSecret)
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/test", proxyAddr), nil)
		req.Header.Set(DefaultTOTPHeader, totpCode)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("With GetTOTPCode: status=%d, body=%s", resp.StatusCode, string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, string(body), "hello from backend")
	})

	t.Run("RejectInvalidTOTP", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/test", proxyAddr), nil)
		req.Header.Set(DefaultTOTPHeader, "123456")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		t.Logf("With invalid TOTP: status=%d", resp.StatusCode)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("RejectNotAllowedPath", func(t *testing.T) {
		// 使用 twofa.WithTwoFa()
		rsp, _, err := poc.DoGET(
			fmt.Sprintf("http://%s/admin/secret", proxyAddr),
			twofa.WithTwoFa(testTOTPSecret),
			poc.WithTimeout(5),
		)
		require.NoError(t, err)

		t.Logf("Not allowed path: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "404"))
	})
}

func TestServer_WithPoc(t *testing.T) {
	// 启动模拟后端
	backendListener, err := mockBackend(t, "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	// 启动代理服务器 - 必须显式设置 TOTPSecret
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret), // 必须显式设置
		WithDebug(true),
		WithAllowedPaths([]string{"/worker_"}),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	t.Run("YakClientWithTOTP_UsingTwofaWithTwoFa", func(t *testing.T) {
		// 使用 twofa.WithTwoFa() 进行交叉验证
		rsp, _, err := poc.DoPOST(
			fmt.Sprintf("http://%s/worker_get_status", proxyAddr),
			twofa.WithTwoFa(testTOTPSecret), // 使用 twofa 包的方法
			poc.WithTimeout(10),
		)
		require.NoError(t, err)

		t.Logf("Yak client with twofa.WithTwoFa: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "200"))
		require.True(t, strings.Contains(string(rsp.RawPacket), "test-model"))
	})

	t.Run("NonYakClientWithoutTOTP", func(t *testing.T) {
		rsp, _, err := poc.DoPOST(
			fmt.Sprintf("http://%s/worker_get_status", proxyAddr),
			poc.WithTimeout(10),
		)
		require.NoError(t, err)

		t.Logf("Non-Yak client response: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "401"))
	})
}

// TestIntegration_OpenAICompatible 集成测试：测试 OpenAI 兼容 API
// FastChat OpenAI API Server: 192.168.1.4:8000
func TestIntegration_OpenAICompatible(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}
	backendAddr := "192.168.1.4:8000"

	// 检查后端是否可达
	conn, err := net.DialTimeout("tcp", backendAddr, 3*time.Second)
	if err != nil {
		t.Skipf("OpenAI API Server %s is not reachable, skipping: %v", backendAddr, err)
		return
	}
	conn.Close()

	// 验证 API 是否正常
	resp, err := http.Get(fmt.Sprintf("http://%s/v1/models", backendAddr))
	if err != nil {
		t.Skipf("OpenAI API not responding: %v", err)
		return
	}
	defer resp.Body.Close()
	modelsBody, _ := io.ReadAll(resp.Body)
	t.Logf("Models: %s", string(modelsBody))

	if !strings.Contains(string(modelsBody), "DeepSeek") {
		t.Skipf("Model not found in response")
		return
	}

	// 启动代理服务器 - 必须显式设置 TOTPSecret
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret), // 必须显式设置
		WithDebug(true),
		WithAllowedPaths([]string{"/v1/"}),
		WithTimeout(300*time.Second),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(200 * time.Millisecond)

	t.Run("GetModels_WithoutTOTP_Rejected", func(t *testing.T) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/v1/models", proxyAddr), nil)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("GetModels_WithTOTP_UsingTwofaWithTwoFa", func(t *testing.T) {
		// 使用 twofa.WithTwoFa() 进行交叉验证
		rsp, _, err := poc.DoGET(
			fmt.Sprintf("http://%s/v1/models", proxyAddr),
			twofa.WithTwoFa(testTOTPSecret), // 使用 twofa 包的方法
			poc.WithTimeout(10),
		)
		require.NoError(t, err)

		t.Logf("Models response with twofa.WithTwoFa: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "200"))
		require.True(t, strings.Contains(string(rsp.RawPacket), "DeepSeek-R1-Distill-Qwen-32B"))
	})

	t.Run("ChatCompletions_WithTOTP_UsingTwofaWithTwoFa", func(t *testing.T) {
		requestBody := `{
			"model": "DeepSeek-R1-Distill-Qwen-32B",
			"messages": [{"role": "user", "content": "1+1等于几？只回答数字"}],
			"max_tokens": 20
		}`

		// 使用 twofa.WithTwoFa() 进行交叉验证
		rsp, _, err := poc.DoPOST(
			fmt.Sprintf("http://%s/v1/chat/completions", proxyAddr),
			poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
			twofa.WithTwoFa(testTOTPSecret), // 使用 twofa 包的方法
			poc.WithReplaceHttpPacketBody([]byte(requestBody), false),
			poc.WithTimeout(120),
		)
		require.NoError(t, err)

		t.Logf("Chat response with twofa.WithTwoFa: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "200"))
		require.True(t, strings.Contains(string(rsp.RawPacket), "chat.completion"))
	})

	t.Run("ChatCompletions_WithoutTOTP_Rejected", func(t *testing.T) {
		requestBody := `{
			"model": "DeepSeek-R1-Distill-Qwen-32B",
			"messages": [{"role": "user", "content": "hello"}],
			"max_tokens": 10
		}`

		rsp, _, err := poc.DoPOST(
			fmt.Sprintf("http://%s/v1/chat/completions", proxyAddr),
			poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
			poc.WithReplaceHttpPacketBody([]byte(requestBody), false),
			poc.WithTimeout(10),
		)
		require.NoError(t, err)

		t.Logf("Rejected response: %s", string(rsp.RawPacket))
		require.True(t, strings.Contains(string(rsp.RawPacket), "401"))
	})
}

// TestServer_SSEStreaming 测试 SSE 流式响应转发
func TestServer_SSEStreaming(t *testing.T) {
	// 启动 SSE 模拟后端
	backendListener, err := mockSSEBackend(t, "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	// 启动代理服务器
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret),
		WithDebug(true),
		WithAllowedPaths([]string{"/v1/"}),
		WithTimeout(30*time.Second),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	t.Run("SSE_StreamingResponse_RealTime", func(t *testing.T) {
		// 建立连接
		conn, err := net.Dial("tcp", proxyAddr)
		require.NoError(t, err)
		defer conn.Close()

		// 发送请求
		totpCode := GetTOTPCode(testTOTPSecret)
		request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/json\r\n"+
			"%s: %s\r\n"+
			"Content-Length: 2\r\n"+
			"\r\n{}",
			proxyAddr, DefaultTOTPHeader, totpCode)

		_, err = conn.Write([]byte(request))
		require.NoError(t, err)

		// 读取响应并验证流式特性
		reader := bufio.NewReader(conn)
		var receivedEvents []string
		var receiveTimes []time.Time
		startTime := time.Now()

		// 读取响应头
		for {
			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			if line == "\r\n" {
				break // 响应头结束
			}
			if strings.HasPrefix(strings.ToLower(line), "content-type:") {
				require.Contains(t, strings.ToLower(line), "text/event-stream")
			}
		}

		// 读取 SSE 事件
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}

			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				receivedEvents = append(receivedEvents, line)
				receiveTimes = append(receiveTimes, time.Now())
				t.Logf("Received event at %v: %s", time.Since(startTime), line)

				if strings.Contains(line, "[DONE]") {
					break
				}
			}
		}

		// 验证收到了所有事件
		require.GreaterOrEqual(t, len(receivedEvents), 4, "Should receive at least 4 events")

		// 验证流式特性：事件之间应该有时间间隔
		if len(receiveTimes) >= 2 {
			for i := 1; i < len(receiveTimes); i++ {
				gap := receiveTimes[i].Sub(receiveTimes[i-1])
				t.Logf("Gap between event %d and %d: %v", i-1, i, gap)
				// 事件之间应该有间隔（后端模拟 50ms 延迟）
				require.Greater(t, gap.Milliseconds(), int64(10), "Events should arrive with delay")
			}
		}

		// 验证内容
		fullContent := strings.Join(receivedEvents, " ")
		require.Contains(t, fullContent, "Hello")
		require.Contains(t, fullContent, "World")
		require.Contains(t, fullContent, "[DONE]")

		t.Logf("SSE streaming test passed: received %d events in %v", len(receivedEvents), time.Since(startTime))
	})

	t.Run("SSE_WithoutTOTP_Rejected", func(t *testing.T) {
		conn, err := net.Dial("tcp", proxyAddr)
		require.NoError(t, err)
		defer conn.Close()

		// 发送请求（无 TOTP）
		request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: 2\r\n"+
			"\r\n{}",
			proxyAddr)

		_, err = conn.Write([]byte(request))
		require.NoError(t, err)

		// 读取响应
		reader := bufio.NewReader(conn)
		statusLine, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Contains(t, statusLine, "401")
	})
}

// TestServer_ChunkedTransfer 测试 chunked 分块传输转发
func TestServer_ChunkedTransfer(t *testing.T) {
	// 启动 chunked 模拟后端
	backendListener, err := mockChunkedBackend(t, "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	// 启动代理服务器
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret),
		WithDebug(true),
		WithTimeout(30*time.Second),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	t.Run("Chunked_StreamingResponse", func(t *testing.T) {
		conn, err := net.Dial("tcp", proxyAddr)
		require.NoError(t, err)
		defer conn.Close()

		// 发送请求
		totpCode := GetTOTPCode(testTOTPSecret)
		request := fmt.Sprintf("GET /chunked HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"%s: %s\r\n"+
			"\r\n",
			proxyAddr, DefaultTOTPHeader, totpCode)

		_, err = conn.Write([]byte(request))
		require.NoError(t, err)

		// 读取完整响应
		var response bytes.Buffer
		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		for {
			n, err := conn.Read(buf)
			if n > 0 {
				response.Write(buf[:n])
			}
			if err != nil {
				break
			}
			// 检查是否收到结束标记
			if bytes.Contains(response.Bytes(), []byte("0\r\n\r\n")) {
				break
			}
		}

		respStr := response.String()
		t.Logf("Chunked response: %s", respStr)

		// 验证响应
		require.Contains(t, respStr, "200 OK")
		require.Contains(t, respStr, "Transfer-Encoding: chunked")
		require.Contains(t, respStr, `{"chunk":1}`)
		require.Contains(t, respStr, `{"chunk":2}`)
		require.Contains(t, respStr, `{"chunk":3}`)
	})
}

// TestServer_StreamingConcurrency 测试流式响应的并发处理
func TestServer_StreamingConcurrency(t *testing.T) {
	// 启动 SSE 模拟后端
	backendListener, err := mockSSEBackend(t, "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	// 启动代理服务器
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret),
		WithDebug(false), // 关闭调试避免日志过多
		WithAllowedPaths([]string{"/v1/"}),
		WithTimeout(30*time.Second),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	// 并发测试
	concurrency := 5
	var wg sync.WaitGroup
	results := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				t.Logf("Client %d: connection failed: %v", id, err)
				results <- false
				return
			}
			defer conn.Close()

			totpCode := GetTOTPCode(testTOTPSecret)
			request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
				"Host: %s\r\n"+
				"Content-Type: application/json\r\n"+
				"%s: %s\r\n"+
				"Content-Length: 2\r\n"+
				"\r\n{}",
				proxyAddr, DefaultTOTPHeader, totpCode)

			_, err = conn.Write([]byte(request))
			if err != nil {
				t.Logf("Client %d: write failed: %v", id, err)
				results <- false
				return
			}

			// 读取响应
			var response bytes.Buffer
			buf := make([]byte, 4096)
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))

			for {
				n, err := conn.Read(buf)
				if n > 0 {
					response.Write(buf[:n])
				}
				if err != nil || bytes.Contains(response.Bytes(), []byte("[DONE]")) {
					break
				}
			}

			respStr := response.String()
			success := strings.Contains(respStr, "200 OK") &&
				strings.Contains(respStr, "Hello") &&
				strings.Contains(respStr, "[DONE]")

			t.Logf("Client %d: success=%v, response length=%d", id, success, len(respStr))
			results <- success
		}(i)
	}

	wg.Wait()
	close(results)

	// 统计结果
	successCount := 0
	for success := range results {
		if success {
			successCount++
		}
	}

	t.Logf("Concurrency test: %d/%d successful", successCount, concurrency)
	require.Equal(t, concurrency, successCount, "All concurrent requests should succeed")
}

// TestServer_LargeStreamingResponse 测试大数据量流式响应
func TestServer_LargeStreamingResponse(t *testing.T) {
	// 创建一个发送大量数据的后端
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	backendAddr := listener.Addr().String()

	// 后端发送大量数据
	totalChunks := 100
	chunkSize := 1024 // 1KB per chunk
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()

				// 读取请求
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}

				// 发送响应头
				response := "HTTP/1.1 200 OK\r\n" +
					"Content-Type: application/octet-stream\r\n" +
					"Transfer-Encoding: chunked\r\n\r\n"
				c.Write([]byte(response))

				// 发送大量分块数据
				chunk := strings.Repeat("X", chunkSize)
				for i := 0; i < totalChunks; i++ {
					fmt.Fprintf(c, "%x\r\n%s\r\n", len(chunk), chunk)
					time.Sleep(10 * time.Millisecond)
				}
				c.Write([]byte("0\r\n\r\n"))
			}(conn)
		}
	}()

	// 启动代理服务器
	proxyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := proxyListener.Addr().String()
	proxyListener.Close()

	server := NewServer(
		WithListenAddr(proxyAddr),
		WithTargetAddr(backendAddr),
		WithTOTPSecret(testTOTPSecret),
		WithDebug(false),
		WithTimeout(60*time.Second),
	)

	require.NoError(t, server.Start())
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	t.Run("LargeStream", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		conn, err := net.Dial("tcp", proxyAddr)
		require.NoError(t, err)
		defer conn.Close()

		totpCode := GetTOTPCode(testTOTPSecret)
		request := fmt.Sprintf("GET /large HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"%s: %s\r\n"+
			"\r\n",
			proxyAddr, DefaultTOTPHeader, totpCode)

		_, err = conn.Write([]byte(request))
		require.NoError(t, err)

		// 读取响应并计算接收的数据量
		var totalReceived int64
		buf := make([]byte, 8192)
		startTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				t.Fatal("Timeout waiting for response")
			default:
			}

			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(buf)
			if n > 0 {
				totalReceived += int64(n)
			}
			if err != nil {
				break
			}
		}

		duration := time.Since(startTime)
		expectedMinSize := int64(totalChunks * chunkSize)

		t.Logf("Large stream: received %d bytes in %v (expected min: %d)", totalReceived, duration, expectedMinSize)

		// 验证收到了足够的数据
		require.Greater(t, totalReceived, expectedMinSize/2, "Should receive substantial data")
	})
}
