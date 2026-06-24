// server_concurrency_test.go - chat 并发闸回归测试
//
// 覆盖抗坍塌修复: serveChatCompletions 达进程级并发硬上限时不直接 5xx, 而是
// 先排队等待名额释放 (有界等待); 拿到名额即继续, 等待超时才兜底回 429.
//
// 关键词: chat 并发闸排队等待回归, maxConcurrentChatRequests 过载保护, 避免 5xx

package aibalance

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcquireChatSlot_FastPath 未达上限时立即占位 (queued=false).
func TestAcquireChatSlot_FastPath(t *testing.T) {
	c := &ServerConfig{chatSlotFreed: make(chan struct{}, 1)}
	c.SetMaxConcurrentChatRequests(2)

	queued, ok := c.acquireChatSlotOrQueue()
	require.True(t, ok)
	assert.False(t, queued, "first request should not queue")
	assert.Equal(t, int64(1), atomic.LoadInt64(&c.concurrentChatRequests))

	queued2, ok2 := c.acquireChatSlotOrQueue()
	require.True(t, ok2)
	assert.False(t, queued2, "second request still under cap, no queue")
	assert.Equal(t, int64(2), atomic.LoadInt64(&c.concurrentChatRequests))
}

// TestAcquireChatSlot_QueuedThenAdmitted 达上限时排队等待, 名额释放后被放行.
func TestAcquireChatSlot_QueuedThenAdmitted(t *testing.T) {
	c := &ServerConfig{chatSlotFreed: make(chan struct{}, 1)}
	c.SetMaxConcurrentChatRequests(1)
	c.SetChatQueueWaitTimeout(3 * time.Second)
	// 占满唯一名额.
	atomic.StoreInt64(&c.concurrentChatRequests, 1)

	done := make(chan struct{ queued, ok bool }, 1)
	go func() {
		queued, ok := c.acquireChatSlotOrQueue()
		done <- struct{ queued, ok bool }{queued, ok}
	}()

	// 让其先进入排队, 再释放一个名额并发信号.
	time.Sleep(150 * time.Millisecond)
	atomic.AddInt64(&c.concurrentChatRequests, -1)
	c.signalChatSlotFreed()

	select {
	case r := <-done:
		assert.True(t, r.ok, "queued request must be admitted after a slot frees")
		assert.True(t, r.queued, "this request did queue before admission")
		assert.Equal(t, int64(1), atomic.LoadInt64(&c.concurrentChatRequests))
	case <-time.After(2 * time.Second):
		t.Fatal("queued request was not admitted in time")
	}
}

// TestAcquireChatSlot_Timeout 达上限且无名额释放时, 排队超时返回 ok=false 且不占名额.
func TestAcquireChatSlot_Timeout(t *testing.T) {
	c := &ServerConfig{chatSlotFreed: make(chan struct{}, 1)}
	c.SetMaxConcurrentChatRequests(1)
	c.SetChatQueueWaitTimeout(200 * time.Millisecond)
	atomic.StoreInt64(&c.concurrentChatRequests, 1)

	start := time.Now()
	queued, ok := c.acquireChatSlotOrQueue()
	elapsed := time.Since(start)

	assert.False(t, ok, "no slot freed before timeout, must fail to acquire")
	assert.True(t, queued, "request did queue before giving up")
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond, "should wait the full queue timeout")
	assert.Equal(t, int64(1), atomic.LoadInt64(&c.concurrentChatRequests),
		"timed-out request must not hold a concurrency slot")
}

// TestChatConcurrencyQueueTimeoutReturns429 排队超时后整条链路回 429 (非 5xx).
func TestChatConcurrencyQueueTimeoutReturns429(t *testing.T) {
	c := &ServerConfig{chatSlotFreed: make(chan struct{}, 1)}
	c.SetMaxConcurrentChatRequests(1)
	c.SetChatQueueWaitTimeout(200 * time.Millisecond)
	// 模拟已有 1 个在途请求, 使本次排队且无名额释放.
	atomic.StoreInt64(&c.concurrentChatRequests, 1)

	serverConn, clientConn := net.Pipe()
	rawPacket := []byte("POST /v1/chat/completions HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\n{}")

	go func() {
		c.serveChatCompletions(serverConn, rawPacket)
		serverConn.Close()
	}()

	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(3*time.Second)))
	buf := make([]byte, 2048)
	n, _ := clientConn.Read(buf)
	resp := string(buf[:n])
	_ = clientConn.Close()

	assert.Contains(t, resp, "429", "queue-wait timeout must fall back to 429, not 5xx")
	assert.NotContains(t, resp, "503", "should not use 5xx for concurrency limiting")
	assert.Contains(t, resp, "Retry-After", "fallback should carry Retry-After")
}

// TestChatConcurrencyCapDisabled max=0 时不限并发 (不会因并发计数被拦).
func TestChatConcurrencyCapDisabled(t *testing.T) {
	c := &ServerConfig{chatSlotFreed: make(chan struct{}, 1)}
	c.SetMaxConcurrentChatRequests(0) // 不限
	atomic.StoreInt64(&c.concurrentChatRequests, 100)

	queued, ok := c.acquireChatSlotOrQueue()
	require.True(t, ok, "max=0 must always admit")
	assert.False(t, queued, "max=0 never queues")
	assert.Equal(t, int64(0), c.MaxConcurrentChatRequests())
}
