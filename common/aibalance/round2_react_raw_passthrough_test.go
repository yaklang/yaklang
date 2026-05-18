package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// round2_react_raw_passthrough_test.go 覆盖 v2 raw passthrough 引入的 5 种新
// variant 在端到端 server -> upstream -> client SSE 流上的行为. 验证 v2 plan
// 验收标准:
//
//	"上游回的 Anthropic XML / 中文 [调用 X] / Hermes / DeepSeek 全角 / Mistral 任意一种,
//	 客户端的 SSE 流里都能看到 delta.tool_calls[].function.name + 一段 arguments 字符串
//	 (即使 arguments 不是 JSON)."
//
// 跟 round2_react_response_extract_test.go 的差异: 那里的 mock upstream 固定回吐
// canonical [tool_call ...] 形态, 这里 mock upstream 自由配置 hallucinate 内容,
// 模拟 z-deepseek-v4-pro-free 这类 wrapper 在 react 模式下输出各种漂移格式的真实场景.
//
// 关键词: v2 raw passthrough e2e, 5 variants server 透传, opencode 截图修复

// rawPassthroughUpstream 构造一个 mock upstream, 在 react messages 路径下回吐
// 由 caller 指定的 hallucinate content. 走 native tool_calls / tools 路径时
// 模拟 hostile wrapper 空回 (复用 reactRound2Upstream 的策略).
//
// 关键词: rawPassthroughUpstream, hallucinate content 自由配置
func rawPassthroughUpstream(t *testing.T, hallucinateContent string) (string, func() []byte, func()) {
	t.Helper()
	var (
		mu     sync.Mutex
		gotRaw []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotRaw = append([]byte(nil), body...)
		mu.Unlock()

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

		hasOpenAIToolCalls := bytes.Contains(body, []byte(`"tool_calls"`)) ||
			bytes.Contains(body, []byte(`"role":"tool"`))
		hasNativeToolsField := bytes.Contains(body, []byte(`"tools":[`))

		if hasOpenAIToolCalls || hasNativeToolsField {
			writeFrame(`{"id":"empty","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		// react 文本路径: 输出 hallucinate content
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, hallucinateContent))
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":40,"total_tokens":90}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	get := func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return append([]byte(nil), gotRaw...)
	}
	return srv.URL, get, srv.Close
}

// collectToolCallsFromSSE 从 server 回的 SSE 响应里累积 tool_calls.
// 复用 round2_react_response_extract_test.go 的累积逻辑.
//
// 关键词: collectToolCallsFromSSE, e2e SSE 流 tool_calls 累积 helper
func collectToolCallsFromSSE(t *testing.T, resp string) (map[int]*toolCallAccum, string, string) {
	t.Helper()
	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks, "must receive SSE chunks")
	var (
		contentTotal strings.Builder
		toolCalls    = map[int]*toolCallAccum{}
		lastFR       string
	)
	for _, payload := range chunks {
		if payload == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(payload), &chunk) != nil {
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
		if tcArr, ok := delta["tool_calls"].([]any); ok {
			for _, raw := range tcArr {
				m, _ := raw.(map[string]any)
				if m == nil {
					continue
				}
				idxF, _ := m["index"].(float64)
				idx := int(idxF)
				accum, ok := toolCalls[idx]
				if !ok {
					accum = &toolCallAccum{Index: idx}
					toolCalls[idx] = accum
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
	return toolCalls, contentTotal.String(), lastFR
}

// ============================================================================
// E2E 1: Anthropic XML parameter (用户截图1)
// ============================================================================

// TestServer_RawPassthrough_AnthropicXMLParameter
// 用户截图1 端到端复现: 上游 (z-deepseek-v4-pro-free 模拟) 回吐 anthropic XML
// parameter 嵌套形态, 客户端必须收到一个 OpenAI tool_calls delta, name=bash,
// arguments=含 <parameter> XML 的原文.
//
// 关键词: 用户截图1 e2e, anthropic xml param 透传给客户端
func TestServer_RawPassthrough_AnthropicXMLParameter(t *testing.T) {
	hallucinate := `<tool_call name="bash">
<parameter name="command">curl -sI "http://192.168.3.24:18080/portal"</parameter>
<parameter name="description">Probe login portal</parameter>
<parameter name="timeout">30000</parameter>
</tool_call>`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK")

	toolCalls, content, lastFR := collectToolCallsFromSSE(t, resp)
	require.Len(t, toolCalls, 1, "anthropic XML param 必须被识别为 1 个 ToolCall, content=%q", content)
	tc := toolCalls[0]
	assert.Equal(t, "bash", tc.Name, "name 从 header attr name=\"bash\" 抠出")
	assert.Contains(t, tc.Arguments, `<parameter name="command">curl`,
		"v2: args body 原文透传, 客户端拿到 <parameter> XML 而非 JSON")
	assert.Contains(t, tc.Arguments, `<parameter name="timeout">30000</parameter>`,
		"args body 完整保留所有 <parameter> 元素")
	assert.NotContains(t, content, `<tool_call name="bash">`,
		"content 流不应残留 <tool_call> 文本 (extractor 已抠出)")
	assert.Equal(t, "tool_calls", lastFR, "finish_reason 必须切到 tool_calls")
}

// ============================================================================
// E2E 2: Chinese invoke (用户截图2)
// ============================================================================

// TestServer_RawPassthrough_ChineseInvoke
// 用户截图2 端到端复现: 上游回吐 chinese-invoke 形态 [调用 NAME] {...} [/tool_call],
// 客户端拿到 name=todowrite + arguments JSON.
//
// 关键词: 用户截图2 e2e, chinese invoke 透传给客户端
func TestServer_RawPassthrough_ChineseInvoke(t *testing.T) {
	hallucinate := `[调用 todowrite] {"todos":[{"content":"step1","status":"in_progress"},{"content":"step2","status":"pending"}]} [/tool_call]`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK")

	toolCalls, content, lastFR := collectToolCallsFromSSE(t, resp)
	require.Len(t, toolCalls, 1, "chinese invoke 必须被识别, content=%q", content)
	tc := toolCalls[0]
	assert.Equal(t, "todowrite", tc.Name, "name 从 [调用 之后取")
	args := strings.TrimSpace(tc.Arguments)
	assert.True(t, strings.HasPrefix(args, `{"todos":`), "args 原文透传, got: %q", args)
	assert.Contains(t, args, `"step1"`)
	assert.Contains(t, args, `"step2"`)
	assert.NotContains(t, content, `[调用 todowrite]`,
		"content 流不应残留 [调用 ...] 文本")
	assert.Equal(t, "tool_calls", lastFR)
}

// ============================================================================
// E2E 3: Hermes / Qwen2.5 body-name
// ============================================================================

// TestServer_RawPassthrough_HermesBody
// 上游回吐 hermes body-name 形态 <tool_call>{"name":"X","arguments":{...}}</tool_call>,
// 客户端拿到 name=bash + arguments 是 .arguments 子段 JSON (剥 wrapper 后).
//
// 关键词: hermes body name e2e, qwen2.5 兼容
func TestServer_RawPassthrough_HermesBody(t *testing.T) {
	hallucinate := `<tool_call>{"name":"bash","arguments":{"command":"ls -la","timeout":15}}</tool_call>`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK")

	toolCalls, content, lastFR := collectToolCallsFromSSE(t, resp)
	require.Len(t, toolCalls, 1, "hermes body-name 必须被识别, content=%q", content)
	tc := toolCalls[0]
	assert.Equal(t, "bash", tc.Name, "name 从 body JSON .name 字段抠出")
	// .arguments 子段被剥开成 args (恢复 native 协议语义)
	var probe map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Arguments), &probe),
		"hermes body-name 路径下 args 应该是 .arguments 子段 (合法 JSON object): %s", tc.Arguments)
	assert.Equal(t, "ls -la", probe["command"])
	assert.Equal(t, float64(15), probe["timeout"])
	assert.Equal(t, "tool_calls", lastFR)
}

// ============================================================================
// E2E 4: Mistral [TOOL_CALLS] 数组并行
// ============================================================================

// TestServer_RawPassthrough_MistralArray
// 上游回吐 mistral [TOOL_CALLS] [{...},{...},{...}] 数组形态, 客户端必须累积出 3 个
// 独立 ToolCall, index 各自正确.
//
// 关键词: mistral array e2e, 并行多 ToolCall
func TestServer_RawPassthrough_MistralArray(t *testing.T) {
	hallucinate := `[TOOL_CALLS] [` +
		`{"name":"bash","arguments":{"command":"echo a"}},` +
		`{"name":"bash","arguments":{"command":"echo b"}},` +
		`{"name":"todowrite","arguments":{"todos":[]}}` +
		`]`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK")

	toolCalls, content, lastFR := collectToolCallsFromSSE(t, resp)
	require.Len(t, toolCalls, 3, "mistral array 3 个 element 必须各自 emit, content=%q", content)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, `{"command":"echo a"}`, toolCalls[0].Arguments)
	assert.Equal(t, "bash", toolCalls[1].Name)
	assert.Equal(t, `{"command":"echo b"}`, toolCalls[1].Arguments)
	assert.Equal(t, "todowrite", toolCalls[2].Name)
	assert.Equal(t, `{"todos":[]}`, toolCalls[2].Arguments)
	assert.NotContains(t, content, `[TOOL_CALLS]`, "content 流不应残留 [TOOL_CALLS] 文本")
	assert.Equal(t, "tool_calls", lastFR)
}

// ============================================================================
// E2E 5: DeepSeek V3.1 全角分隔符并行
// ============================================================================

// TestServer_RawPassthrough_DeepseekFullwidth
// 上游回吐 deepseek-v3.1 全角分隔符形态, 含外层 calls_begin/calls_end 包裹 2 个
// sub-frame, 客户端必须累积出 2 个独立 ToolCall.
//
// 关键词: deepseek fullwidth e2e, 全角分隔符并行 ToolCall
func TestServer_RawPassthrough_DeepseekFullwidth(t *testing.T) {
	hallucinate := `<｜tool_calls_begin｜>` +
		`<｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"echo s1"}<｜tool_call_end｜>` +
		`<｜tool_call_begin｜>bash<｜tool_sep｜>{"command":"echo s2"}<｜tool_call_end｜>` +
		`<｜tool_calls_end｜>`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK")

	toolCalls, content, lastFR := collectToolCallsFromSSE(t, resp)
	require.Len(t, toolCalls, 2, "deepseek-fullwidth 2 个 sub-frame 必须各自 emit, content=%q", content)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, `{"command":"echo s1"}`, toolCalls[0].Arguments)
	assert.Equal(t, "bash", toolCalls[1].Name)
	assert.Equal(t, `{"command":"echo s2"}`, toolCalls[1].Arguments)
	assert.NotContains(t, content, `<｜tool_calls_begin｜>`,
		"content 流不应残留 deepseek 全角分隔符文本")
	assert.Equal(t, "tool_calls", lastFR)
}
