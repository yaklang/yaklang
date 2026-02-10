package utils

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

func TestReadConnWithContextTimeout(t *testing.T) {
	host, port := DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			writer.Write([]byte("hello world " + fmt.Sprint(i)))
			writer.(http.Flusher).Flush()
		}
		return
	})
	conn, err := net.Dial("tcp", HostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + HostPort(host, port) + "\r\n\r\n"))
	time.Sleep(300 * time.Millisecond)
	bytes, err := ReadConnUntil(conn, 300*time.Millisecond)
	spew.Dump(bytes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), `hello world 8`) {
		t.Fatal("should not have read all")
	}
	conn.Close()
}

func TestReadConnWithTimeout(t *testing.T) {
	var listener net.Listener
	host, port := DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		listener = lis
		time.Sleep(500 * time.Millisecond)
		_, err := conn.Write([]byte("hello"))
		if err != nil {
			log.Errorf("write tcp failed: %v", err)
		}
	})
	if listener != nil {
		defer func() {
			_ = listener.Close()
		}()
	}
	addr := HostPort(host, port)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Logf("failed dail %v: %s", addr, err)
		t.FailNow()
	}

	data, err := ReadConnWithTimeout(c, 200*time.Millisecond)
	if err == nil {
		t.Logf("BUG: should not read data: %s", string(data))
		t.FailNow()
	}

	data, err = ReadConnWithTimeout(c, 500*time.Millisecond)
	if err != nil {
		t.Logf("BUG: should have read data: %s", err)
		t.FailNow()
	}

	if string(data) != "hello" {
		t.Logf("read data is not hello: %s", data)
		t.FailNow()
	}
}

func TestTrigger(t *testing.T) {
	var check = false
	NewTriggerWriter(10, func(buffer io.ReadCloser, _ string) {
		check = true
	}).Write([]byte("àf.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)"))
	if !check {
		t.Fatal("should have triggered")
	}

	check = true
	NewTriggerWriter(100000, func(buffer io.ReadCloser, _ string) {
		check = false
	}).Write([]byte("àf.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)"))
	if !check {
		t.Fatal("should have non-triggered")
	}
}

func TestReadWithContextTickCallback_NoRepeatAfterEOF(t *testing.T) {
	// 测试当 reader 读取完毕后，callback 不会被重复调用
	ctx := context.Background()
	reader := strings.NewReader("hello world")

	callCount := 0

	done := make(chan struct{})
	go func() {
		defer close(done)
		ReadWithContextTickCallback(ctx, reader, func(data []byte) bool {
			callCount++
			t.Logf("Callback called %d times, data length: %d", callCount, len(data))
			return true // 继续读取
		}, 100*time.Millisecond)
	}()

	// 等待读取完成
	select {
	case <-done:
		// 读取完成
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for read to complete")
	}

	// 记录完成时的调用次数
	initialCallCount := callCount
	t.Logf("Initial call count: %d", initialCallCount)

	// 等待一段时间，确保不会有额外的调用
	time.Sleep(500 * time.Millisecond)

	if callCount != initialCallCount {
		t.Fatalf("callback was called %d more times after EOF (initial: %d, final: %d)",
			callCount-initialCallCount, initialCallCount, callCount)
	}

	t.Logf("Test passed: callback was not repeatedly called after EOF")
}

func TestReadWithContextTickCallback_LastDataConsumed(t *testing.T) {
	// 测试竞态问题：确保最后写入的数据一定被消费
	ctx := context.Background()

	// 创建一个管道来模拟流式数据
	r, w := io.Pipe()

	// 模拟数据生产者
	go func() {
		defer w.Close()
		for i := 0; i < 5; i++ {
			msg := fmt.Sprintf("message-%d\n", i)
			w.Write([]byte(msg))
			time.Sleep(50 * time.Millisecond)
		}
		// 写入最后一条重要数据
		w.Write([]byte("FINAL-MESSAGE"))
	}()

	var lastData []byte
	callCount := 0
	var dataLengths []int

	done := make(chan struct{})
	go func() {
		defer close(done)
		ReadWithContextTickCallback(ctx, r, func(data []byte) bool {
			callCount++
			// 记录最后一次收到的数据
			lastData = make([]byte, len(data))
			copy(lastData, data)
			dataLengths = append(dataLengths, len(data))
			t.Logf("Callback #%d: received %d bytes", callCount, len(data))
			return true
		}, 80*time.Millisecond)
	}()

	// 等待读取完成
	select {
	case <-done:
		// 读取完成
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for read to complete")
	}

	// 验证最后的数据包含 FINAL-MESSAGE
	if !strings.Contains(string(lastData), "FINAL-MESSAGE") {
		t.Fatalf("Last data should contain 'FINAL-MESSAGE', but got: %s", string(lastData))
	}

	// 验证数据长度是递增的，不会出现相同长度（即不会重复调用）
	for i := 1; i < len(dataLengths); i++ {
		if dataLengths[i] <= dataLengths[i-1] {
			t.Fatalf("Data length should be increasing, but got %d after %d at callback #%d",
				dataLengths[i], dataLengths[i-1], i+1)
		}
	}

	t.Logf("Test passed: last data was consumed successfully")
	t.Logf("Total callbacks: %d, final data length: %d", callCount, len(lastData))
	t.Logf("Data lengths progression: %v", dataLengths)
}

func TestReadWithContextTickCallback_NoDoubleFinalCall(t *testing.T) {
	// 测试不会出现最后数据被调用两次的问题
	ctx := context.Background()

	r, w := io.Pipe()

	// 模拟快速完成的数据生产者
	go func() {
		defer w.Close()
		w.Write([]byte("data"))
		// 立即结束，测试 ticker 和 done 的竞态
	}()

	var callHistory []int
	var mux sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		ReadWithContextTickCallback(ctx, r, func(data []byte) bool {
			mux.Lock()
			callHistory = append(callHistory, len(data))
			t.Logf("Callback: received %d bytes", len(data))
			mux.Unlock()
			return true
		}, 50*time.Millisecond)
	}()

	select {
	case <-done:
		// 读取完成
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for read to complete")
	}

	// 等待确保没有额外的调用
	time.Sleep(200 * time.Millisecond)

	mux.Lock()
	finalCallHistory := make([]int, len(callHistory))
	copy(finalCallHistory, callHistory)
	mux.Unlock()

	// 检查是否有重复的长度（表示相同数据被调用多次）
	seen := make(map[int]int)
	for i, length := range finalCallHistory {
		if prevIndex, exists := seen[length]; exists {
			t.Fatalf("Same data length %d appeared at callback #%d and #%d - indicates duplicate call",
				length, prevIndex+1, i+1)
		}
		seen[length] = i
	}

	t.Logf("Test passed: no duplicate final calls")
	t.Logf("Call history (data lengths): %v", finalCallHistory)
}
