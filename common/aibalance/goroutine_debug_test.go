package aibalance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// TestRealAIBalanceFlow 模拟真实的 aibalance 请求流程
// This test is skipped by default as it requires 10+ seconds to complete
// Run manually with: go test -v -run TestRealAIBalanceFlow -timeout 30s
func TestRealAIBalanceFlow(t *testing.T) {
	t.Skip("Skipping long-running real AI balance flow test. Run manually if needed.")
	// 1. 创建 mock AI provider
	mockProvider := createDetailedMockAIServer(t)
	defer mockProvider.Close()
	t.Logf("Mock AI provider at: %s", mockProvider.Addr)

	// 2. 创建 aibalance 服务器
	cfg := NewServerConfig()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	aibalanceAddr := listener.Addr().String()
	t.Logf("AIBalance server at: %s", aibalanceAddr)

	// 3. 设置数据库中的 provider 配置
	// 注意：这个测试需要数据库支持，如果没有数据库，跳过
	if GetDB() == nil {
		t.Skip("Database not available, skipping real flow test")
	}

	// 清理并创建测试 provider
	testProviderName := "test-goroutine-leak-provider"
	GetDB().Where("model_name = ?", testProviderName).Delete(&schema.AiProvider{})

	provider := &schema.AiProvider{
		ModelName:    testProviderName,
		TypeName:     "openai",
		DomainOrURL:  fmt.Sprintf("http://%s/v1/chat/completions", mockProvider.Addr),
		APIKey:       "test-key",
		NoHTTPS:      true,
		ProviderMode: "chat",
	}
	if err := GetDB().Create(provider).Error; err != nil {
		t.Fatalf("Failed to create test provider: %v", err)
	}
	defer GetDB().Where("model_name = ?", testProviderName).Delete(&schema.AiProvider{})

	// 启动服务器
	serverCtx, serverCancel := withCancelCtx()
	defer serverCancel()

	var serverWg sync.WaitGroup
	serverWg.Add(1)
	go func() {
		defer serverWg.Done()
		for {
			select {
			case <-serverCtx.Done():
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					if serverCtx.Err() != nil {
						return
					}
					continue
				}
				go cfg.Serve(conn)
			}
		}
	}()

	// 4. 记录初始状态
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("\n=== INITIAL STATE ===")
	t.Logf("Goroutines: %d", initialGoroutines)
	printGoroutineSummary(t, "initial")

	// 5. 发送请求
	numRequests := 20
	t.Logf("\n=== SENDING %d REQUESTS ===", numRequests)

	var requestWg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		requestWg.Add(1)
		go func(id int) {
			defer requestWg.Done()
			sendAIBalanceRequest(t, aibalanceAddr, testProviderName, id)
		}(i)
		time.Sleep(100 * time.Millisecond) // 间隔发送
	}

	// 等待请求完成
	done := make(chan struct{})
	go func() {
		requestWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All requests completed")
	case <-time.After(60 * time.Second):
		t.Log("Timeout waiting for requests")
	}

	// 6. 请求完成后立即检查
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	afterRequestsGoroutines := runtime.NumGoroutine()
	t.Logf("\n=== AFTER REQUESTS ===")
	t.Logf("Goroutines: %d (diff: %+d)", afterRequestsGoroutines, afterRequestsGoroutines-initialGoroutines)
	printGoroutineSummary(t, "after_requests")

	// 7. 等待并监控 goroutine 清理 (reduced from 60s to 5s for faster tests)
	t.Logf("\n=== MONITORING GOROUTINE CLEANUP ===")
	for i := 1; i <= 5; i++ {
		time.Sleep(1 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("After %d seconds: %d goroutines (diff from initial: %+d)",
			i, current, current-initialGoroutines)
	}

	// 8. 最终状态
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("\n=== FINAL STATE (after 5s) ===")
	t.Logf("Goroutines: %d (diff from initial: %+d)", finalGoroutines, finalGoroutines-initialGoroutines)
	printGoroutineSummary(t, "final")

	// 9. 分析泄漏
	leaked := finalGoroutines - initialGoroutines
	if leaked > 20 {
		t.Errorf("GOROUTINE LEAK DETECTED! Leaked: %d", leaked)
	}
}

// TestGoroutineTracing 追踪每个 goroutine 的创建
func TestGoroutineTracing(t *testing.T) {
	// 创建 mock server
	mockServer := createDetailedMockAIServer(t)
	defer mockServer.Close()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	t.Logf("Before: %d goroutines", before)

	// 打印当前所有 goroutine
	printAllGoroutines(t, "before_request")

	// 发送单个请求，观察 goroutine 变化
	t.Log("\n=== SENDING SINGLE REQUEST ===")

	// 直接使用 provider 创建 client 并发送请求
	provider := &Provider{
		TypeName:    "openai",
		DomainOrURL: fmt.Sprintf("http://%s/v1/chat/completions", mockServer.Addr),
		APIKey:      "test-key",
		NoHTTPS:     true,
		ModelName:   "test-model",
	}

	var wg sync.WaitGroup
	pr, pw := utils.NewBufPipe(nil)
	rr, rw := utils.NewBufPipe(nil)

	client, err := provider.GetAIClientWithImages(
		nil,
		func(reader io.Reader) {
			defer pw.Close()
			io.Copy(pw, reader)
			log.Warnf("[TRACE] onStream completed")
		},
		func(reader io.Reader) {
			defer rw.Close()
			io.Copy(rw, reader)
			log.Warnf("[TRACE] onReasonStream completed")
		},
		nil, // onToolCall callback - not used in this test
	)
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	// 请求前 goroutine
	afterClient := runtime.NumGoroutine()
	t.Logf("After GetAIClient: %d goroutines (diff: %+d)", afterClient, afterClient-before)
	printAllGoroutines(t, "after_get_client")

	// 发送 Chat 请求
	wg.Add(1)
	chatDone := make(chan struct{})
	go func() {
		defer wg.Done()
		log.Warnf("[TRACE] Starting Chat request")
		result, err := client.Chat("Hello, this is a test")
		if err != nil {
			log.Warnf("[TRACE] Chat error: %v", err)
		} else {
			log.Warnf("[TRACE] Chat completed, result length: %d", len(result))
		}
		close(chatDone)
	}()

	// 监控 goroutine 变化
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(200 * time.Millisecond)
			current := runtime.NumGoroutine()
			log.Warnf("[TRACE] During request (%dms): %d goroutines", (i+1)*200, current)
		}
	}()

	// 消费流
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(io.Discard, pr)
		log.Warnf("[TRACE] Output stream consumed")
	}()
	go func() {
		defer wg.Done()
		io.Copy(io.Discard, rr)
		log.Warnf("[TRACE] Reason stream consumed")
	}()

	// 等待完成
	select {
	case <-chatDone:
		t.Log("Chat completed")
	case <-time.After(30 * time.Second):
		t.Log("Chat timeout")
	}

	wg.Wait()

	// 请求完成后
	time.Sleep(1 * time.Second)
	runtime.GC()
	afterRequest := runtime.NumGoroutine()
	t.Logf("\nAfter request: %d goroutines (diff from before: %+d)", afterRequest, afterRequest-before)
	printAllGoroutines(t, "after_request")

	// 等待清理 (reduced from 30s to 5s for faster tests)
	t.Log("\n=== WAITING FOR CLEANUP ===")
	for i := 1; i <= 5; i++ {
		time.Sleep(1 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("After %ds: %d goroutines (diff: %+d)", i, current, current-before)
	}

	// 最终
	runtime.GC()
	final := runtime.NumGoroutine()
	t.Logf("\nFinal: %d goroutines (diff: %+d)", final, final-before)
	printAllGoroutines(t, "final")

	if final-before > 10 {
		t.Errorf("Goroutine leak: %d leaked", final-before)
	}
}

// Helper functions

func createDetailedMockAIServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		log.Warnf("[MOCK] Received request from %s", r.RemoteAddr)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// 发送几个 SSE 事件
		for i := 0; i < 3; i++ {
			chunk := map[string]interface{}{
				"id":      fmt.Sprintf("chatcmpl-%d", i),
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "test-model",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": fmt.Sprintf("chunk %d ", i),
						},
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}

		// 发送完成标记
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		log.Warnf("[MOCK] Request completed for %s", r.RemoteAddr)
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}

func sendAIBalanceRequest(t *testing.T, serverAddr, modelName string, id int) {
	reqBody := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Hello %d", id)},
		},
		"stream": true,
	}
	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/v1/chat/completions", serverAddr), bytes.NewReader(data))
	if err != nil {
		t.Logf("Request %d: failed to create: %v", id, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Request %d: failed: %v", id, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Request %d: status=%d, body_len=%d", id, resp.StatusCode, len(body))
}

func printGoroutineSummary(t *testing.T, label string) {
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	fullDump := buf.String()

	// 按函数分组统计
	stackMap := make(map[string]int)
	blocks := strings.Split(fullDump, "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || !strings.HasPrefix(block, "goroutine ") {
			continue
		}

		lines := strings.Split(block, "\n")
		var signature string
		for i, line := range lines {
			if i == 0 {
				continue
			}
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "/") {
				continue
			}
			if strings.Contains(line, "(") {
				idx := strings.Index(line, "(")
				if idx > 0 {
					signature = line[:idx]
				} else {
					signature = line
				}
				break
			}
		}
		if signature != "" {
			stackMap[signature]++
		}
	}

	// 打印统计（只显示数量 > 1 或与 aibalance/lowhttp 相关的）
	t.Logf("--- Goroutine Summary [%s] ---", label)
	for sig, count := range stackMap {
		if count > 1 || strings.Contains(sig, "aibalance") || strings.Contains(sig, "aispec") ||
			strings.Contains(sig, "lowhttp") || strings.Contains(sig, "bufpipe") {
			// 简化显示
			shortSig := sig
			if len(shortSig) > 60 {
				shortSig = shortSig[:60] + "..."
			}
			t.Logf("  %3d: %s", count, shortSig)
		}
	}
}

func printAllGoroutines(t *testing.T, label string) {
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	fullDump := buf.String()

	// 只打印与 aibalance/aispec/lowhttp 相关的
	blocks := strings.Split(fullDump, "\n\n")
	t.Logf("--- All Goroutines [%s] (filtered) ---", label)
	count := 0
	for _, block := range blocks {
		if strings.Contains(block, "aibalance") || strings.Contains(block, "aispec") ||
			strings.Contains(block, "lowhttp.(*persistConn)") || strings.Contains(block, "bufpipe") {
			count++
			if count <= 10 { // 只打印前 10 个
				t.Logf("\n%s", block)
			}
		}
	}
	if count > 10 {
		t.Logf("... and %d more related goroutines", count-10)
	}
	t.Logf("Total related goroutines: %d", count)
}

func withCancelCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// TestGoroutineLeakWithSlowAIProvider 模拟 AI provider 响应慢的情况
// This test is skipped by default as it requires 60+ seconds to complete
// Run manually with: go test -v -run TestGoroutineLeakWithSlowAIProvider -timeout 120s
func TestGoroutineLeakWithSlowAIProvider(t *testing.T) {
	t.Skip("Skipping long-running slow provider test (requires 60s+). Run manually if needed.")
	// 创建响应慢的 mock server（模拟真实 AI provider）
	responseDelay := 10 * time.Second
	slowServer := createSlowResponseMockAIServer(t, responseDelay)
	defer slowServer.Close()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 同时发送多个请求到慢服务器
	numRequests := 10
	var wg sync.WaitGroup

	t.Logf("Sending %d requests to slow provider (delay: %v)", numRequests, responseDelay)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			startTime := time.Now()

			provider := &Provider{
				TypeName:    "openai",
				DomainOrURL: fmt.Sprintf("http://%s/v1/chat/completions", slowServer.Addr),
				APIKey:      "test-key",
				NoHTTPS:     true,
				ModelName:   "test-model",
			}

			pr, pw := utils.NewBufPipe(nil)
			rr, rw := utils.NewBufPipe(nil)

			client, err := provider.GetAIClientWithImages(
				nil,
				func(reader io.Reader) {
					defer pw.Close()
					io.Copy(pw, reader)
				},
				func(reader io.Reader) {
					defer rw.Close()
					io.Copy(rw, reader)
				},
				nil, // onToolCall callback
			)
			if err != nil {
				log.Warnf("[REQUEST %d] Failed to get client: %v", id, err)
				return
			}

			// 消费流
			go func() { io.Copy(io.Discard, pr) }()
			go func() { io.Copy(io.Discard, rr) }()

			_, err = client.Chat("Hello")
			duration := time.Since(startTime)

			if err != nil {
				log.Warnf("[REQUEST %d] Failed after %v: %v", id, duration, err)
			} else {
				log.Warnf("[REQUEST %d] Completed after %v", id, duration)
			}
		}(i)
	}

	// 请求进行中时检查 goroutine
	t.Logf("\n=== MONITORING DURING SLOW REQUESTS ===")
	for i := 1; i <= 5; i++ {
		time.Sleep(2 * time.Second)
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*2, current, current-initialGoroutines)
	}

	// 等待所有请求完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All slow requests completed")
	case <-time.After(60 * time.Second):
		t.Log("Timeout waiting for slow requests")
	}

	// 请求完成后检查
	runtime.GC()
	afterRequests := runtime.NumGoroutine()
	t.Logf("\nAfter requests: %d goroutines (diff: %+d)", afterRequests, afterRequests-initialGoroutines)

	// 等待连接池清理
	t.Logf("\n=== WAITING FOR CONNECTION POOL CLEANUP ===")
	for i := 1; i <= 8; i++ {
		time.Sleep(5 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*5, current, current-initialGoroutines)
	}

	runtime.GC()
	final := runtime.NumGoroutine()
	t.Logf("\nFinal: %d goroutines (diff: %+d)", final, final-initialGoroutines)

	if final-initialGoroutines > 15 {
		t.Errorf("Goroutine leak with slow provider! Leaked: %d", final-initialGoroutines)
		printGoroutineSummary(t, "final")
	}
}

// TestGoroutineLeakWithHangingAIProviderWithTimeout 验证设置 timeout 后卡住的请求能正确回收
// This test is skipped by default as it requires 60+ seconds to complete
// Run manually with: go test -v -run TestGoroutineLeakWithHangingAIProviderWithTimeout -timeout 120s
func TestGoroutineLeakWithHangingAIProviderWithTimeout(t *testing.T) {
	t.Skip("Skipping long-running hanging provider timeout test (requires 60s+). Run manually if needed.")
	// 创建完全卡住的 mock server
	hangingServer := createHangingAIServer(t)
	defer hangingServer.Close()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送请求，但使用较短的超时
	numRequests := 5
	var wg sync.WaitGroup

	t.Logf("Sending %d requests to HANGING provider with 10s timeout", numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			startTime := time.Now()
			log.Warnf("[REQUEST %d] Starting at %v", id, startTime)

			serverURL := fmt.Sprintf("http://%s/v1/chat/completions", hangingServer.Addr)
			result, err := aispec.ChatBase(
				serverURL,
				"test-model",
				fmt.Sprintf("Hello %d", id),
				aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
					log.Warnf("[REQUEST %d] PoCOptions called, setting timeout=10s", id)
					return []poc.PocConfigOption{
						poc.WithTimeout(10), // 10 秒超时
						poc.WithConnectTimeout(3),
						poc.WithConnPool(false), // 禁用连接池，简化调试
					}, nil
				}),
				aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
					log.Warnf("[REQUEST %d] StreamHandler started", id)
					io.Copy(io.Discard, reader)
					log.Warnf("[REQUEST %d] StreamHandler finished", id)
				}),
			)
			duration := time.Since(startTime)
			if err != nil {
				log.Warnf("[REQUEST %d] Failed after %v: %v", id, duration, err)
			} else {
				log.Warnf("[REQUEST %d] Completed after %v: %s", id, duration, result)
			}
		}(i)
	}

	// 等待请求完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// 监控 goroutine
	t.Logf("\n=== MONITORING DURING REQUESTS ===")
	for i := 1; i <= 6; i++ {
		time.Sleep(2 * time.Second)
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*2, current, current-initialGoroutines)
	}

	select {
	case <-done:
		t.Log("All requests completed (with timeout)")
	case <-time.After(30 * time.Second):
		t.Log("Outer timeout")
	}

	// 请求超时后监控恢复
	t.Logf("\n=== MONITORING AFTER TIMEOUT ===")
	for i := 1; i <= 8; i++ {
		time.Sleep(5 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*5, current, current-initialGoroutines)
	}

	final := runtime.NumGoroutine()
	t.Logf("\nFinal: %d goroutines (diff: %+d)", final, final-initialGoroutines)
	printGoroutineSummary(t, "final")

	if final-initialGoroutines > 15 {
		t.Errorf("Goroutine leak even with timeout! Leaked: %d", final-initialGoroutines)
	} else {
		t.Logf("SUCCESS: Goroutines recovered after timeout!")
	}
}

// TestGoroutineLeakWithHangingAIProvider 模拟 AI provider 完全卡住的情况
// This test is skipped by default as it requires 60+ seconds to complete
// Run manually with: go test -v -run TestGoroutineLeakWithHangingAIProvider -timeout 120s
func TestGoroutineLeakWithHangingAIProvider(t *testing.T) {
	t.Skip("Skipping long-running hanging provider test (requires 60s+). Run manually if needed.")
	// 创建完全卡住的 mock server
	hangingServer := createHangingAIServer(t)
	defer hangingServer.Close()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送请求到卡住的服务器
	numRequests := 5
	var wg sync.WaitGroup

	t.Logf("Sending %d requests to HANGING provider", numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 使用 context 超时
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			provider := &Provider{
				TypeName:    "openai",
				DomainOrURL: fmt.Sprintf("http://%s/v1/chat/completions", hangingServer.Addr),
				APIKey:      "test-key",
				NoHTTPS:     true,
				ModelName:   "test-model",
			}

			pr, pw := utils.NewBufPipe(nil)
			rr, rw := utils.NewBufPipe(nil)

			client, err := provider.GetAIClientWithImages(
				nil,
				func(reader io.Reader) {
					defer pw.Close()
					io.Copy(pw, reader)
				},
				func(reader io.Reader) {
					defer rw.Close()
					io.Copy(rw, reader)
				},
				nil, // onToolCall callback
			)
			if err != nil {
				log.Warnf("[REQUEST %d] Failed to get client: %v", id, err)
				return
			}

			// 消费流
			go func() { io.Copy(io.Discard, pr) }()
			go func() { io.Copy(io.Discard, rr) }()

			// 在 goroutine 中发起请求，使用 context 超时
			resultCh := make(chan string, 1)
			errCh := make(chan error, 1)
			go func() {
				result, err := client.Chat("Hello")
				if err != nil {
					errCh <- err
				} else {
					resultCh <- result
				}
			}()

			select {
			case result := <-resultCh:
				log.Warnf("[REQUEST %d] Completed: %s", id, result)
			case err := <-errCh:
				log.Warnf("[REQUEST %d] Failed: %v", id, err)
			case <-ctx.Done():
				log.Warnf("[REQUEST %d] Timeout!", id)
			}
		}(i)
	}

	// 等待请求完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All requests done (possibly with timeout)")
	case <-time.After(30 * time.Second):
		t.Log("Outer timeout")
	}

	// 监控 goroutine
	t.Logf("\n=== MONITORING AFTER HANGING REQUESTS ===")
	afterHanging := runtime.NumGoroutine()
	t.Logf("Immediately after: %d goroutines (diff: %+d)", afterHanging, afterHanging-initialGoroutines)

	for i := 1; i <= 10; i++ {
		time.Sleep(5 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*5, current, current-initialGoroutines)

		if current <= initialGoroutines+5 {
			t.Logf("Goroutines recovered!")
			break
		}
	}

	final := runtime.NumGoroutine()
	t.Logf("\nFinal: %d goroutines (diff: %+d)", final, final-initialGoroutines)
	printGoroutineSummary(t, "final")

	// 这个测试预期会有泄漏，因为 SDK 内部的 goroutine 可能无法取消
	if final-initialGoroutines > 20 {
		t.Logf("WARNING: Hanging provider causes goroutine accumulation!")
	}
}

func createSlowResponseMockAIServer(t *testing.T, delay time.Duration) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		log.Warnf("[SLOW_MOCK] Request received, waiting %v before response", delay)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// 延迟发送
		time.Sleep(delay)

		for i := 0; i < 3; i++ {
			chunk := map[string]interface{}{
				"id":     fmt.Sprintf("slow-%d", i),
				"object": "chat.completion.chunk",
				"choices": []map[string]interface{}{
					{"index": 0, "delta": map[string]interface{}{"content": fmt.Sprintf("chunk%d ", i)}},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		log.Warnf("[SLOW_MOCK] Response completed")
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()
	return server
}

func createHangingAIServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		log.Warnf("[HANGING_MOCK] Request received, will hang indefinitely")

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// 发送一个 chunk 表示连接建立
		chunk := map[string]interface{}{
			"id":     "hanging-0",
			"object": "chat.completion.chunk",
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]interface{}{"content": "starting..."}},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// 永远阻塞，直到客户端断开
		<-r.Context().Done()
		log.Warnf("[HANGING_MOCK] Client disconnected")
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()
	return server
}

// TestLowhttpTimeoutDirectly 直接测试 lowhttp 的超时机制
func TestLowhttpTimeoutDirectly(t *testing.T) {
	// 创建卡住的服务器
	hangingServer := createHangingAIServer(t)
	defer hangingServer.Close()

	t.Logf("Hanging server at: %s", hangingServer.Addr)

	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 直接使用 lowhttp 发送请求，设置 5 秒超时
	t.Log("Sending request with 5s timeout...")
	startTime := time.Now()

	reqPacket := []byte(fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n\r\n{\"model\":\"test\"}", hangingServer.Addr))

	parts := strings.Split(hangingServer.Addr, ":")
	port := 80
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(reqPacket),
		lowhttp.WithHost(parts[0]),
		lowhttp.WithPort(port),
		lowhttp.WithTimeout(5),
		lowhttp.WithConnPool(false),
	)

	duration := time.Since(startTime)
	t.Logf("Request completed after %v", duration)

	if duration > 10*time.Second {
		t.Errorf("Timeout did not work! Request took %v", duration)
	} else {
		t.Logf("Timeout worked correctly! Request took %v", duration)
	}

	// 检查 goroutine
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %+d)", finalGoroutines, finalGoroutines-initialGoroutines)
}

// TestAISpecChatBaseTimeout 测试 aispec.ChatBase 的超时行为
// This test is skipped by default as it requires 60+ seconds to complete
// Run manually with: go test -v -run TestAISpecChatBaseTimeout -timeout 120s
func TestAISpecChatBaseTimeout(t *testing.T) {
	t.Skip("Skipping long-running ChatBase timeout test (requires 60s+). Run manually if needed.")
	// 使用标准 HTTP 服务器创建卡住的 AI provider
	hangingServer := createHangingAIServer(t)
	defer hangingServer.Close()

	t.Logf("Hanging AI server at: %s", hangingServer.Addr)

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 使用 aispec.ChatBase 发送请求，设置 5 秒超时
	t.Log("Calling aispec.ChatBase with 5s timeout...")
	startTime := time.Now()

	serverURL := fmt.Sprintf("http://%s/v1/chat/completions", hangingServer.Addr)

	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	streamDone := make(chan struct{})

	go func() {
		result, err := aispec.ChatBase(
			serverURL,
			"test-model",
			"Hello",
			aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
				log.Warnf("[TEST] PoCOptions called, setting timeout=5s")
				return []poc.PocConfigOption{
					poc.WithTimeout(5), // 5 秒超时
					poc.WithConnectTimeout(3),
					poc.WithConnPool(false),
				}, nil
			}),
			aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
				log.Warnf("[TEST] StreamHandler started")
				n, err := io.Copy(io.Discard, reader)
				log.Warnf("[TEST] StreamHandler finished: %d bytes, err: %v", n, err)
				close(streamDone)
			}),
		)
		if err != nil {
			errCh <- err
		} else {
			resultCh <- result
		}
	}()

	// 监控 goroutine 变化
	for i := 1; i <= 5; i++ {
		time.Sleep(2 * time.Second)
		current := runtime.NumGoroutine()
		t.Logf("At %ds: %d goroutines (diff: %+d)", i*2, current, current-initialGoroutines)
	}

	// 等待结果
	var result string
	var err error
	select {
	case result = <-resultCh:
		t.Logf("ChatBase returned result: %s", result)
	case err = <-errCh:
		t.Logf("ChatBase returned error: %v", err)
	case <-time.After(15 * time.Second):
		t.Log("ChatBase did not return within 15s!")
	}

	duration := time.Since(startTime)
	t.Logf("Total time: %v", duration)

	// 等待 stream handler 完成
	select {
	case <-streamDone:
		t.Log("Stream handler completed")
	case <-time.After(5 * time.Second):
		t.Log("Stream handler still running!")
	}

	// 检查 goroutine
	t.Log("Checking goroutine cleanup...")
	for i := 1; i <= 6; i++ {
		time.Sleep(5 * time.Second)
		runtime.GC()
		current := runtime.NumGoroutine()
		t.Logf("After %ds: %d goroutines (diff: %+d)", i*5, current, current-initialGoroutines)
	}

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %+d)", finalGoroutines, finalGoroutines-initialGoroutines)

	if finalGoroutines-initialGoroutines > 15 {
		t.Errorf("Goroutine leak! Leaked: %d", finalGoroutines-initialGoroutines)
		printGoroutineSummary(t, "final")
	} else {
		t.Log("SUCCESS: Goroutines cleaned up properly!")
	}
}

// TestLowhttpTimeoutWithBodyStreamHandler 测试带 BodyStreamHandler 的超时
func TestLowhttpTimeoutWithBodyStreamHandler(t *testing.T) {
	// 创建卡住的服务器
	hangingServer := createHangingAIServer(t)
	defer hangingServer.Close()

	t.Logf("Hanging server at: %s", hangingServer.Addr)

	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 直接使用 lowhttp 发送请求，设置 5 秒超时，并使用 BodyStreamReaderHandler
	t.Log("Sending request with 5s timeout and BodyStreamReaderHandler...")
	startTime := time.Now()

	reqPacket := []byte(fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\n\r\n{\"model\":\"test\"}", hangingServer.Addr))

	parts := strings.Split(hangingServer.Addr, ":")
	port := 80
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	streamHandlerCalled := false
	streamHandlerDone := make(chan struct{})

	lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(reqPacket),
		lowhttp.WithHost(parts[0]),
		lowhttp.WithPort(port),
		lowhttp.WithTimeout(5),
		lowhttp.WithConnPool(false),
		lowhttp.WithBodyStreamReaderHandler(func(header []byte, body io.ReadCloser) {
			streamHandlerCalled = true
			log.Warnf("[STREAM_HANDLER] Called, header length: %d", len(header))
			log.Warnf("[STREAM_HANDLER] Reading body...")
			n, err := io.Copy(io.Discard, body)
			log.Warnf("[STREAM_HANDLER] Body read: %d bytes, err: %v", n, err)
			close(streamHandlerDone)
		}),
	)

	duration := time.Since(startTime)
	t.Logf("Request completed after %v", duration)
	t.Logf("Stream handler called: %v", streamHandlerCalled)

	// 等待 stream handler 完成
	select {
	case <-streamHandlerDone:
		t.Log("Stream handler completed")
	case <-time.After(10 * time.Second):
		t.Log("Stream handler timeout")
	}

	if duration > 10*time.Second {
		t.Errorf("Timeout did not work with BodyStreamHandler! Request took %v", duration)
	} else {
		t.Logf("Timeout worked correctly! Request took %v", duration)
	}

	// 检查 goroutine
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %+d)", finalGoroutines, finalGoroutines-initialGoroutines)
}
