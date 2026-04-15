package aicommon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func newTestConfigForHandle429(ctx context.Context) *Config {
	var mu sync.Mutex
	events := make([]*schema.AiOutputEvent, 0)
	emitter := NewEmitter("test-429", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
		return e, nil
	})
	return &Config{
		Ctx:     ctx,
		Emitter: emitter,
	}
}

func collectEvents(cfg *Config) []*schema.AiOutputEvent {
	cfg.Emitter.WaitForStream()
	return nil
}

func make429Response(headers ...string) *AIResponse {
	header := "HTTP/1.1 429 Too Many Requests\r\n"
	for _, h := range headers {
		header += h + "\r\n"
	}
	header += "\r\n"
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte(header), []byte(`{"error":"rate limited"}`))
	return rsp
}

func make200Response() *AIResponse {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
		[]byte(`{"choices":[]}`),
	)
	return rsp
}

func TestHandle429_NilResponse(t *testing.T) {
	cfg := newTestConfigForHandle429(context.Background())
	is429, ctxDone := cfg.handle429RateLimit(nil)
	assert.False(t, is429)
	assert.False(t, ctxDone)
}

func TestHandle429_Non429StatusCode(t *testing.T) {
	cfg := newTestConfigForHandle429(context.Background())

	rsp200 := make200Response()
	is429, ctxDone := cfg.handle429RateLimit(rsp200)
	assert.False(t, is429)
	assert.False(t, ctxDone)

	rsp500 := NewUnboundAIResponse()
	rsp500.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"),
		nil,
	)
	is429, ctxDone = cfg.handle429RateLimit(rsp500)
	assert.False(t, is429)
	assert.False(t, ctxDone)
}

func TestHandle429_Generic429_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response()

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second, "should return immediately when context is already cancelled")
}

func TestHandle429_AIBalance_ParseableQueue_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: 3")

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second, "should return immediately on cancelled context")
}

func TestHandle429_AIBalance_UnparseableQueue_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: abc")

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second, "should return immediately on cancelled context")
}

func TestHandle429_AIBalance_ZeroQueue_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: 0")

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second)
}

func TestHandle429_Generic429_WaitsWhenContextAlive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response()

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.False(t, ctxDone)
	require.GreaterOrEqual(t, elapsed, 4*time.Second, "should wait at least 5s for generic 429")
}

func TestHandle429_AIBalance_ParseableQueue_WaitDuration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: 1")

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.False(t, ctxDone)
	require.GreaterOrEqual(t, elapsed, 4*time.Second, "queue=1 => waitSec=max(3,5)=5, should wait ~5s")
	require.Less(t, elapsed, 8*time.Second, "should not wait much longer than 5s")
}

func TestHandle429_ContextCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: 100")

	done := make(chan struct{})
	var is429, ctxDone bool
	go func() {
		is429, ctxDone = cfg.handle429RateLimit(rsp)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		assert.True(t, is429)
		assert.True(t, ctxDone, "should detect context cancellation")
	case <-time.After(3 * time.Second):
		t.Fatal("handle429RateLimit did not return after context cancellation")
	}
}

func TestHandle429_AsyncHeaderSet_429Detected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newTestConfigForHandle429(ctx)
	rsp := NewUnboundAIResponse()

	done := make(chan struct{})
	var is429, ctxDone bool
	go func() {
		is429, ctxDone = cfg.handle429RateLimit(rsp)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	header429 := "HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Info: 2\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(header429), []byte(`{"error":"rate limited"}`))

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		assert.True(t, is429, "should detect 429 even when headers arrive asynchronously")
		assert.True(t, ctxDone, "should exit via context cancel during wait")
	case <-time.After(5 * time.Second):
		t.Fatal("handle429RateLimit blocked too long after async header set")
	}
}

func TestHandle429_AsyncHeaderSet_Non429(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := newTestConfigForHandle429(ctx)
	rsp := NewUnboundAIResponse()

	done := make(chan struct{})
	var is429, ctxDone bool
	go func() {
		is429, ctxDone = cfg.handle429RateLimit(rsp)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	header200 := "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(header200), []byte(`{"choices":[]}`))

	select {
	case <-done:
		assert.False(t, is429, "should not detect 429 for 200 response")
		assert.False(t, ctxDone)
	case <-time.After(3 * time.Second):
		t.Fatal("handle429RateLimit blocked too long")
	}
}

func TestHandle429_ContextCancelBeforeHeaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newTestConfigForHandle429(ctx)
	rsp := NewUnboundAIResponse()

	done := make(chan struct{})
	var is429, ctxDone bool
	go func() {
		is429, ctxDone = cfg.handle429RateLimit(rsp)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		assert.False(t, is429, "should not detect 429 when context cancelled before headers")
		assert.True(t, ctxDone)
	case <-time.After(3 * time.Second):
		t.Fatal("handle429RateLimit did not return after context cancellation")
	}
}
