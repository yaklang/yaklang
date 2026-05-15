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

	"github.com/yaklang/yaklang/common/schema"
)

// round2_react_response_extract_test.go 覆盖 bug:
//
//   当客户端发起 round2 (messages 含 assistant.tool_calls / role=tool) 且
//   provider 处于 ReAct mode (DB ToolCallsRound2Mode="react" 或 AutoFallback
//   命中) 时, server.go 仅做了 messages flatten, 但是:
//     1. 没有把 client 携带的 tools=[...] 转成 ReAct system prompt;
//     2. 没有清空 toolsForUpstream / toolChoiceForUpstream;
//     3. 没有在 writer 上调用 EnableReactExtractor.
//
//   后果: 上游 wrapper 收到的 messages 是 ReAct 文本, 历史里全是
//   `[tool_call ...]...[/tool_call]` 文本, 因此上游模型会继续以同样的
//   ReAct 文本格式输出新的工具调用; 但 writer 没启用 extractor, 这些
//   `[tool_call ...]...[/tool_call]` 文本被原样作为 content 透传给客户端,
//   opencode / litellm / Vercel AI SDK 等 OpenAI 兼容客户端无法触发工具执行,
//   表现为 "AI 输出了 tool_calls 但客户端不执行, 显示成 markdown 文本".
//
// 关键词: round2 react 响应侧 extractor 缺失, opencode 工具不执行, content 误识别

// reactRound2Upstream 模拟一个 hostile wrapper:
//   - 看到 OpenAI tool_calls 字段 -> 拒绝处理, finish_reason=stop 空回
//   - 看到 ReAct 文本 messages -> 用 ReAct 文本格式返回新的 tool_calls
//     (单条 / 并行多条都覆盖)
//
// 这复刻了 z-deepseek-v4-pro 等线上 wrapper 的真实行为: 它们在 round-trip
// 场景下只能消费纯文本对话, 也会用纯文本输出新一轮工具调用.
//
// parallelCount: 模拟模型一次响应里输出几个并行 tool_call (1 表示单工具,
// >=2 表示并行多工具, 复现用户截图里 5 个并行 bash 调用的场景).
//
// 关键词: round2 react mock upstream, 并行多 tool_call 响应
func reactRound2Upstream(t *testing.T, parallelCount int) (string, func() []byte, func()) {
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

		// 检测上游收到的 body 里是否含 OpenAI tool_calls round-trip 标记
		hasOpenAIToolCalls := bytes.Contains(body, []byte(`"tool_calls"`)) ||
			bytes.Contains(body, []byte(`"role":"tool"`))
		hasNativeToolsField := bytes.Contains(body, []byte(`"tools":[`))

		if hasOpenAIToolCalls || hasNativeToolsField {
			// hostile: 看见 OpenAI tool_calls 或者 tools 字段就空回
			writeFrame(`{"id":"empty","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		// ReAct 文本路径: 输出新一轮 tool_call(s)
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`,
			"Let me run more checks.\n"))

		for i := 0; i < parallelCount; i++ {
			text := fmt.Sprintf(`[tool_call id="call_model_%02d_%s" name="bash"]{"command":"echo step-%d"}[/tool_call]`,
				i+1, "abcdef0123456789", i+1)
			if i+1 < parallelCount {
				text += "\n"
			}
			writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, text))
		}

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

// setupReactRound2Cfg 构造一个 Provider, DB 显式声明
// ToolCallsRound1Mode/Round2Mode = "react", 强制 ReAct 模式.
// 使用与 setupDeepseekServerCfg 同样的 API key "test-key", 复用
// driveServeRequest 默认 Authorization header.
// 关键词: react mode provider 构造
func setupReactRound2Cfg(t *testing.T, upstreamURL, modelName string) *ServerConfig {
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
			// 关键: round1/round2 都强制走 react, 模拟运维已经 probe 出
			// 该 wrapper 不识别 OpenAI tool_calls 协议.
			// 关键词: react mode 强制配置
			ToolCallsRound1Mode: "react",
			ToolCallsRound2Mode: "react",
		},
	}
	cfg.Models.models[modelName] = []*Provider{provider}
	cfg.Entrypoints.providers[modelName] = []*Provider{provider}
	return cfg
}

// round2RequestBodyWithTools 是模拟 opencode round2 真实请求体: messages
// 含 assistant.tool_calls + role=tool, 同时也保留 client 端 tools=[...].
// 关键词: opencode round2 真实请求体, tools 仍存在
const round2RequestBodyWithTools = `{
  "model": "deepseek-v4-pro",
  "stream": true,
  "messages": [
    {"role": "system", "content": "You are a security assistant."},
    {"role": "user", "content": "Please probe http://target/ for SQLi."},
    {"role": "assistant", "content": "", "tool_calls": [
      {"id": "call_xyz", "type": "function", "function": {"name": "bash", "arguments": "{\"command\":\"curl http://target/\"}"}}
    ]},
    {"role": "tool", "tool_call_id": "call_xyz", "name": "bash", "content": "{\"stdout\":\"index page\"}"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "bash",
        "description": "Run a bash command",
        "parameters": {
          "type": "object",
          "properties": {"command": {"type": "string"}},
          "required": ["command"]
        }
      }
    }
  ]
}`

// TestRound2ReactMode_SingleToolCall_ParsedAsToolCallsNotContent
// 验证 BUG: round2 ReAct mode 下, 上游用 ReAct 文本格式返回单个 tool_call,
// 客户端必须看到 OpenAI tool_calls delta (而不是把 [tool_call ...] 文本
// 当 content 透传).
//
// CURRENT BUG: server.go 在 round2 react flatten 路径上没有调用
// writer.EnableReactExtractor, 因此上游回吐的 `[tool_call ...]...[/tool_call]`
// 文本被原样当 content 发给客户端 -> opencode 无法触发 bash 执行.
//
// 关键词: round2 react 单 tool_call 解析, content 不应包含 tool_call 文本
func TestRound2ReactMode_SingleToolCall_ParsedAsToolCallsNotContent(t *testing.T) {
	srvURL, getRaw, closeFn := reactRound2Upstream(t, 1)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

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

	// 上游收到的请求必须是 ReAct 文本风格, 不能含 OpenAI tool_calls 字段
	// 也不能含 tools=[...] 原生字段 (那样上游会立即空回).
	// 关键词: 上游请求 ReAct 文本, tools 字段已隐藏
	raw := getRaw()
	assert.NotContains(t, string(raw), `"tool_calls"`,
		"round2 react: upstream must not see OpenAI tool_calls field (would trigger hostile empty response)")
	assert.NotContains(t, string(raw), `"role":"tool"`,
		"round2 react: upstream must not see role=tool message")
	assert.NotContains(t, string(raw), `"tools":[{`,
		"round2 react: upstream must not see native tools=[{...}] field, must be injected as system prompt instead")
	assert.Contains(t, string(raw), "[tool_call",
		"round2 react: upstream should see ReAct text history with [tool_call ...]")

	// 客户端必须看到 OpenAI tool_calls delta, 而不是 [tool_call ...] 文本当 content
	require.Len(t, toolCalls, 1,
		"client must accumulate exactly 1 tool_call from delta.tool_calls, got: %+v, content=%q",
		toolCalls, contentTotal.String())
	tc := toolCalls[0]
	assert.Equal(t, "bash", tc.Name, "tool name extracted from upstream ReAct text")
	assert.Equal(t, "function", tc.Type)
	assert.NotEmpty(t, tc.ID, "tool_call id should be non-empty (extractor generates call_react_N)")
	assert.Equal(t, `{"command":"echo step-1"}`, tc.Arguments,
		"tool arguments must be parsed from upstream ReAct text and forwarded as JSON string")

	// 客户端 content 流不应包含 [tool_call ...] 这种 ReAct 文本残骸.
	// 允许包含模型的前导思考文本 ("Let me run more checks.").
	// 关键词: content 不应包含 tool_call 文本残骸
	assert.NotContains(t, contentTotal.String(), "[tool_call",
		"client content stream must not contain [tool_call ...] text (bug: extractor not enabled in round2)")
	assert.NotContains(t, contentTotal.String(), "[/tool_call]",
		"client content stream must not contain [/tool_call] text")

	// finish_reason 必须切换为 tool_calls
	assert.Equal(t, "tool_calls", lastFR,
		"round2 react with tool_call response: finish_reason must be tool_calls")
}

// TestRound2ReactMode_ParallelToolCalls_AllParsed
// 验证 BUG + 并行多工具场景: 上游一次响应里输出 5 个 [tool_call ...] 块
// (用户截图里的 "5 个并行 bash" 场景), 客户端必须能累积出 5 个独立 tool_call,
// 每个有自己的 index/id, 不能出现串号 / 漏帧 / 文本残骸.
//
// 关键词: round2 react 并行 5 个 tool_call, 全部成功透传给客户端,
//        opencode 并行工具执行修复
func TestRound2ReactMode_ParallelToolCalls_AllParsed(t *testing.T) {
	const parallelN = 5
	srvURL, getRaw, closeFn := reactRound2Upstream(t, parallelN)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks)

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
	_ = getRaw

	// 必须解析出 5 个并行 tool_call, 每个 index 唯一, args 不互相串
	require.Len(t, toolCalls, parallelN,
		"parallel %d tool_calls must be all accumulated, got: %+v, content=%q",
		parallelN, toolCalls, contentTotal.String())

	for i := 0; i < parallelN; i++ {
		tc, ok := toolCalls[i]
		require.True(t, ok, "must have tool_call at index %d", i)
		assert.Equal(t, "bash", tc.Name, "tool_call[%d] name", i)
		assert.NotEmpty(t, tc.ID, "tool_call[%d] id must be non-empty", i)
		assert.Equal(t, fmt.Sprintf(`{"command":"echo step-%d"}`, i+1), tc.Arguments,
			"tool_call[%d] arguments must not be cross-contaminated", i)
	}

	// 各 tool_call id 必须不同
	ids := map[string]int{}
	for i, tc := range toolCalls {
		ids[tc.ID]++
		_ = i
	}
	assert.Len(t, ids, parallelN, "each tool_call must have a unique id, got duplicates: %v", ids)

	// 5 个并行 tool_call 全部解析后, content 不能残留任何 [tool_call ...] 文本
	assert.NotContains(t, contentTotal.String(), "[tool_call",
		"after parsing all parallel tool_calls, content must not contain [tool_call ...] residue")
	assert.NotContains(t, contentTotal.String(), "[/tool_call]")

	assert.Equal(t, "tool_calls", lastFR,
		"parallel tool_calls response: finish_reason must be tool_calls")
}

// TestRound2AutoFallback_ToolCallsExtracted 验证 unknown provider 配合
// AutoFallback 兜底机制: DB 未填 ToolCallsRound2Mode 时, AutoFallback=true,
// server.go 应当自动启用 react flatten + extractor, 与显式声明 react mode
// 行为一致.
//
// 关键词: AutoFallback round2 react 一致性, unknown provider 兜底
func TestRound2AutoFallback_ToolCallsExtracted(t *testing.T) {
	srvURL, _, closeFn := reactRound2Upstream(t, 2)
	defer closeFn()

	cfg := NewServerConfig()
	defer cfg.Close()

	const modelName = "deepseek-v4-pro"
	const apiKey = "test-key"
	cfg.Keys.keys[apiKey] = &Key{
		Key:           apiKey,
		AllowedModels: map[string]bool{modelName: true},
	}
	cfg.KeyAllowedModels.allowedModels[apiKey] = map[string]bool{modelName: true}

	provider := &Provider{
		ModelName:   modelName,
		TypeName:    "deepseek",
		DomainOrURL: srvURL,
		APIKey:      "upstream-key",
		WrapperName: modelName,
		NoHTTPS:     true,
		DbProvider: &schema.AiProvider{
			WrapperName: modelName,
			ModelName:   modelName,
			TypeName:    "deepseek",
			DomainOrURL: srvURL,
			APIKey:      "upstream-key",
			IsHealthy:   true,
			LastLatency: 100,
			// DB 字段留空 -> ResolveToolCallsMode 返回 default + AutoFallback=true
			// 关键词: unknown provider AutoFallback 默认值
		},
	}
	cfg.Models.models[modelName] = []*Provider{provider}
	cfg.Entrypoints.providers[modelName] = []*Provider{provider}

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"))

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks)

	toolCalls := map[int]*toolCallAccum{}
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
		delta, _ := first["delta"].(map[string]any)
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

	// AutoFallback 命中后, round2 必须走 react 路径 -> 客户端应该看到
	// 提取出来的 OpenAI tool_calls (而不是 [tool_call ...] 文本).
	require.Len(t, toolCalls, 2,
		"AutoFallback round2: expect 2 parallel tool_calls extracted, got: %+v", toolCalls)
	assert.Equal(t, "bash", toolCalls[0].Name)
	assert.Equal(t, "bash", toolCalls[1].Name)
	assert.Equal(t, `{"command":"echo step-1"}`, toolCalls[0].Arguments)
	assert.Equal(t, `{"command":"echo step-2"}`, toolCalls[1].Arguments)
}
