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

/*
================================================================================
EXPERIMENT: Reproduce and Verify goroutine leak fix in lowhttp connection pool

PROBLEM DESCRIPTION:
When a connection pool request fails and triggers RECONNECT (goto RECONNECT),
the old persistConn's readLoop and writeLoop goroutines were NOT being terminated
because pc.cancel() was never called.

ROOT CAUSE:
In exec.go, at multiple "goto RECONNECT" locations, the old persistConn was
abandoned without calling closeConn(), leaving goroutines blocked forever on:
- readLoop: waiting on pc.reqCh or pc.ctx.Done()
- writeLoop: waiting on pc.writeCh or pc.ctx.Done()

FIX:
Added pc.closeConn(err) before each "goto RECONNECT" to properly cancel the
context and allow goroutines to exit.

This test file contains experiments to:
1. Reproduce the leak scenario
2. Verify the fix works
3. Cover edge cases
================================================================================
*/

// getGoroutineSnapshot returns current goroutine count and a summary of stacks
func getGoroutineSnapshot() (int, string) {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
	stacks := buf.String()
	return runtime.NumGoroutine(), stacks
}

// countSpecificGoroutines counts goroutines matching a pattern
func countSpecificGoroutines(pattern string) int {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
	stacks := buf.String()
	count := 0
	for _, line := range strings.Split(stacks, "\n") {
		if strings.Contains(line, pattern) {
			count++
		}
	}
	return count
}

// waitForGoroutineCount waits for goroutine count to reach target
func waitForGoroutineCount(pattern string, maxCount int, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	var count int
	for time.Now().Before(deadline) {
		runtime.GC()
		count = countSpecificGoroutines(pattern)
		if count <= maxCount {
			return count, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return count, false
}

/*
================================================================================
EXPERIMENT 1: Reproduce RECONNECT Goroutine Leak

This experiment simulates the scenario where:
1. A server closes connections after responding (Connection: close)
2. Client with connection pool sends multiple requests
3. Each closed connection triggers internal retry/reconnect logic
4. WITHOUT the fix: readLoop/writeLoop goroutines would leak

Expected behavior:
- With fix: goroutines should be cleaned up properly
- Without fix: goroutines would accumulate (1904 in the original report)
================================================================================
*/
func TestExperiment_ReproduceReconnectLeak(t *testing.T) {
	log.Infof("=== EXPERIMENT 1: Reproduce RECONNECT Goroutine Leak ===")

	// Record baseline
	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

	// Create a server that forces connection close after each response
	var requestCount int32
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		// Force connection close to simulate server behavior that triggers reconnect
		writer.Header().Set("Connection", "close")
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte(fmt.Sprintf("response-%d", count)))
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed to start")
	}

	// Create connection pool
	pool := NewHttpConnPool(context.Background(), 20, 5)
	defer pool.Clear()

	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))

	// Send many requests to trigger reconnect scenarios
	const numRequests = 50
	log.Infof("Sending %d requests with connection pool (each will trigger close)...", numRequests)

	for i := 0; i < numRequests; i++ {
		_, err := HTTP(
			WithPacketBytes(reqBytes),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(2*time.Second),
		)
		if err != nil {
			log.Infof("Request %d failed: %v", i, err)
		}
	}

	log.Infof("Total requests handled by server: %d", atomic.LoadInt32(&requestCount))

	// Clear pool and wait for goroutines to clean up
	pool.Clear()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()

	// Check goroutine count
	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+5, 3*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		_, stacks := getGoroutineSnapshot()
		t.Logf("Goroutine stacks:\n%s", stacks)
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines still running (expected <= %d)",
			finalCount, baselineGoroutines+5)
	}

	log.Infof("=== EXPERIMENT 1 PASSED: No goroutine leak detected ===")
}

/*
================================================================================
EXPERIMENT 2: Server Abrupt Close (EOF on read)

This experiment simulates:
1. Server accepts connection but immediately closes it (before or during response)
2. Client gets EOF error, triggers reconnect
3. Verifies goroutines are cleaned up properly
================================================================================
*/
func TestExperiment_ServerAbruptClose(t *testing.T) {
	log.Infof("=== EXPERIMENT 2: Server Abrupt Close ===")

	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

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

	// Server that randomly closes connections abruptly
	var requestCount int32
	var serverWg sync.WaitGroup
	serverWg.Add(1)
	go func() {
		defer serverWg.Done()
		for {
			conn, acceptErr := lis.Accept()
			if acceptErr != nil {
				return
			}
			count := atomic.AddInt32(&requestCount, 1)
			go func(c net.Conn, idx int32) {
				defer c.Close()
				// Read request
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)

				if idx%3 == 0 {
					// Abrupt close - no response
					return
				} else if idx%3 == 1 {
					// Partial response then close
					_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n"))
					return
				} else {
					// Full response with connection close
					resp := "HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 2\r\n\r\nok"
					_, _ = c.Write([]byte(resp))
				}
			}(conn, count)
		}
	}()

	pool := NewHttpConnPool(context.Background(), 20, 5)
	defer pool.Clear()

	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort("127.0.0.1", port)))

	const numRequests = 30
	log.Infof("Sending %d requests with various server close behaviors...", numRequests)

	for i := 0; i < numRequests; i++ {
		_, _ = HTTP(
			WithPacketBytes(reqBytes),
			WithHost("127.0.0.1"),
			WithPort(port),
			WithConnPool(true),
			ConnPool(pool),
			WithTimeout(1*time.Second),
		)
	}

	log.Infof("Server handled %d requests", atomic.LoadInt32(&requestCount))

	// Cleanup
	lis.Close()
	serverWg.Wait()
	pool.Clear()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()

	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+5, 3*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines leaked", leakedCount)
	}

	log.Infof("=== EXPERIMENT 2 PASSED: No goroutine leak detected ===")
}

/*
================================================================================
EXPERIMENT 3: Concurrent Requests with Mixed Failure Modes

This experiment tests concurrent scenarios:
1. Multiple goroutines sending requests simultaneously
2. Server exhibits mixed behavior (success, close, timeout)
3. High concurrency stress test
================================================================================
*/
func TestExperiment_ConcurrentMixedFailures(t *testing.T) {
	log.Infof("=== EXPERIMENT 3: Concurrent Mixed Failure Modes ===")

	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

	var requestCount int32
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		switch count % 4 {
		case 0:
			// Normal response
			writer.WriteHeader(200)
			_, _ = writer.Write([]byte("ok"))
		case 1:
			// Connection close
			writer.Header().Set("Connection", "close")
			writer.WriteHeader(200)
			_, _ = writer.Write([]byte("close"))
		case 2:
			// Slow response
			time.Sleep(50 * time.Millisecond)
			writer.WriteHeader(200)
			_, _ = writer.Write([]byte("slow"))
		case 3:
			// Large response
			writer.WriteHeader(200)
			_, _ = writer.Write(bytes.Repeat([]byte("x"), 1024))
		}
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed to start")
	}

	pool := NewHttpConnPool(context.Background(), 30, 10)
	defer pool.Clear()

	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))

	const (
		numGoroutines = 20
		numRequests   = 10
	)

	log.Infof("Launching %d goroutines, each sending %d requests...", numGoroutines, numRequests)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < numRequests; i++ {
				_, err := HTTP(
					WithPacketBytes(reqBytes),
					WithConnPool(true),
					ConnPool(pool),
					WithTimeout(500*time.Millisecond),
				)
				if err != nil {
					log.Infof("Goroutine %d request %d failed: %v", gid, i, err)
				}
			}
		}(g)
	}
	wg.Wait()

	log.Infof("Total requests: %d", atomic.LoadInt32(&requestCount))

	// Cleanup
	pool.Clear()
	time.Sleep(300 * time.Millisecond)
	runtime.GC()

	// Allow slightly more goroutines due to timing issues with concurrent cleanup
	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+15, 5*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines leaked under concurrent load", leakedCount)
	}

	log.Infof("=== EXPERIMENT 3 PASSED: No goroutine leak under concurrent load ===")
}

/*
================================================================================
EXPERIMENT 4: HTTP/2 Reconnect Scenarios

This experiment tests HTTP/2 specific reconnect scenarios:
1. Server sends GOAWAY frame
2. Stream creation fails
3. Request/response errors
================================================================================
*/
func TestExperiment_HTTP2ReconnectLeak(t *testing.T) {
	log.Infof("=== EXPERIMENT 4: HTTP/2 Reconnect Scenarios ===")

	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

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

	// H2 server that terminates after first request
	var serverDone = make(chan struct{})
	var requestCount int32
	go func() {
		defer close(serverDone)
		for {
			conn, acceptErr := lis.Accept()
			if acceptErr != nil {
				return
			}
			count := atomic.AddInt32(&requestCount, 1)
			go func(c net.Conn, idx int32) {
				defer c.Close()
				if idx > 5 {
					// Just close after a few requests
					return
				}
				_ = serveH2(c, c, withH2Handler(func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
					if idx%2 == 0 {
						return []byte("HTTP/2.0 200\r\n\r\n"), nil, nil
					}
					// Simulate error
					return nil, nil, io.EOF
				}))
			}(conn, count)
		}
	}()

	reqBytes := []byte("GET / HTTP/2\r\nHost: 127.0.0.1\r\n\r\n")

	const numRequests = 15
	log.Infof("Sending %d HTTP/2 requests with various failure modes...", numRequests)

	for i := 0; i < numRequests; i++ {
		_, _ = HTTPWithoutRedirect(
			WithHttps(false),
			WithHttp2(true),
			WithPacketBytes(reqBytes),
			WithHost("127.0.0.1"),
			WithPort(port),
			WithTimeout(500*time.Millisecond),
		)
	}

	log.Infof("Server handled %d requests", atomic.LoadInt32(&requestCount))

	// Cleanup
	lis.Close()
	<-serverDone
	time.Sleep(300 * time.Millisecond)
	runtime.GC()

	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+5, 3*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines leaked in HTTP/2 scenario", leakedCount)
	}

	log.Infof("=== EXPERIMENT 4 PASSED: No HTTP/2 goroutine leak ===")
}

/*
================================================================================
EXPERIMENT 5: Pool Context Cancellation

This experiment tests:
1. Connection pool with a cancellable context
2. Context is cancelled while requests are in-flight
3. Verifies all goroutines exit when pool context is cancelled
================================================================================
*/
func TestExperiment_PoolContextCancellation(t *testing.T) {
	log.Infof("=== EXPERIMENT 5: Pool Context Cancellation ===")

	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Slow response
		time.Sleep(100 * time.Millisecond)
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed to start")
	}

	// Create pool with cancellable context
	poolCtx, poolCancel := context.WithCancel(context.Background())
	pool := NewHttpConnPool(poolCtx, 20, 5)

	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))

	// Start some requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = HTTP(
				WithPacketBytes(reqBytes),
				WithConnPool(true),
				ConnPool(pool),
				WithTimeout(2*time.Second),
			)
		}()
	}

	// Cancel context while requests are in-flight
	time.Sleep(50 * time.Millisecond)
	log.Infof("Cancelling pool context...")
	poolCancel()
	pool.Clear()

	wg.Wait()
	time.Sleep(300 * time.Millisecond)
	runtime.GC()

	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+5, 3*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines leaked after context cancellation", leakedCount)
	}

	log.Infof("=== EXPERIMENT 5 PASSED: Context cancellation properly cleaned up goroutines ===")
}

/*
================================================================================
EXPERIMENT 6: Edge Case - Rapid Pool Clear During Active Connections

This experiment tests the edge case where:
1. Multiple active connections are in use
2. Pool.Clear() is called rapidly/repeatedly
3. Verifies no race conditions or goroutine leaks
================================================================================
*/
func TestExperiment_RapidPoolClear(t *testing.T) {
	log.Infof("=== EXPERIMENT 6: Rapid Pool Clear ===")

	baselineGoroutines := countSpecificGoroutines("persistConn")
	log.Infof("Baseline persistConn goroutines: %d", baselineGoroutines)

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte("ok"))
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed to start")
	}

	pool := NewHttpConnPool(context.Background(), 20, 5)
	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))

	// Goroutine sending requests continuously
	var done int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&done) == 0 {
			_, _ = HTTP(
				WithPacketBytes(reqBytes),
				WithConnPool(true),
				ConnPool(pool),
				WithTimeout(500*time.Millisecond),
			)
		}
	}()

	// Rapidly clear pool multiple times
	for i := 0; i < 5; i++ {
		time.Sleep(20 * time.Millisecond)
		pool.Clear()
	}

	// Stop requests
	atomic.StoreInt32(&done, 1)
	wg.Wait()

	pool.Clear()
	time.Sleep(300 * time.Millisecond)
	runtime.GC()

	finalCount, ok := waitForGoroutineCount("persistConn", baselineGoroutines+5, 3*time.Second)
	leakedCount := finalCount - baselineGoroutines

	log.Infof("Final persistConn goroutines: %d (leaked: %d)", finalCount, leakedCount)

	if !ok {
		t.Fatalf("LEAK DETECTED: %d persistConn goroutines leaked during rapid clear", leakedCount)
	}

	log.Infof("=== EXPERIMENT 6 PASSED: Rapid pool clear handled correctly ===")
}

/*
================================================================================
SUMMARY TEST: Run all experiments and report overall results
================================================================================
*/
func TestExperiment_AllScenariosSummary(t *testing.T) {
	// This test just logs the summary - individual tests run separately
	t.Log(`
================================================================================
GOROUTINE LEAK FIX VERIFICATION EXPERIMENTS
================================================================================

Experiments designed to reproduce and verify the fix for:
  "1904 goroutine leak in lowhttp.(*persistConn).readLoop"

Root Cause:
  When 'goto RECONNECT' was triggered (connection reuse failure, server close,
  HTTP/2 errors), the old persistConn was abandoned WITHOUT calling closeConn().
  This left readLoop/writeLoop goroutines blocked forever.

Fix Applied (in exec.go):
  Added 'pc.closeConn(err)' before each 'goto RECONNECT' to:
  1. Cancel the persistConn's context
  2. Allow readLoop/writeLoop to exit via <-pc.ctx.Done()
  3. Properly clean up network resources

Experiments:
  1. ReproduceReconnectLeak - Server forces Connection: close
  2. ServerAbruptClose - Server closes without response
  3. ConcurrentMixedFailures - High concurrency with mixed failures
  4. HTTP2ReconnectLeak - HTTP/2 specific reconnect scenarios
  5. PoolContextCancellation - Pool context cancelled during requests
  6. RapidPoolClear - Rapid pool clear during active connections

Run experiments with:
  go test -v -run "TestExperiment_" ./common/utils/lowhttp/...

================================================================================
`)
}
