package aibalance

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 实验目的：验证 goroutine 泄漏的根本原因
//
// 假设1: goroutine 泄漏是因为 AI provider 流式响应没有正确结束
// 假设2: goroutine 泄漏是因为 onStream 中的 io.Copy 永远无法退出
// 假设3: goroutine 泄漏是因为 HTTP 连接没有设置正确的超时

// TestExperiment1_HangingProviderCausesGoroutineLeak 验证当 AI provider 卡住时是否会导致 goroutine 泄漏
func TestExperiment1_HangingProviderCausesGoroutineLeak(t *testing.T) {
	// 创建一个会卡住的 mock server
	hangingServer := createHangingServer(t)
	defer hangingServer.Close()

	t.Logf("Hanging server started at: %s", hangingServer.Addr)

	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 模拟 onStream 函数中的行为
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 创建一个带超时的连接
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/hanging", hangingServer.Addr), strings.NewReader("test"))
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Logf("Request %d error (expected): %v", id, err)
				return
			}
			defer resp.Body.Close()

			// 模拟 onStream 中的 io.Copy
			buf := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					t.Logf("Request %d received: %s", id, string(buf[:n]))
				}
				if err == io.EOF {
					t.Logf("Request %d: EOF", id)
					break
				}
				if err != nil {
					t.Logf("Request %d: read error: %v", id, err)
					break
				}
			}
		}(i)
	}

	// 等待所有请求完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All requests completed")
	case <-time.After(10 * time.Second):
		t.Log("Timeout waiting for requests")
	}

	// 等待一下让 GC 和 goroutine 清理
	time.Sleep(1 * time.Second)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (leaked: %d)", finalGoroutines, finalGoroutines-initialGoroutines)

	// 验证是否有泄漏
	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Goroutine leak detected! Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
	}
}

// TestExperiment2_PipeBlockingCausesGoroutineLeak 验证 pipe 阻塞是否会导致 goroutine 泄漏
func TestExperiment2_PipeBlockingCausesGoroutineLeak(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	var leakedGoroutines int32

	for i := 0; i < 10; i++ {
		// 创建 pipe，模拟 server.go 中的使用
		pr, pw := io.Pipe()

		var wg sync.WaitGroup
		wg.Add(1)

		// 模拟写入端 goroutine（像 onStream 一样）
		go func() {
			defer wg.Done()
			defer pw.Close()
			// 写入一些数据
			pw.Write([]byte("some data"))
			// 注意：这里没有阻塞，正常结束
		}()

		// 模拟读取端
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := pr.Read(buf)
				if err != nil {
					break
				}
				_ = n
			}
			pr.Close()
		}()

		wg.Wait()
	}

	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %d)", finalGoroutines, finalGoroutines-initialGoroutines)

	if finalGoroutines > initialGoroutines+2 {
		atomic.AddInt32(&leakedGoroutines, int32(finalGoroutines-initialGoroutines))
		t.Errorf("Goroutine leak in pipe scenario! Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
	}
}

// TestExperiment3_BlockedPipeWriteCausesLeak 验证当 pipe 写入被阻塞时是否会导致 goroutine 泄漏
func TestExperiment3_BlockedPipeWriteCausesLeak(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// 这个测试模拟的场景：
	// 1. onStream 正在向 pw 写入数据
	// 2. 但是没有人从 pr 读取
	// 3. pw.Write() 会阻塞
	// 4. 如果我们不关闭 pr，goroutine 会永远阻塞

	for i := 0; i < 5; i++ {
		pr, pw := io.Pipe()

		go func() {
			// 这个 goroutine 会阻塞，因为没有人从 pr 读取
			// 但是 buffer 满了之后就会阻塞
			for j := 0; j < 100; j++ {
				select {
				case <-time.After(10 * time.Millisecond):
					// 模拟超时退出
					pw.Close()
					return
				default:
					_, err := pw.Write([]byte("blocking write attempt\n"))
					if err != nil {
						return
					}
				}
			}
			pw.Close()
		}()

		// 关闭读取端，让写入端可以退出
		time.Sleep(50 * time.Millisecond)
		pr.Close()
	}

	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %d)", finalGoroutines, finalGoroutines-initialGoroutines)

	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Goroutine leak when pipe write is blocked! Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
	}
}

// TestExperiment4_RealWorldScenario 模拟真实世界场景
func TestExperiment4_RealWorldScenario(t *testing.T) {
	// 创建一个模拟 AI 响应的 server
	slowServer := createSlowSSEServer(t)
	defer slowServer.Close()

	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	for i := 0; i < 3; i++ {
		t.Logf("Request %d starting...", i)

		// 创建 pipes (模拟 server.go)
		outputPr, outputPw := io.Pipe()
		reasonPr, reasonPw := io.Pipe()

		var wg sync.WaitGroup

		// 模拟 provider.go 中的 streamHandler
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/sse", slowServer.Addr), strings.NewReader("{}"))
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Logf("Request error: %v", err)
				outputPw.Close()
				return
			}
			defer resp.Body.Close()

			// 这就是 onStream 做的事情
			_, copyErr := io.Copy(outputPw, resp.Body)
			if copyErr != nil {
				t.Logf("Copy error: %v", copyErr)
			}
			outputPw.Close()
		}()

		// 模拟处理输出流
		wg.Add(1)
		go func() {
			defer wg.Done()
			io.Copy(io.Discard, outputPr)
			outputPr.Close()
		}()

		// 关闭 reason pipe（这个测试不用它）
		reasonPw.Close()
		reasonPr.Close()

		// 等待完成
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Logf("Request %d completed normally", i)
		case <-time.After(5 * time.Second):
			t.Logf("Request %d timed out", i)
		}
	}

	time.Sleep(1 * time.Second)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (diff: %d)", finalGoroutines, finalGoroutines-initialGoroutines)

	if finalGoroutines > initialGoroutines+3 {
		t.Errorf("Goroutine leak in real-world scenario! Initial: %d, Final: %d", initialGoroutines, finalGoroutines)
	}
}

// createHangingServer 创建一个会卡住的 HTTP 服务器
func createHangingServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 发送一些数据
			w.Write([]byte("data: start\n\n"))
			w.(http.Flusher).Flush()

			// 然后永远阻塞（模拟 AI provider 卡住）
			<-r.Context().Done()
		}),
	}

	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}

// TestExperiment5_IdentifyLeakingGoroutines 打印当前的 goroutine 堆栈，帮助分析泄漏
func TestExperiment5_IdentifyLeakingGoroutines(t *testing.T) {
	t.Skip("Run this test manually to analyze goroutine leaks")

	// 打印所有 goroutine 的堆栈
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 1)

	// 解析并统计 goroutine
	scanner := bufio.NewScanner(&buf)
	goroutineCount := make(map[string]int)

	var currentStack strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "goroutine ") {
			if currentStack.Len() > 0 {
				// 提取第一个函数调用作为 key
				lines := strings.Split(currentStack.String(), "\n")
				if len(lines) > 1 {
					key := lines[1]
					goroutineCount[key]++
				}
				currentStack.Reset()
			}
		}
		currentStack.WriteString(line + "\n")
	}

	// 打印统计结果
	t.Log("=== Goroutine Statistics ===")
	for key, count := range goroutineCount {
		if count > 1 {
			t.Logf("Count: %d - %s", count, key)
		}
	}
	t.Logf("Total goroutines: %d", runtime.NumGoroutine())

	// 打印完整堆栈
	t.Log("\n=== Full Goroutine Dump ===")
	t.Log(buf.String())
}

// createSlowSSEServer 创建一个慢速 SSE 服务器
func createSlowSSEServer(t *testing.T) *http.Server {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming not supported", http.StatusInternalServerError)
				return
			}

			// 发送几个事件，然后慢慢结束
			for i := 0; i < 3; i++ {
				select {
				case <-r.Context().Done():
					return
				default:
					fmt.Fprintf(w, "data: {\"chunk\": %d}\n\n", i)
					flusher.Flush()
					time.Sleep(200 * time.Millisecond)
				}
			}

			// 发送结束标记
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}),
	}

	go server.Serve(listener)
	server.Addr = listener.Addr().String()

	return server
}
