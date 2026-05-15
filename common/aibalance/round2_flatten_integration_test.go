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

// hostileWrapperUpstream 模拟一个**不识别 OpenAI tool_calls 协议**的上游 wrapper
// (例如线上 z-deepseek-v4-pro / z-deepseek-v4-flash 实际表现):
//   - 一旦 messages 数组中出现 assistant.tool_calls 或 role=tool, 立刻
//     finish_reason=stop + 空 content (复刻线上观察到的 z-deepseek 行为);
//   - 否则按 ReAct 文本风格正常生成 NL 回答。
//
// 关键词: 上游 wrapper hostile mock, tool_calls 字段触发空回
func hostileWrapperUpstream(t *testing.T) (string, func() []byte, func()) {
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

		// 关键: 检测上游收到的 body 里是否含 OpenAI tool_calls round-trip 标记
		hostile := bytes.Contains(body, []byte(`"tool_calls"`)) ||
			bytes.Contains(body, []byte(`"role":"tool"`))

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

		if hostile {
			// 复刻线上空回: 立刻 finish_reason=stop 没有任何 content
			// 关键词: hostile wrapper 空回, finish_reason=stop 立刻关闭
			writeFrame(`{"id":"empty","object":"chat.completion.chunk","created":1717000000,"model":"hostile-wrapper","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		} else {
			// ReAct 文本风格请求: 正常给出 NL 回答
			// 关键词: hostile wrapper ReAct 文本路径正常输出
			writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile-wrapper","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
			for _, piece := range []string{"Beijing ", "is currently ", "sunny ", "with ", "21C."} {
				writeFrame(fmt.Sprintf(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile-wrapper","choices":[{"index":0,"delta":{"content":%q},"finish_reason":null}]}`, piece))
			}
			writeFrame(`{"id":"r2","object":"chat.completion.chunk","created":1717000000,"model":"hostile-wrapper","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60}}`)
		}
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

const round2RequestBody = `{
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

// 注: 「不启用 flatten 时 hostile wrapper 必然空回」的反例 baseline 此处不写
// 端到端集成 case: 因为 server.go 在 all-providers-failed 分支会等 90s tcp
// timeout 才退出 server goroutine, 在全量并发跑测试套时容易跟其它用例 race
// 互相污染状态。该 BUG 复现已经在以下层面充分覆盖:
//   - probe_round2.sh + curl 真实命中线上 z-deepseek-v4-pro 给出空响应;
//   - round2_flatten_test.go 的 IsRoundTripFlattenEligible 单元测试断言
//     hostile 标记被正确识别;
//   - 下面 TestServeChatCompletions_HostileWrapperWithFlatten 的对照路径
//     会在同一个 hostile mock + 同一份 round2 body 上验证: 启用 flatten 后
//     客户端拿到 NL 响应、上游收不到 tool_calls 字段。
//
// 关键词: round2 flatten baseline 论证, 端到端覆盖说明

// TestServeChatCompletions_HostileWrapperWithFlatten 启用 flatten 后, 上游
// wrapper 不再收到 tool_calls 字段, 而是收到 ReAct 文本风格 messages, 客户端
// 应拿到完整 NL 响应。这是核心修复验证。
// 关键词: hostile wrapper round2 flatten 修复验证, 客户端收到 NL 响应
func TestServeChatCompletions_HostileWrapperWithFlatten(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "deepseek-v4-pro")
	t.Setenv(envFlattenToolCallsAll, "")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)

	srvURL, getRaw, closeFn := hostileWrapperUpstream(t)
	defer closeFn()

	cfg := setupDeepseekServerCfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBody, nil)
	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks, "must receive at least 1 SSE chunk")

	var content strings.Builder
	var lastFR string
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
			content.WriteString(c)
		}
	}

	// 上游 mock 必须**没有**收到 tool_calls 与 role=tool (说明 flatten 生效)
	raw := getRaw()
	assert.NotContains(t, string(raw), `"tool_calls"`,
		"flatten 生效后, 上游 wrapper 不应再看到 tool_calls 字段")
	assert.NotContains(t, string(raw), `"role":"tool"`,
		"flatten 生效后, 上游 wrapper 不应再看到 role=tool 消息")
	// 上游 mock 应该看到 ReAct 文本痕迹 (tool_call / tool_result 标签)
	assert.Contains(t, string(raw), "[tool_call",
		"flatten 后 assistant 内容应包含 [tool_call ...] 文本")
	assert.Contains(t, string(raw), "[tool_result",
		"flatten 后 user 内容应包含 [tool_result ...] 文本")

	// 客户端应该真的拿到 NL 响应
	assert.Equal(t, "Beijing is currently sunny with 21C.", content.String(),
		"flatten 后客户端 round2 必须拿到完整 NL 响应 (修复 z-deepseek-v4-pro 空回 BUG)")
	assert.Equal(t, "stop", lastFR,
		"flatten 后 round2 finish_reason 应为 stop")
}

// TestServeChatCompletions_GlobalKillSwitchFlatten 验证全局开关
// AIBALANCE_FLATTEN_TOOLCALLS_ALL=true 时, 即使 model 不在白名单也强制 flatten。
// 关键词: 全局 kill switch round2 flatten 端到端
func TestServeChatCompletions_GlobalKillSwitchFlatten(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "")
	t.Setenv(envFlattenToolCallsAll, "true")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)

	srvURL, getRaw, closeFn := hostileWrapperUpstream(t)
	defer closeFn()

	cfg := setupDeepseekServerCfg(t, srvURL, "any-model-name")
	defer cfg.Close()

	body := strings.Replace(round2RequestBody, `"deepseek-v4-pro"`, `"any-model-name"`, 1)
	resp := driveServeRequest(t, cfg, body, nil)
	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks)

	var content strings.Builder
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
			content.WriteString(c)
		}
	}

	raw := getRaw()
	assert.NotContains(t, string(raw), `"tool_calls"`,
		"全局 kill switch on 时, 上游不应再看到 tool_calls")
	assert.NotEmpty(t, content.String(),
		"全局 kill switch on 时, 客户端应拿到 NL 响应")
}

// TestServeChatCompletions_FlattenSkippedWhenNoRoundTripMarker 没有 tool_calls
// round-trip 标记的请求 (纯文本对话或 round1) 即使白名单命中也不应 flatten,
// 避免对正常请求产生副作用。
// 关键词: round2 flatten 触发条件 - 仅 round-trip 才 flatten
func TestServeChatCompletions_FlattenSkippedWhenNoRoundTripMarker(t *testing.T) {
	t.Setenv(envFlattenToolCallsForModels, "deepseek-v4-pro")
	t.Setenv(envFlattenToolCallsAll, "")
	resetFlattenEnvCacheForTest()
	t.Cleanup(resetFlattenEnvCacheForTest)

	srvURL, getRaw, closeFn := hostileWrapperUpstream(t)
	defer closeFn()

	cfg := setupDeepseekServerCfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	plainBody := `{
      "model": "deepseek-v4-pro",
      "stream": true,
      "messages": [
        {"role": "user", "content": "What's 2+2?"},
        {"role": "assistant", "content": "It is 4."},
        {"role": "user", "content": "And 3+3?"}
      ]
    }`
	resp := driveServeRequest(t, cfg, plainBody, nil)
	chunks := parseSSEDataChunks(t, resp)
	require.NotEmpty(t, chunks, "must receive SSE chunks for plain text round")

	raw := getRaw()
	// 既然 messages 不含 tool_calls / role=tool, flatten 不应触发, 即上游
	// 收到的 body 应保持原样 (不含 [tool_call] / [tool_result] 文本)。
	assert.NotContains(t, string(raw), "[tool_call",
		"纯文本对话不应触发 flatten")
	assert.NotContains(t, string(raw), "[tool_result",
		"纯文本对话不应触发 flatten")
}
