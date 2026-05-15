package aibalance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: deepseek v4 pro thinking + tool_calls 端到端 bug 捕获
//
// 本文件验证 aibalance 中转层在以下场景的正确性 (TDD 先红后绿):
//
//   1. 流式: 上游返回 reasoning_content 增量 + tool_calls 增量 +
//      finish_reason="tool_calls"。
//      期望 aibalance 转发给客户端的 SSE:
//        - 包含 delta.reasoning_content 片段
//        - 包含 delta.tool_calls 片段 (按 index 可累积)
//        - 末尾 finish_reason 为 "tool_calls" (而不是 "stop")
//        - 以 [DONE] 收尾
//
//   2. 非流式: 上游返回完整 message {reasoning_content, tool_calls,
//      finish_reason="tool_calls"}。
//      期望 aibalance 返回的 chat.completion.chunk 中包含完整 tool_calls
//      数组 (CURRENT BUG: GetNotStreamBody 只走 buildMessage, tool_calls 完全丢失)。

// ============================================================
// 共享: mock 上游 helpers
// ============================================================

// startDeepseekStreamMock 启动一个 mock 上游, 返回 SSE 流 (每帧之间带 keep-alive
// 间隔), 模拟 deepseek-v4-pro thinking + tool_calls 的真实响应顺序:
//
//   1. role: "assistant"
//   2. delta.reasoning_content 多帧 (思考)
//   3. delta.tool_calls 多帧 (函数名 -> 参数 incremental)
//   4. finish_reason="tool_calls"
//   5. [DONE]
//
// 关键词: deepseek mock SSE, thinking 多帧, tool_calls incremental, finish_reason tool_calls
func startDeepseekStreamMock(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer must implement http.Flusher")
			return
		}
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}

		// 1. 首帧: role=assistant
		writeFrame(`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)

		// 2. reasoning_content 多帧 (deepseek thinking 模式)
		for _, piece := range []string{"Let me think ", "about ", "the weather ", "tools."} {
			writeFrame(fmt.Sprintf(`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"reasoning_content":%q},"finish_reason":null}]}`, piece))
		}

		// 3. tool_calls 增量 (与 OpenAI/deepseek 一致: 第 1 帧带 id+name, 后续帧只带 arguments)
		writeFrame(`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_xyz123","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`)
		for _, fragment := range []string{`{"city`, `":"Beijing"`, `}`} {
			writeFrame(fmt.Sprintf(`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":%q}}]},"finish_reason":null}]}`, fragment))
		}

		// 4. finish_reason 帧 (deepseek 在 tool_calls 完成后会发 finish_reason="tool_calls")
		writeFrame(`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":20,"total_tokens":70}}`)

		// 5. SSE 终止
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	return srv
}

// startDeepseekNonStreamMock 启动一个非流式 mock 上游, 返回完整 chat completion JSON,
// 包含 tool_calls + reasoning_content + finish_reason="tool_calls"。
//
// 关键词: deepseek mock JSON, 非流式 tool_calls, 完整 message
func startDeepseekNonStreamMock(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		t.Logf("upstream non-stream got body: %s", body)
		// 验证一下客户端没把 stream 拍成 true 给上游 (留作 sanity)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
  "id": "chatcmpl-non-stream",
  "object": "chat.completion",
  "created": 1717000000,
  "model": "deepseek-v4-pro",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "reasoning_content": "I should call get_weather for Beijing.",
        "content": "",
        "tool_calls": [
          {
            "id": "call_xyz123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"city\":\"Beijing\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {"prompt_tokens": 50, "completion_tokens": 20, "total_tokens": 70}
}`))
	}))
	return srv
}

// setupDeepseekServerCfg 构造一个最小可用的 ServerConfig:
//   - 一个 key (test-key), 允许 model "deepseek-v4-pro"
//   - 一个 provider 指向 mock upstream URL
//   - DbProvider 设为健康 + 低延迟, 让 PeekOrderedProvidersWithAffinity 通过
//
// 关键词: aibalance 测试 ServerConfig 构造, deepseek provider 注入
func setupDeepseekServerCfg(t *testing.T, upstreamURL, modelName string) *ServerConfig {
	t.Helper()
	cfg := NewServerConfig()

	apiKey := "test-key"
	cfg.Keys.keys[apiKey] = &Key{
		Key:           apiKey,
		AllowedModels: map[string]bool{modelName: true},
	}
	cfg.KeyAllowedModels.allowedModels[apiKey] = map[string]bool{modelName: true}

	provider := &Provider{
		ModelName:   modelName,
		TypeName:    "deepseek",
		DomainOrURL: upstreamURL,
		APIKey:      "upstream-key",
		WrapperName: modelName,
		NoHTTPS:     true,
		DbProvider: &schema.AiProvider{
			WrapperName: modelName,
			ModelName:   modelName,
			TypeName:    "deepseek",
			DomainOrURL: upstreamURL,
			APIKey:      "upstream-key",
			IsHealthy:   true,
			LastLatency: 100,
			// 这些测试模拟的是"上游 native 支持 OpenAI tool_calls 协议"的场景,
			// 在 capability matrix v1 设计里, 这相当于运维已通过 probe 按钮
			// 把该 provider 标记为 native, 因此走透传不走 react 降级.
			// 关键词: setupDeepseekServerCfg native mode, capability matrix mock
			ToolCallsRound1Mode: "native",
			ToolCallsRound2Mode: "native",
		},
	}
	cfg.Models.models[modelName] = []*Provider{provider}
	cfg.Entrypoints.providers[modelName] = []*Provider{provider}
	return cfg
}

// readClientResponse 从 client side 的 net.Pipe 读取所有数据直到对端关闭或超时。
// 关键词: 测试读 SSE 直到关闭
func readClientResponse(t *testing.T, client net.Conn, max time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(max)
	_ = client.SetReadDeadline(deadline)
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		if time.Now().After(deadline) {
			break
		}
		n, err := client.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.String()
}

// parseSSEDataChunks 把 chunked transfer encoding + SSE 混合的字节流拆成
// 一组 data: ... 后面的 JSON 字符串 (或 [DONE] 字面量)。
//
// 关键词: 测试解析 chunked SSE, data 分帧
func parseSSEDataChunks(t *testing.T, body string) []string {
	t.Helper()

	// 先去掉 HTTP header
	idx := strings.Index(body, "\r\n\r\n")
	require.GreaterOrEqual(t, idx, 0, "must contain HTTP header terminator, body=%s", body)
	rest := body[idx+4:]

	// chunked transfer 解码: 每帧 hex\r\npayload\r\n
	var sseRaw bytes.Buffer
	cursor := 0
	for cursor < len(rest) {
		newline := strings.Index(rest[cursor:], "\r\n")
		if newline < 0 {
			break
		}
		sizeStr := strings.TrimSpace(rest[cursor : cursor+newline])
		if sizeStr == "" {
			cursor += newline + 2
			continue
		}
		size := 0
		_, perr := fmt.Sscanf(sizeStr, "%x", &size)
		if perr != nil || size <= 0 {
			break
		}
		cursor += newline + 2
		if cursor+size > len(rest) {
			break
		}
		sseRaw.Write([]byte(rest[cursor : cursor+size]))
		cursor += size + 2 // skip trailing \r\n
	}

	// 拆 data: ... 行
	scanner := bufio.NewScanner(&sseRaw)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	var out []string
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		out = append(out, payload)
	}
	return out
}

// driveServeRequest 在 net.Pipe 上构造一次 chat completions 请求, 把 server 端
// 交给 cfg.Serve, 在 client 端写完请求后读取所有响应。
// 关键词: 测试驱动 serveChatCompletions 全链路
func driveServeRequest(t *testing.T, cfg *ServerConfig, body string, headers map[string]string) string {
	t.Helper()
	client, server := net.Pipe()
	defer client.Close()
	var srvWg sync.WaitGroup
	srvWg.Add(1)
	go func() {
		defer srvWg.Done()
		defer server.Close()
		cfg.Serve(server)
	}()

	headerLines := []string{
		"POST /v1/chat/completions HTTP/1.1",
		"Host: localhost",
		"Authorization: Bearer test-key",
		"Content-Type: application/json",
		fmt.Sprintf("Content-Length: %d", len(body)),
	}
	for k, v := range headers {
		headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, v))
	}
	req := strings.Join(headerLines, "\r\n") + "\r\n\r\n" + body
	_, err := client.Write([]byte(req))
	require.NoError(t, err, "write request failed")

	resp := readClientResponse(t, client, 15*time.Second)
	srvWg.Wait()
	return resp
}

// ============================================================
// 测试 1: 流式 + thinking + tool_calls
// 关键词: deepseek v4 pro 流式 finish_reason tool_calls 端到端
// ============================================================

func TestServeChatCompletions_DeepseekThinkingToolCalls_Stream(t *testing.T) {
	upstream := startDeepseekStreamMock(t)
	defer upstream.Close()

	cfg := setupDeepseekServerCfg(t, upstream.URL, "deepseek-v4-pro")
	defer cfg.Close()

	body := `{
  "model": "deepseek-v4-pro",
  "stream": true,
  "messages": [{"role": "user", "content": "What's the weather in Beijing?"}],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"]
        }
      }
    }
  ]
}`
	resp := driveServeRequest(t, cfg, body, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks, "must receive SSE chunks, raw resp:\n%s", resp)

	// 收集结构化片段
	var (
		reasoningTotal      strings.Builder
		toolCallsAccum      = map[int]*toolCallAccum{}
		seenDONE            bool
		finalFinishReason   string
		gotToolCallsInDelta bool
	)
	for _, payload := range chunks {
		if payload == "[DONE]" {
			seenDONE = true
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]any)
		if first == nil {
			continue
		}
		if fr, ok := first["finish_reason"].(string); ok && fr != "" {
			finalFinishReason = fr
		}
		delta, _ := first["delta"].(map[string]any)
		if rc, ok := delta["reasoning_content"].(string); ok && rc != "" {
			reasoningTotal.WriteString(rc)
		}
		if tc, ok := delta["tool_calls"].([]any); ok && len(tc) > 0 {
			gotToolCallsInDelta = true
			for _, raw := range tc {
				m, _ := raw.(map[string]any)
				if m == nil {
					continue
				}
				idxF, _ := m["index"].(float64)
				idx := int(idxF)
				accum, ok := toolCallsAccum[idx]
				if !ok {
					accum = &toolCallAccum{Index: idx}
					toolCallsAccum[idx] = accum
				}
				if id, ok := m["id"].(string); ok && id != "" {
					accum.ID = id
				}
				if typ, ok := m["type"].(string); ok && typ != "" {
					accum.Type = typ
				}
				if fnRaw, ok := m["function"].(map[string]any); ok {
					if name, ok := fnRaw["name"].(string); ok && name != "" {
						accum.Name = name
					}
					if args, ok := fnRaw["arguments"].(string); ok {
						accum.Arguments += args
					}
				}
			}
		}
	}

	// 验证 1: reasoning_content 必须被原样转发
	assert.Equal(t, "Let me think about the weather tools.",
		reasoningTotal.String(),
		"reasoning_content should be forwarded byte-by-byte to client")

	// 验证 2: 至少应该有 tool_calls delta 出现
	assert.True(t, gotToolCallsInDelta,
		"client must see at least one delta.tool_calls chunk")

	// 验证 3: 客户端应当能累积出完整的 tool_call (id/type/name/arguments)
	require.Len(t, toolCallsAccum, 1, "expect exactly 1 tool call accumulated, got: %+v", toolCallsAccum)
	tc := toolCallsAccum[0]
	assert.Equal(t, "call_xyz123", tc.ID, "tool call id should be preserved")
	assert.Equal(t, "function", tc.Type, "tool call type should be preserved")
	assert.Equal(t, "get_weather", tc.Name, "tool call function name should be preserved")
	assert.Equal(t, `{"city":"Beijing"}`, tc.Arguments,
		"tool call arguments should be reassembled from incremental chunks")

	// 验证 4: 末尾 finish_reason 必须是 "tool_calls" (CURRENT BUG: aibalance 写死 "stop")
	assert.Equal(t, "tool_calls", finalFinishReason,
		"finish_reason MUST be 'tool_calls' so OpenAI-compatible clients trigger tool execution")

	// 验证 5: 必须以 [DONE] 收尾
	assert.True(t, seenDONE, "stream should terminate with data: [DONE]")
}

type toolCallAccum struct {
	Index     int
	ID        string
	Type      string
	Name      string
	Arguments string
}

// ============================================================
// 测试 2: 非流式 + tool_calls
// 关键词: deepseek 非流式 tool_calls 不丢失
// ============================================================

func TestServeChatCompletions_DeepseekToolCalls_NonStream(t *testing.T) {
	upstream := startDeepseekNonStreamMock(t)
	defer upstream.Close()

	cfg := setupDeepseekServerCfg(t, upstream.URL, "deepseek-v4-pro")
	defer cfg.Close()

	body := `{
  "model": "deepseek-v4-pro",
  "stream": false,
  "messages": [{"role": "user", "content": "What's the weather in Beijing?"}],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"]
        }
      }
    }
  ]
}`
	resp := driveServeRequest(t, cfg, body, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

	// 非流式响应使用 Content-Length + 一次性 body, 不再用 chunked transfer.
	// 这样上游 nginx 反代到 HTTP/2 时不会出现 stream not closed cleanly 的问题。
	// 关键词: 非流式 Content-Length 响应解析
	idx := strings.Index(resp, "\r\n\r\n")
	require.GreaterOrEqual(t, idx, 0)
	headerPart := resp[:idx]
	body2 := resp[idx+4:]

	// 必须含 application/json header, 防止退回 SSE
	assert.True(t, strings.Contains(strings.ToLower(headerPart), "content-type: application/json"),
		"non-stream response must use application/json content-type, header=%s", headerPart)
	assert.True(t, strings.Contains(strings.ToLower(headerPart), "content-length:"),
		"non-stream response must declare Content-Length, header=%s", headerPart)
	assert.False(t, strings.Contains(strings.ToLower(headerPart), "transfer-encoding: chunked"),
		"non-stream response MUST NOT use chunked transfer encoding (HTTP/2 INTERNAL_ERROR risk)")

	respBody := bytes.TrimSpace([]byte(body2))
	require.NotEmpty(t, respBody, "non-stream body should not be empty, raw=%s", resp)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(respBody, &parsed),
		"non-stream body should be valid JSON, body=%s", respBody)

	choices, _ := parsed["choices"].([]any)
	require.Len(t, choices, 1, "expect 1 choice in non-stream response")
	first, _ := choices[0].(map[string]any)
	require.NotNil(t, first)
	msg, _ := first["message"].(map[string]any)
	require.NotNil(t, msg, "message must exist in non-stream response")

	// 验证 1: reasoning_content 必须被透传
	assert.Equal(t, "I should call get_weather for Beijing.",
		msg["reasoning_content"], "reasoning_content should be forwarded in non-stream")

	// 验证 2: tool_calls 必须存在 (CURRENT BUG: GetNotStreamBody 仅含 content/reasoning_content, tool_calls 完全丢失)
	tcRaw, hasToolCalls := msg["tool_calls"]
	assert.True(t, hasToolCalls, "non-stream message MUST include tool_calls, got message=%v", msg)
	if hasToolCalls {
		tcList, _ := tcRaw.([]any)
		require.Len(t, tcList, 1, "expect 1 tool call in non-stream response")
		tc, _ := tcList[0].(map[string]any)
		require.NotNil(t, tc)
		assert.Equal(t, "call_xyz123", tc["id"])
		assert.Equal(t, "function", tc["type"])
		fn, _ := tc["function"].(map[string]any)
		require.NotNil(t, fn)
		assert.Equal(t, "get_weather", fn["name"])
		assert.Equal(t, `{"city":"Beijing"}`, fn["arguments"])
	}

	// 验证 3: finish_reason 必须是 "tool_calls" (CURRENT BUG: 写死 "stop")
	assert.Equal(t, "tool_calls", first["finish_reason"],
		"non-stream finish_reason must be 'tool_calls' for tool-call responses")
}

// ============================================================
// 测试 3: 仅文本 (无 tool_calls) 时, finish_reason 应当保持 "stop"
// 防止「修了 tool_calls 又把无 tool_calls 的回归打坏」。
// 关键词: 防回归 finish_reason stop, 仅文本不应当被改成 tool_calls
// ============================================================

func TestServeChatCompletions_DeepseekTextOnly_FinishReasonStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(2 * time.Millisecond)
		}
		writeFrame(`{"id":"1","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`)
		writeFrame(`{"id":"1","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`)
		writeFrame(`{"id":"1","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	cfg := setupDeepseekServerCfg(t, srv.URL, "deepseek-v4-pro")
	defer cfg.Close()

	body := `{"model":"deepseek-v4-pro","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	resp := driveServeRequest(t, cfg, body, nil)
	chunks := parseSSEDataChunks(t, resp)

	var lastFR string
	for _, payload := range chunks {
		if payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]any)
		if fr, ok := first["finish_reason"].(string); ok && fr != "" {
			lastFR = fr
		}
	}
	assert.Equal(t, "stop", lastFR,
		"text-only response must keep finish_reason='stop', regression guard for tool_calls fix")
}

// ============================================================
// 测试 4 (writer 单元): 多 tool_calls (index 0 + index 1) 增量交错时,
// chatJSONChunkWriter 内部 accumulatedToolCalls 必须按 index 各自累积、
// 不串号、不丢失,GetNotStreamBody 输出的 tool_calls 数组必须完整。
//
// 直接调用 writer 公共 API 而不经过 server / 上游 deepseek 客户端,
// 这样可以独立验证 accumulator 的隔离性,与 net.Pipe 流控异步无关。
//
// 关键词: 多 tool_calls 并行 index 隔离, accumulatedToolCalls 单元测试,
// 非流式 tool_calls 完整还原
// ============================================================

func TestChatJSONChunkWriter_AccumulateParallelToolCalls_IndexIsolation(t *testing.T) {
	// 使用 io.Discard 包装的 nopCloser 充当下游连接, 因为本测试不关心
	// 流式 SSE 的字节,只关心 GetNotStreamBody 输出的最终 JSON。
	w := NewChatJSONChunkWriterEx(nopWriteCloser{io.Discard}, "uid-x", "deepseek-v4-pro", true)
	defer w.Close()

	mk := func(idx int, id, name, args string) *aispec.ToolCall {
		tc := &aispec.ToolCall{
			Index: idx,
			ID:    id,
		}
		if id != "" || name != "" {
			tc.Type = "function"
		}
		tc.Function.Name = name
		tc.Function.Arguments = args
		return tc
	}

	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(0, "call_0", "get_weather", "")}))
	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(1, "call_1", "get_time", "")}))
	// 刻意交错 arguments 增量,验证按 index 隔离累积
	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(1, "", "", `{"tz`)}))
	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(0, "", "", `{"city`)}))
	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(1, "", "", `":"UTC"}`)}))
	require.NoError(t, w.WriteToolCalls([]*aispec.ToolCall{mk(0, "", "", `":"Beijing"}`)}))

	body := w.GetNotStreamBody()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed),
		"GetNotStreamBody must produce valid JSON, body=%s", body)

	choices, _ := parsed["choices"].([]any)
	require.Len(t, choices, 1)
	first, _ := choices[0].(map[string]any)
	require.NotNil(t, first)
	assert.Equal(t, "tool_calls", first["finish_reason"],
		"finish_reason must switch to 'tool_calls' when accumulator non-empty")

	msg, _ := first["message"].(map[string]any)
	require.NotNil(t, msg)
	tcRaw, ok := msg["tool_calls"].([]any)
	require.True(t, ok, "tool_calls field must exist in message, got message=%v", msg)
	require.Len(t, tcRaw, 2, "expect exactly 2 parallel tool calls, got %+v", tcRaw)

	tc0, _ := tcRaw[0].(map[string]any)
	require.NotNil(t, tc0)
	assert.Equal(t, "call_0", tc0["id"], "index 0 id must be call_0, no cross-talk with index 1")
	fn0, _ := tc0["function"].(map[string]any)
	require.NotNil(t, fn0)
	assert.Equal(t, "get_weather", fn0["name"])
	assert.Equal(t, `{"city":"Beijing"}`, fn0["arguments"],
		"index 0 arguments must reassemble to Beijing payload, no fragments leaked from index 1")

	tc1, _ := tcRaw[1].(map[string]any)
	require.NotNil(t, tc1)
	assert.Equal(t, "call_1", tc1["id"], "index 1 id must be call_1")
	fn1, _ := tc1["function"].(map[string]any)
	require.NotNil(t, fn1)
	assert.Equal(t, "get_time", fn1["name"])
	assert.Equal(t, `{"tz":"UTC"}`, fn1["arguments"],
		"index 1 arguments must reassemble to UTC payload, no fragments leaked from index 0")
}

// nopWriteCloser 把 io.Writer 包成 io.WriteCloser, 仅用于 writer 单元测试。
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// ============================================================
// 测试 5: tool round-trip 第二轮 (回灌 tool result -> 模型 NL 回答)
// 模拟 OpenAI Python SDK / langchain / litellm / codex 真实链路:
//   round1: client -> aibalance -> upstream, upstream 返回 tool_calls
//   round2: client 把 tool 结果回灌给 aibalance, 上游模型给出最终 NL 回答
//
// 旧实现真实链路在这个 round2 出现 "上游正常回了 content,
// 但 aibalance 返回客户端的 SSE 是空" 的现象。该测试 mock 一个上游,
// 上游对 round2 (识别出消息里包含 role=tool 的消息) 返回正常 content
// + finish_reason=stop, 然后断言 aibalance 透传给客户端的 SSE 也含 content。
//
// 关键词: tool round-trip 第二轮, role=tool 回灌, OpenAI SDK 真实闭环
// ============================================================

func TestServeChatCompletions_DeepseekToolRoundTrip_Round2NotEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// 上游 mock 必须看到 round2 的 messages: 应包含 role=tool 与 tool_calls 字段
		hasTool := bytes.Contains(body, []byte(`"role":"tool"`))
		hasToolCalls := bytes.Contains(body, []byte(`"tool_calls"`))
		if !hasTool || !hasToolCalls {
			t.Errorf("upstream did not see round2 fields: hasTool=%v hasToolCalls=%v body=%s",
				hasTool, hasToolCalls, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(2 * time.Millisecond)
		}
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		for _, piece := range []string{"Beijing ", "is currently ", "sunny ", "with 21C."} {
			writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, piece))
		}
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek-v4-pro","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	cfg := setupDeepseekServerCfg(t, srv.URL, "deepseek-v4-pro")
	defer cfg.Close()

	body := `{
  "model": "deepseek-v4-pro",
  "stream": true,
  "messages": [
    {"role": "user", "content": "What's the weather in Beijing?"},
    {"role": "assistant", "content": "", "tool_calls": [
      {"id": "call_xyz", "type": "function", "function": {"name": "get_current_weather", "arguments": "{\"city\":\"Beijing\"}"}}
    ]},
    {"role": "tool", "tool_call_id": "call_xyz", "name": "get_current_weather", "content": "{\"temperature_c\":21,\"condition\":\"sunny\"}"}
  ]
}`
	resp := driveServeRequest(t, cfg, body, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks, "must receive SSE chunks, raw resp:\n%s", resp)

	var contentTotal strings.Builder
	var lastFR string
	for _, payload := range chunks {
		if payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]any)
		if fr, ok := first["finish_reason"].(string); ok && fr != "" {
			lastFR = fr
		}
		delta, _ := first["delta"].(map[string]any)
		if c, ok := delta["content"].(string); ok && c != "" {
			contentTotal.WriteString(c)
		}
	}

	assert.Equal(t, "Beijing is currently sunny with 21C.", contentTotal.String(),
		"round2 content must be forwarded to client byte-by-byte, "+
			"not silently dropped (regression of 'aibalance round2 empty response')")
	assert.Equal(t, "stop", lastFR,
		"round2 finish_reason must be 'stop' since model returned NL answer")
}
