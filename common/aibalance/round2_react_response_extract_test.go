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

// TestRound2ReactMode_ParallelToolCallsInOneContentChunk
// 复刻用户截图里 "多 tool_call 样式全炸" 的真实场景: 上游模型在一个
// content 流块里一次性吐出多个 [tool_call ...][/tool_call] 块, 中间还
// 夹着前导思考文本与块间换行. extractor 必须能在一次 Write 里把它们
// 全部抠出来, 透传给客户端的 delta.tool_calls 数量必须与上游吐出的块
// 数严格一致, content 流不能残留任何 [tool_call / [/tool_call] 文本.
//
// 关键词: 真实场景复刻, multi tool_call 单 chunk, opencode 样式全炸修复,
//        content 不残留 tool_call 文本
func TestRound2ReactMode_ParallelToolCallsInOneContentChunk(t *testing.T) {
	// 模拟用户截图里 6 个并行 bash tool_call 的内容流: 一次 SSE content
	// chunk 包含前导文本 + 6 个 [tool_call ...]...[/tool_call] 块.
	// 关键词: 单 chunk 多 tool_call 实战场景
	combinedContent := `让我继续测试余下的漏洞类别:
[tool_call id="call01_lyf4_Jffk0pgTH_KRq74Gop6FH6w6sap" name="bash"]
{"command":"curl -s \"http://127.0.0.1:8787/jwt/unsafe-login1\" -d \"username=admin&password=admin\" -i 2>&1 | head -20","description":"JWT login attempt"}
[/tool_call]
[tool_call id="call_02_yfJk60hwYjJHKgo0qOe" name="bash"]
{"command":"curl -s \"http://127.0.0.1:8787/jwt/unsafe-login1/register\" 2>&1","description":"JWT register page"}
[/tool_call]
[tool_call id="call_03_xWyJ7BTGmIRfcZcfaPRkB" name="bash"]
{"command":"curl -s \"http://127.0.0.1:8787/exec/ping/bash?ip=127.0.0.1;id\" 2>&1","description":"Command injection with semicolon"}
[/tool_call]
[tool_call id="call_04_zfaqM2WBxbREyl3WdM6N" name="bash"]
{"command":"curl -s \"http://127.0.0.1:8787/exec/ping/shlex?ip=127.0.0.1;whoami\" 2>&1","description":"Command injection shlex with semicolon"}
[/tool_call]
[tool_call id="call05_8VqMNd76rR0fMo6jdMa6822" name="bash"]
{"command":"curl -s \"http://127.0.0.1:8787/user/cookie-id\" -b \"id=1 OR 1=1\" 2>&1","description":"Cookie SQLi OR injection"}
[/tool_call]
[tool_call id="call06_idMrgp3n0YRWxBpVxrJVgN0BS0" name="bash"]
{"command":"curl -s -X POST \"http://127.0.0.1:8787/vul/auth-bypass/unsafe?user=1\" -d \"username=admin&password=admin\" 2>&1","description":"Auth bypass with user param in query"}
[/tool_call]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		hostile := bytes.Contains(body, []byte(`"tool_calls"`)) ||
			bytes.Contains(body, []byte(`"role":"tool"`)) ||
			bytes.Contains(body, []byte(`"tools":[{`))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if hostile {
			writeFrame(`{"id":"hostile","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		// 关键: 把所有内容塞到 *一个* content delta 里, 模拟 deepseek-v4-pro
		// 用户截图里真实的"5-6 个 tool_call 一次性吐出"的 chunk 形态.
		writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, combinedContent))
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":200,"total_tokens":300}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	cfg := setupReactRound2Cfg(t, srv.URL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"))

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

	// 必须解析出 6 个 tool_call
	require.Len(t, toolCalls, 6,
		"6 parallel tool_calls in one chunk must all be extracted, got %d, content=%q",
		len(toolCalls), contentTotal.String())

	// 校验每个 tool_call 的 name + id 都正确
	expectedIDs := []string{
		"call01_lyf4_Jffk0pgTH_KRq74Gop6FH6w6sap",
		"call_02_yfJk60hwYjJHKgo0qOe",
		"call_03_xWyJ7BTGmIRfcZcfaPRkB",
		"call_04_zfaqM2WBxbREyl3WdM6N",
		"call05_8VqMNd76rR0fMo6jdMa6822",
		"call06_idMrgp3n0YRWxBpVxrJVgN0BS0",
	}
	for i := 0; i < 6; i++ {
		tc, ok := toolCalls[i]
		require.True(t, ok, "must have tool_call at index %d", i)
		assert.Equal(t, "bash", tc.Name, "tool_call[%d] name", i)
		assert.Equal(t, expectedIDs[i], tc.ID, "tool_call[%d] id must be preserved from model output", i)
		// args 必须是合法 JSON
		var parsedArgs map[string]any
		require.NoError(t, json.Unmarshal([]byte(tc.Arguments), &parsedArgs),
			"tool_call[%d] args must be valid JSON, got: %q", i, tc.Arguments)
		_, hasCmd := parsedArgs["command"]
		assert.True(t, hasCmd, "tool_call[%d] args must contain 'command' field", i)
	}

	// content 流不应包含任何 [tool_call ...] 或 [/tool_call] 文本残骸,
	// 否则会以 markdown 文本形态出现在 opencode UI, 用户截图里的 "样式
	// 全炸" 就是这个 bug.
	// 关键词: opencode 样式全炸 - content 不残留 tool_call 文本
	assert.NotContains(t, contentTotal.String(), "[tool_call",
		"content stream must NOT contain [tool_call ...] residue (opencode renders as markdown link)")
	assert.NotContains(t, contentTotal.String(), "[/tool_call]",
		"content stream must NOT contain [/tool_call] residue")

	// 前导思考文本必须保留
	assert.Contains(t, contentTotal.String(), "让我继续测试",
		"leading thinking text before tool_calls must be forwarded as content")

	assert.Equal(t, "tool_calls", lastFR,
		"6 parallel tool_calls response: finish_reason must be tool_calls")
}

// TestSafetyNet_NativeProviderLeaksToolCallText_StillExtracted
// 复刻用户截图里的真实退化: provider DB 明确配置为 native (Round1=native,
// Round2=native, AutoFallback=false), 但上游 wrapper 在并行多 tool_call
// 场景下随机退化, 把 [tool_call ...]...[/tool_call] 文本写入 content 流.
//
// 旧实现: useReactMode=false -> extractor 未启用 -> content 流原样透传 ->
//        opencode 把 [tool_call ...] 当 markdown 链接渲染 -> 工具不执行
//        (用户截图里 "样式全炸" 的根因).
// 新实现: needReactExtractor=true (因为有 tools / round-trip 标记) ->
//        extractor 始终兜底 -> content 流里的 [tool_call ...] 文本被反解析
//        为 OpenAI tool_calls delta -> opencode 正确触发工具执行.
//
// 关键词: safety net 兜底 react extractor 验证, native mode 也启用 extractor,
//        provider 退化场景, opencode 样式全炸根因修复
func TestSafetyNet_NativeProviderLeaksToolCallText_StillExtracted(t *testing.T) {
	// 模拟一个**明确配置为 native** 的 provider, 但上游 wrapper 在 content 流里
	// 退化吐 ReAct 文本格式的 tool_calls.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		// 退化点: 上游本应走 native delta.tool_calls, 却把多个 tool_call 以
		// ReAct 文本格式写到 content. native 模式下 (旧实现) extractor 不启用,
		// 这些文本就会直接透传给客户端被当 markdown 渲染.
		// 关键词: native provider content 流退化 ReAct 文本
		degradedContent := `让我做并行测试:
[tool_call id="call_native_1" name="bash"]{"command":"curl /a"}[/tool_call]
[tool_call id="call_native_2" name="bash"]{"command":"curl /b"}[/tool_call]
[tool_call id="call_native_3" name="bash"]{"command":"curl /c"}[/tool_call]`
		writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, degradedContent))
		writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"deepseek","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":80,"total_tokens":130}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	// 关键: provider 明确配置为 native (不走 react 路径)
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
		DomainOrURL: srv.URL,
		APIKey:      "upstream-key",
		WrapperName: modelName,
		NoHTTPS:     true,
		DbProvider: &schema.AiProvider{
			WrapperName: modelName,
			ModelName:   modelName,
			TypeName:    "deepseek",
			DomainOrURL: srv.URL,
			APIKey:      "upstream-key",
			IsHealthy:   true,
			LastLatency: 100,
			// 显式 native, 关闭 AutoFallback
			ToolCallsRound1Mode: "native",
			ToolCallsRound2Mode: "native",
		},
	}
	cfg.Models.models[modelName] = []*Provider{provider}
	cfg.Entrypoints.providers[modelName] = []*Provider{provider}

	// 请求体: round1 (无 round-trip 标记) + 带 tools, native mode 直接透传
	body := `{
  "model": "deepseek-v4-pro",
  "stream": true,
  "messages": [{"role": "user", "content": "Run parallel tests."}],
  "tools": [
    {"type":"function","function":{"name":"bash","description":"run bash","parameters":{"type":"object","properties":{"command":{"type":"string"}}}}}
  ]
}`

	resp := driveServeRequest(t, cfg, body, nil)
	require.True(t, strings.Contains(resp, "200 OK"))

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks)

	var (
		contentTotal strings.Builder
		toolCalls    = map[int]*toolCallAccum{}
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

	// safety-net extractor 必须把 content 流里的 3 个 [tool_call ...] 文本
	// 抠出来转成 OpenAI tool_calls delta, 让 opencode 能触发执行.
	require.Len(t, toolCalls, 3,
		"safety-net extractor must extract 3 tool_calls from native-mode content leakage, got: %+v, content=%q",
		toolCalls, contentTotal.String())
	assert.Equal(t, "call_native_1", toolCalls[0].ID)
	assert.Equal(t, "call_native_2", toolCalls[1].ID)
	assert.Equal(t, "call_native_3", toolCalls[2].ID)

	// content 必须不残留 [tool_call ...] 文本
	assert.NotContains(t, contentTotal.String(), "[tool_call",
		"safety-net: content stream must NOT contain [tool_call ...] residue even in native mode")
	// 但前导思考文本必须保留
	assert.Contains(t, contentTotal.String(), "让我做并行测试")
}

// TestSafetyNet_NoToolsNoMarker_ExtractorNotEnabled
// 反例: 纯文本对话 (无 tools 字段、无 round-trip 标记) 时,
// safety-net 不应启用 extractor, 避免对正常文本对话产生副作用.
// 即使内容里恰好有 `[tool_call ...]` 字面文本 (比如用户问 "what is
// [tool_call] syntax"), 也应原样透传给客户端.
//
// 关键词: safety net 零副作用, 纯文本对话不启用 extractor
func TestSafetyNet_NoToolsNoMarker_ExtractorNotEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		writeFrame := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		// 输出里包含 [tool_call ...] 子串但不是真的工具调用 (用户在问语法)
		// 关键词: 教学文本 [tool_call] 字面引用, 不应被误抠
		teachingText := `The [tool_call name=foo]args[/tool_call] syntax is used in ReAct prompts.`
		writeFrame(fmt.Sprintf(`{"id":"r","object":"chat.completion.chunk","created":1717000000,"model":"deepseek","choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":null}]}`, teachingText))
		writeFrame(`{"id":"r","object":"chat.completion.chunk","created":1717000000,"model":"deepseek","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	cfg := setupDeepseekServerCfg(t, srv.URL, "deepseek-v4-pro")
	defer cfg.Close()

	// 纯文本对话: 不带 tools, 不带 round-trip 标记
	body := `{
  "model": "deepseek-v4-pro",
  "stream": true,
  "messages": [{"role":"user","content":"explain ReAct tool_call syntax"}]
}`
	resp := driveServeRequest(t, cfg, body, nil)
	require.True(t, strings.Contains(resp, "200 OK"))

	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks)

	var contentTotal strings.Builder
	var toolCallCount int
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
		if c, ok := delta["content"].(string); ok && c != "" {
			contentTotal.WriteString(c)
		}
		if tcArr, ok := delta["tool_calls"].([]any); ok {
			toolCallCount += len(tcArr)
		}
	}

	// 纯文本对话 + 教学文本场景: safety-net 不应启用 extractor,
	// 客户端 content 必须原样收到 [tool_call ...] 字面文本.
	assert.Zero(t, toolCallCount,
		"plain text conversation must not emit tool_calls (no tools/marker in request)")
	assert.Contains(t, contentTotal.String(), `[tool_call name=foo]`,
		"plain text conversation must forward literal [tool_call ...] text as-is")
	assert.Contains(t, contentTotal.String(), `[/tool_call]`,
		"plain text conversation must forward literal [/tool_call] text as-is")
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
