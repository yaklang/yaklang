package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capability_probe_test.go 用 httptest mock 上游验证 ProbeToolCallsForProvider 的正确性.
//
// 关键词: aibalance capability probe test, native vs dumb upstream, round1 / round2 探测

// ---------- mock upstream helpers ----------

// startProbeNativeUpstream 模拟一个原生支持 OpenAI tool_calls 协议的 wrapper.
// 关键词: probe native mock, OpenAI tool_calls SSE
func startProbeNativeUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		hasTools := strings.Contains(string(body), `"tools"`) && strings.Contains(string(body), probeToolName)
		hasRoleTool := strings.Contains(string(body), `"role":"tool"`) || strings.Contains(string(body), `"role": "tool"`)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(3 * time.Millisecond)
		}

		switch {
		case hasRoleTool:
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
			for _, piece := range []string{"Ping result is ok.", " Echo says pong."} {
				writeFrame(fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"content":%q}}]}`, piece))
			}
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		case hasTools:
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
			writeFrame(fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":%q,"type":"function","function":{"name":%q,"arguments":""}}]}}]}`, probeToolCallID, probeToolName))
			writeFrame(fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`))
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		default:
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{"role":"assistant","content":"plain"}}]}`)
			writeFrame(`{"id":"c1","object":"chat.completion.chunk","model":"native","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

// startProbeDumbUpstream 模拟一个完全不识别 OpenAI tool_calls 协议的 wrapper:
// 含 tools 字段 -> 不调工具, 回纯文本; 含 role=tool 消息 -> 直接 finish_reason=stop 空回.
// 这是线上 z-deepseek-v4-pro 实际观察到的故障行为.
// 关键词: probe dumb mock, hostile wrapper 复刻
func startProbeDumbUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		hasRoleTool := strings.Contains(string(body), `"role":"tool"`) || strings.Contains(string(body), `"role": "tool"`)
		hasToolCalls := strings.Contains(string(body), `"tool_calls"`)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(3 * time.Millisecond)
		}

		if hasRoleTool || hasToolCalls {
			// 复刻 z-deepseek-v4-pro round2 故障: 立即 finish_reason=stop 空回
			writeFrame(`{"id":"d1","object":"chat.completion.chunk","model":"dumb","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
			writeFrame(`{"id":"d1","object":"chat.completion.chunk","model":"dumb","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		} else {
			// 含 tools 字段时, 不识别工具协议, 直接 NL 回 (不调工具)
			writeFrame(`{"id":"d1","object":"chat.completion.chunk","model":"dumb","choices":[{"index":0,"delta":{"role":"assistant","content":"I cannot call any tool."}}]}`)
			writeFrame(`{"id":"d1","object":"chat.completion.chunk","model":"dumb","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
}

// ---------- 单元测试 ----------

// 关键词: probe native upstream, round1 native 探测命中, round2 native 探测命中
func TestProbeToolCallsForProvider_Native(t *testing.T) {
	srv := startProbeNativeUpstream(t)
	defer srv.Close()

	p := &Provider{
		ModelName:   "probe-native-model",
		TypeName:    "deepseek",
		DomainOrURL: srv.URL,
		APIKey:      "test-upstream-key",
		NoHTTPS:     true,
		WrapperName: "probe-native",
	}
	result, err := ProbeToolCallsForProvider(p)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "native", result.Round1Mode, "native upstream should return native round1")
	assert.Equal(t, "native", result.Round2Mode, "native upstream should return native round2")
	assert.Empty(t, result.Error, "no probe error expected for native upstream")
}

// 关键词: probe dumb upstream, round1/round2 都 fallback react
func TestProbeToolCallsForProvider_Dumb(t *testing.T) {
	srv := startProbeDumbUpstream(t)
	defer srv.Close()

	p := &Provider{
		ModelName:   "probe-dumb-model",
		TypeName:    "deepseek",
		DomainOrURL: srv.URL,
		APIKey:      "test-upstream-key",
		NoHTTPS:     true,
		WrapperName: "probe-dumb",
	}
	result, err := ProbeToolCallsForProvider(p)
	require.NoError(t, err)
	require.NotNil(t, result)
	// round1 没拿到 tool_calls 回调 -> react
	assert.Equal(t, "react", result.Round1Mode, "dumb upstream should fall back to react in round1")
	// round2 上游空回 -> react
	assert.Equal(t, "react", result.Round2Mode, "dumb upstream should fall back to react in round2")
}

// 关键词: probe ConnRefused 超时, mode 字段保留旧值
func TestProbeToolCallsForProvider_Unreachable(t *testing.T) {
	p := &Provider{
		ModelName:   "probe-unreachable-model",
		TypeName:    "deepseek",
		DomainOrURL: "http://127.0.0.1:1", // 不可连通
		APIKey:      "test-upstream-key",
		NoHTTPS:     true,
		WrapperName: "probe-unreachable",
	}
	t0 := time.Now()
	result, err := ProbeToolCallsForProvider(p)
	dur := time.Since(t0)
	require.NoError(t, err)
	require.NotNil(t, result)
	// 探测失败保留 react 默认 + error 字段有内容
	assert.Equal(t, "react", result.Round1Mode)
	assert.Equal(t, "react", result.Round2Mode)
	assert.NotEmpty(t, result.Error)
	assert.Less(t, dur, 35*time.Second, "two 15s timeouts cap")
}

// 关键词: probe 序列化, ProbeResult JSON 兼容
func TestProbeResult_JsonShape(t *testing.T) {
	r := &ProbeResult{
		Round1Mode: "native",
		Round2Mode: "react",
		ProbedAt:   time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Error:      "round2: timeout after 15s",
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	got := string(b)
	assert.Contains(t, got, `"round1_mode":"native"`)
	assert.Contains(t, got, `"round2_mode":"react"`)
	assert.Contains(t, got, `"error"`)
	assert.Contains(t, got, `"probed_at"`)
}

// 关键词: probe 并发安全, 多 goroutine 同时探测同 provider
func TestProbeToolCallsForProvider_Concurrent(t *testing.T) {
	srv := startProbeNativeUpstream(t)
	defer srv.Close()

	p := &Provider{
		ModelName:   "probe-concurrent-model",
		TypeName:    "deepseek",
		DomainOrURL: srv.URL,
		APIKey:      "test-upstream-key",
		NoHTTPS:     true,
		WrapperName: "probe-concurrent",
	}
	const N = 4
	var ok int32
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			r, err := ProbeToolCallsForProvider(p)
			if err == nil && r.Round1Mode == "native" && r.Round2Mode == "native" {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	timeout := time.After(40 * time.Second)
	for i := 0; i < N; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatalf("concurrent probe timed out, %d/%d done", i, N)
		}
	}
	assert.EqualValues(t, N, atomic.LoadInt32(&ok), "all concurrent probes should succeed")
}
