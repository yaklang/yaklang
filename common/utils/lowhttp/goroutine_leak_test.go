package lowhttp

import (
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

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type goroutineLeakChecker struct {
	t        *testing.T
	allow    int
	timeout  time.Duration
	interval time.Duration
	base     int
}

func newGoroutineLeakChecker(t *testing.T, allow int, timeout time.Duration) *goroutineLeakChecker {
	t.Helper()
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &goroutineLeakChecker{
		t:        t,
		allow:    allow,
		timeout:  timeout,
		interval: 50 * time.Millisecond,
		base:     runtime.NumGoroutine(),
	}
}

func (c *goroutineLeakChecker) Check() {
	c.t.Helper()
	deadline := time.Now().Add(c.timeout)
	var current int
	for {
		runtime.GC()
		runtime.GC()
		current = runtime.NumGoroutine()
		if current-c.base <= c.allow {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(c.interval)
	}
	diff := current - c.base
	log.Infof("goroutine leak check failed: base=%d current=%d diff=%d allow=%d", c.base, current, diff, c.allow)
	log.Infof("goroutine stacks:\n%s", dumpAllGoroutines())
	c.t.Fatalf("goroutine leak detected: base=%d current=%d diff=%d allow=%d", c.base, current, diff, c.allow)
}

func dumpAllGoroutines() string {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
	return buf.String()
}

func countPersistConnGoroutines() int {
	stacks := dumpAllGoroutines()
	count := 0
	for _, line := range strings.Split(stacks, "\n") {
		if strings.Contains(line, "lowhttp.(*persistConn).readLoop") ||
			strings.Contains(line, "lowhttp.(*persistConn).writeLoop") {
			count++
		}
	}
	return count
}

func assertNoGoroutineLeak(t *testing.T, name string, fn func()) {
	t.Helper()
	log.Infof("goroutine leak check start: %s", name)
	checker := newGoroutineLeakChecker(t, 6, 2*time.Second)
	fn()
	checker.Check()
}

func buildBasicRequest(host string, port int) []byte {
	return []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))
}

func TestGoroutineLeak_BodyStreamReaderHandler_NoConnPool(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(200)
		_, _ = writer.Write(bytes.Repeat([]byte("a"), 1024))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	checker := newGoroutineLeakChecker(t, 5, 3*time.Second)
	for i := 0; i < 30; i++ {
		_, err := HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithTimeout(500*time.Millisecond),
			WithBodyStreamReaderHandler(func(header []byte, body io.ReadCloser) {
				buf := make([]byte, 16)
				_, _ = body.Read(buf)
				_ = body.Close()
			}),
		)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	checker.Check()
}

func TestGoroutineLeak_BodyStreamReaderHandler_ConnPool(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(200)
		_, _ = writer.Write(bytes.Repeat([]byte("b"), 2048))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	pool := NewHttpConnPool(context.Background(), 10, 2)
	t.Cleanup(func() { pool.Clear() })

	checker := newGoroutineLeakChecker(t, 5, 3*time.Second)
	for i := 0; i < 30; i++ {
		_, err := HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(500*time.Millisecond),
			WithBodyStreamReaderHandler(func(header []byte, body io.ReadCloser) {
				buf := make([]byte, 32)
				_, _ = body.Read(buf)
				_ = body.Close()
			}),
		)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	checker.Check()
}

func TestGoroutineLeak_BodyStreamReaderHandler_Panic(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(200)
		_, _ = writer.Write(bytes.Repeat([]byte("c"), 512))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	checker := newGoroutineLeakChecker(t, 5, 3*time.Second)
	for i := 0; i < 10; i++ {
		_, err := HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithTimeout(500*time.Millisecond),
			WithBodyStreamReaderHandler(func(header []byte, body io.ReadCloser) {
				panic("handler panic for test")
			}),
		)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	}
	checker.Check()
}

func TestGoroutineLeak_ConnPool_DroppedResponse(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(300 * time.Millisecond)
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	pool := NewHttpConnPool(context.Background(), 10, 2)
	t.Cleanup(func() { pool.Clear() })

	checker := newGoroutineLeakChecker(t, 5, 3*time.Second)
	done := make(chan struct{})
	go func() {
		_, _ = HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(200*time.Millisecond),
		)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	pool.Clear()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish in time")
	}
	checker.Check()
}

func TestGoroutineLeak_HTTP2Timeout(t *testing.T) {
	var port int
	var lis net.Listener
	var err error
	for i := 0; i < 10; i++ {
		port = utils.GetRandomAvailableTCPPort()
		lis, err = net.Listen("tcp", utils.HostPort("127.0.0.1", port))
		if err != nil {
			continue
		}
		break
	}
	if lis == nil {
		t.Fatal("listener is nil")
	}
	defer lis.Close()

	blockCh := make(chan struct{})
	serveDone := make(chan error, 1)
	var conn net.Conn
	go func() {
		c, acceptErr := lis.Accept()
		if acceptErr != nil {
			serveDone <- acceptErr
			return
		}
		conn = c
		serveDone <- serveH2(c, c, withH2Handler(func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
			<-blockCh
			return nil, nil, io.EOF
		}))
	}()

	checker := newGoroutineLeakChecker(t, 5, 3*time.Second)
	reqBytes := []byte("GET / HTTP/2\r\nHost: 127.0.0.1\r\n\r\n")
	_, err = HTTPWithoutRedirect(
		WithHttps(false),
		WithHttp2(true),
		WithPacketBytes(reqBytes),
		WithHost("127.0.0.1"),
		WithPort(port),
		WithTimeout(100*time.Millisecond),
	)
	if err == nil {
		t.Fatal("expected timeout error for http2")
	}

	close(blockCh)
	if conn != nil {
		_ = conn.Close()
	}

	select {
	case <-serveDone:
	case <-time.After(3 * time.Second):
		t.Fatal("serveH2 did not exit after closing connection")
	}
	checker.Check()
}

func TestGoroutineLeak_ConnPool_Reconnect(t *testing.T) {
	// This test verifies that goroutines are properly cleaned up when RECONNECT is triggered
	// Simulate a server that closes connections after each request (triggering retry logic)
	var requestCount int32
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count%2 == 1 {
			// First request: close connection immediately to trigger RECONNECT
			writer.Header().Set("Connection", "close")
		}
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	pool := NewHttpConnPool(context.Background(), 10, 2)
	t.Cleanup(func() { pool.Clear() })

	checker := newGoroutineLeakChecker(t, 10, 3*time.Second)
	for i := 0; i < 20; i++ {
		_, err := HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(500*time.Millisecond),
		)
		if err != nil {
			t.Logf("request %d failed: %v", i, err)
		}
	}
	pool.Clear()
	checker.Check()
}

func TestGoroutineLeak_ConnPool_ServerClose(t *testing.T) {
	// Test goroutine cleanup when server closes connection unexpectedly
	var requestCount int32
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		// Every 3rd request gets an immediate close
		if count%3 == 0 {
			// Force close by hijacking and closing the connection
			hj, ok := writer.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				if conn != nil {
					_ = conn.Close()
				}
				return
			}
		}
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	pool := NewHttpConnPool(context.Background(), 10, 2)
	t.Cleanup(func() { pool.Clear() })

	checker := newGoroutineLeakChecker(t, 10, 3*time.Second)
	for i := 0; i < 30; i++ {
		_, _ = HTTP(
			WithPacketBytes(buildBasicRequest(host, port)),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(500*time.Millisecond),
		)
	}
	pool.Clear()
	checker.Check()
}

func TestGoroutineLeak_ConnPool_StressMaxGoroutines(t *testing.T) {
	// Wait for any goroutines from previous tests to settle
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	// Record baseline goroutines from previous tests
	baselinePersistConnGoroutines := countPersistConnGoroutines()

	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})
	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	const (
		requests      = 400
		poolSize      = 40
		maxGoroutines = poolSize * 2
	)
	poolCtx, poolCancel := context.WithCancel(context.Background())
	pool := NewHttpConnPool(poolCtx, poolSize, poolSize)

	var wg sync.WaitGroup
	sem := make(chan struct{}, poolSize)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := HTTP(
				WithPacketBytes(buildBasicRequest(host, port)),
				WithConnPool(true),
				ConnPool(pool),
				WithTimeout(500*time.Millisecond),
			)
			if err != nil {
				log.Infof("conn pool stress request failed: %v", err)
			}
		}()
	}
	wg.Wait()
	cancel()

	// Clear the pool and wait for goroutines to exit
	pool.Clear()
	poolCancel()
	time.Sleep(500 * time.Millisecond)
	runtime.GC()

	persistConnGoroutines := countPersistConnGoroutines()
	// Only count the goroutines created by this test (subtract baseline)
	thisTestGoroutines := persistConnGoroutines - baselinePersistConnGoroutines
	if thisTestGoroutines < 0 {
		thisTestGoroutines = 0
	}
	if thisTestGoroutines > maxGoroutines {
		log.Infof("persistConn goroutines exceed limit: current=%d (baseline=%d) limit=%d", persistConnGoroutines, baselinePersistConnGoroutines, maxGoroutines)
		t.Fatalf("persistConn goroutines exceed limit: current=%d (baseline=%d, this_test=%d) limit=%d", persistConnGoroutines, baselinePersistConnGoroutines, thisTestGoroutines, maxGoroutines)
	}
}
