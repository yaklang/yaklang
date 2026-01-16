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
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// TestGoroutineLeakWithMockProvider 使用 mock AI provider 测试 goroutine 泄漏
func TestGoroutineLeakWithMockProvider(t *testing.T) {
	// 创建 mock AI provider 服务器
	mockServer := createMockAIServer(t)
	defer mockServer.Close()

	t.Logf("Mock AI server started at: %s", mockServer.Addr)

	// 创建 aibalance 服务器配置
	cfg := &ServerConfig{}

	// 启动 aibalance 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	t.Logf("AIBalance server started at: %s", serverAddr)

	// 启动服务器 goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var serverWg sync.WaitGroup
	serverWg.Add(1)
	go func() {
		defer serverWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					continue
				}
				go cfg.Serve(conn)
			}
		}
	}()

	// 记录初始 goroutine 数量
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送大量请求
	numRequests := 50
	var requestWg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		requestWg.Add(1)
		go func(id int) {
			defer requestWg.Done()
			sendLeakTestChatRequest(t, serverAddr, id)
		}(i)

		// 稍微间隔一下，避免太快
		time.Sleep(20 * time.Millisecond)
	}

	t.Logf("All %d requests sent, waiting for completion...", numRequests)

	// 等待所有请求完成
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

	// 等待一段时间让 goroutine 清理
	t.Log("Waiting for goroutines to clean up...")
	for i := 0; i < 10; i++ {
		runtime.GC()
		time.Sleep(500 * time.Millisecond)

		currentGoroutines := runtime.NumGoroutine()
		t.Logf("After %d iterations: %d goroutines (initial: %d, diff: %d)",
			i+1, currentGoroutines, initialGoroutines, currentGoroutines-initialGoroutines)

		// 如果 goroutine 数量稳定在接近初始值，说明没有泄漏
		if currentGoroutines <= initialGoroutines+10 {
			t.Logf("Goroutines stabilized at %d (acceptable)", currentGoroutines)
			return
		}
	}

	// 最终检查
	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	if leaked > 20 {
		t.Errorf("GOROUTINE LEAK DETECTED! Initial: %d, Final: %d, Leaked: %d",
			initialGoroutines, finalGoroutines, leaked)

		// 打印 goroutine 堆栈
		printGoroutineStacks(t)
	} else {
		t.Logf("Test passed. Final goroutines: %d (leaked: %d, within acceptable range)",
			finalGoroutines, leaked)
	}
}

// TestGoroutineLeakWithSlowProvider 测试慢速 provider 是否导致泄漏
func TestGoroutineLeakWithSlowProvider(t *testing.T) {
	// 创建慢速 mock AI provider
	slowServer := createSlowMockAIServer(t, 2*time.Second)
	defer slowServer.Close()

	t.Logf("Slow mock AI server started at: %s", slowServer.Addr)

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送请求到慢速服务器
	numRequests := 10
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sendDirectChatRequest(t, slowServer.Addr, id)
		}(i)
	}

	// 等待请求完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All slow requests completed")
	case <-time.After(30 * time.Second):
		t.Log("Timeout waiting for slow requests")
	}

	// 检查泄漏
	time.Sleep(2 * time.Second)
	runtime.GC()
	time.Sleep(500 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	t.Logf("Initial: %d, Final: %d, Leaked: %d", initialGoroutines, finalGoroutines, leaked)

	if leaked > 5 {
		t.Errorf("GOROUTINE LEAK with slow provider! Leaked: %d", leaked)
		printGoroutineStacks(t)
	}
}

// TestGoroutineLeakWithAISDK 直接测试 AI SDK 的 goroutine 泄漏
func TestGoroutineLeakWithAISDK(t *testing.T) {
	// 创建 mock AI provider
	mockServer := createMockAIServer(t)
	defer mockServer.Close()

	t.Logf("Mock AI server started at: %s", mockServer.Addr)

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 直接使用 AI SDK 发送请求
	numRequests := 50
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			serverURL := fmt.Sprintf("http://%s/v1/chat/completions", mockServer.Addr)
			result, err := aispec.ChatBase(
				serverURL,
				"test-model",
				fmt.Sprintf("Hello %d", id),
				aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
					return []poc.PocConfigOption{
						poc.WithTimeout(30),
					}, nil
				}),
				aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
					io.Copy(io.Discard, reader)
				}),
			)
			if err != nil {
				t.Logf("Request %d failed: %v", id, err)
			} else {
				t.Logf("Request %d completed, result length: %d", id, len(result))
			}
		}(i)
		time.Sleep(50 * time.Millisecond)
	}

	// 等待完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All AI SDK requests completed")
	case <-time.After(30 * time.Second):
		t.Log("Timeout waiting for AI SDK requests")
	}

	// 检查泄漏
	t.Log("Waiting for goroutines to clean up...")
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(500 * time.Millisecond)
		currentGoroutines := runtime.NumGoroutine()
		t.Logf("After %d iterations: %d goroutines (initial: %d)", i+1, currentGoroutines, initialGoroutines)
	}

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	t.Logf("Initial: %d, Final: %d, Leaked: %d", initialGoroutines, finalGoroutines, leaked)

	// 打印详细堆栈，无论是否泄漏，帮助诊断
	printDetailedGoroutineStacks(t)

	if leaked > 15 {
		t.Errorf("GOROUTINE LEAK in AI SDK! Leaked: %d", leaked)
	}
}

// TestGoroutineLeakScaling 测试 goroutine 泄漏是否随请求数增长
func TestGoroutineLeakScaling(t *testing.T) {
	mockServer := createMockAIServer(t)
	defer mockServer.Close()

	t.Logf("Mock server at: %s", mockServer.Addr)

	// 测试多轮请求
	for round := 1; round <= 3; round++ {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		beforeGoroutines := runtime.NumGoroutine()

		// 发送 100 个请求
		numRequests := 100
		var wg sync.WaitGroup
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				serverURL := fmt.Sprintf("http://%s/v1/chat/completions", mockServer.Addr)
				_, _ = aispec.ChatBase(
					serverURL,
					"test-model",
					fmt.Sprintf("Round %d Hello %d", round, id),
					aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
						return []poc.PocConfigOption{poc.WithTimeout(30)}, nil
					}),
					aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
						io.Copy(io.Discard, reader)
					}),
				)
			}(i)
		}
		wg.Wait()

		runtime.GC()
		time.Sleep(500 * time.Millisecond)
		afterGoroutines := runtime.NumGoroutine()

		t.Logf("Round %d: Before=%d, After=%d, Diff=%d", round, beforeGoroutines, afterGoroutines, afterGoroutines-beforeGoroutines)
	}

	// 最终检查
	runtime.GC()
	time.Sleep(1 * time.Second)
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	// 注意：lowhttp 连接池有 90 秒 idle timeout，所以 goroutine 会保持一段时间
	// 这不是泄漏，而是连接池的正常行为
	t.Logf("NOTE: lowhttp connection pool has 90s idle timeout, goroutines will be cleaned up after that")
	t.Logf("In production, if AI providers hang, connections stay open and goroutines accumulate")
}

// TestGoroutineLeakWithConnectionPoolTimeout 测试连接池超时后 goroutine 是否被回收
// Note: lowhttp idle timeout has been reduced from 90s to 30s
func TestGoroutineLeakWithConnectionPoolTimeout(t *testing.T) {
	mockServer := createMockAIServer(t)
	defer mockServer.Close()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送一些请求
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			serverURL := fmt.Sprintf("http://%s/v1/chat/completions", mockServer.Addr)
			_, _ = aispec.ChatBase(
				serverURL, "test-model", fmt.Sprintf("Hello %d", id),
				aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
					return []poc.PocConfigOption{poc.WithTimeout(30)}, nil
				}),
				aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
					io.Copy(io.Discard, reader)
				}),
			)
		}(i)
	}
	wg.Wait()

	// 请求完成后检查
	runtime.GC()
	afterRequestsGoroutines := runtime.NumGoroutine()
	t.Logf("After requests: %d goroutines (diff: %d)", afterRequestsGoroutines, afterRequestsGoroutines-initialGoroutines)

	// 等待连接池 idle timeout (30 秒 + buffer)
	t.Logf("Waiting 35 seconds for connection pool idle timeout (reduced from 90s to 30s)...")
	time.Sleep(35 * time.Second)

	runtime.GC()
	time.Sleep(1 * time.Second)
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("After 35s wait: %d goroutines (diff from initial: %d)", finalGoroutines, finalGoroutines-initialGoroutines)

	// 检查泄漏：考虑到一些全局 cache goroutines 可能在首次请求时创建
	// 允许一些额外的 goroutines（全局缓存等）
	if finalGoroutines > initialGoroutines+30 {
		t.Errorf("Goroutine leak after connection pool timeout! Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
		printDetailedGoroutineStacks(t)
	} else {
		t.Logf("Connection pool correctly released goroutines (some global cache goroutines may have been created)")
	}
}

// TestGoroutineLeakWithHangingProvider 测试卡住的 provider 是否导致泄漏
func TestGoroutineLeakWithHangingProvider(t *testing.T) {
	// 创建会卡住的 mock AI provider
	hangingServer := createHangingMockAIServer(t)
	defer hangingServer.Close()

	t.Logf("Hanging mock AI server started at: %s", hangingServer.Addr)

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 发送请求到卡住的服务器（使用超时）
	numRequests := 5
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sendDirectChatRequestWithTimeout(t, hangingServer.Addr, id, 3*time.Second)
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
		t.Log("All hanging requests completed (with timeout)")
	case <-time.After(10 * time.Second):
		t.Log("Outer timeout waiting for hanging requests")
	}

	// 检查泄漏
	t.Log("Waiting for goroutines to clean up after hanging requests...")
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(1 * time.Second)

		currentGoroutines := runtime.NumGoroutine()
		t.Logf("After %d seconds: %d goroutines (initial: %d)", i+1, currentGoroutines, initialGoroutines)
	}

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	t.Logf("Initial: %d, Final: %d, Leaked: %d", initialGoroutines, finalGoroutines, leaked)

	if leaked > 10 {
		t.Errorf("GOROUTINE LEAK with hanging provider! Leaked: %d", leaked)
		printGoroutineStacks(t)
	}
}

// Helper functions

func createMockAIServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		// 模拟流式响应
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
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}

func createSlowMockAIServer(t *testing.T, delay time.Duration) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// 慢速发送
		for i := 0; i < 5; i++ {
			chunk := map[string]interface{}{
				"id":      fmt.Sprintf("chatcmpl-%d", i),
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "slow-model",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": fmt.Sprintf("slow chunk %d ", i),
						},
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			time.Sleep(delay / 5)
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}

func createHangingMockAIServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// 发送一个 chunk 然后挂起
		chunk := map[string]interface{}{
			"id":     "chatcmpl-hanging",
			"object": "chat.completion.chunk",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"content": "starting... ",
					},
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// 然后永远阻塞（直到客户端断开）
		<-r.Context().Done()
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}

func sendLeakTestChatRequest(t *testing.T, serverAddr string, id int) {
	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Hello %d", id)},
		},
		"stream": true,
	}
	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/v1/chat/completions", serverAddr), bytes.NewReader(data))
	if err != nil {
		t.Logf("Request %d: failed to create request: %v", id, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Request %d: failed: %v", id, err)
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, _ := io.ReadAll(resp.Body)
	t.Logf("Request %d: completed with status %d, body length: %d", id, resp.StatusCode, len(body))
}

func sendDirectChatRequest(t *testing.T, serverAddr string, id int) {
	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Hello %d", id)},
		},
		"stream": true,
	}
	data, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("http://%s/v1/chat/completions", serverAddr)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		t.Logf("Direct request %d: failed to create: %v", id, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Direct request %d: failed: %v", id, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Direct request %d: completed, body length: %d", id, len(body))
}

func sendDirectChatRequestWithTimeout(t *testing.T, serverAddr string, id int, timeout time.Duration) {
	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Hello %d", id)},
		},
		"stream": true,
	}
	data, _ := json.Marshal(reqBody)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	url := fmt.Sprintf("http://%s/v1/chat/completions", serverAddr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		t.Logf("Timeout request %d: failed to create: %v", id, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Timeout request %d: failed (expected): %v", id, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Timeout request %d: completed, body length: %d", id, len(body))
}

func printGoroutineStacks(t *testing.T) {
	printDetailedGoroutineStacks(t)
}

func printDetailedGoroutineStacks(t *testing.T) {
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	fullDump := buf.String()

	// 按函数签名分组统计
	type stackInfo struct {
		count  int
		sample string
	}
	stackMap := make(map[string]*stackInfo)

	blocks := strings.Split(fullDump, "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || !strings.HasPrefix(block, "goroutine ") {
			continue
		}

		// 提取函数签名
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
			// 找到第一个函数调用
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

		if signature == "" {
			signature = "unknown"
		}

		if info, exists := stackMap[signature]; exists {
			info.count++
		} else {
			stackMap[signature] = &stackInfo{count: 1, sample: block}
		}
	}

	// 打印统计
	t.Log("\n=== Goroutine Stack Summary ===")
	for sig, info := range stackMap {
		if info.count > 1 || strings.Contains(sig, "aibalance") || strings.Contains(sig, "aispec") {
			t.Logf("  %d goroutines: %s", info.count, sig)
		}
	}

	// 打印疑似泄漏的堆栈详情
	t.Log("\n=== Suspicious Goroutine Stacks (aibalance/aispec related) ===")
	for sig, info := range stackMap {
		if strings.Contains(sig, "aibalance") || strings.Contains(sig, "aispec") || strings.Contains(sig, "lowhttp") {
			t.Logf("\n--- %s (%d goroutines) ---\n%s", sig, info.count, info.sample)
		}
	}

	log.Warnf("Total unique stacks: %d", len(stackMap))
}
