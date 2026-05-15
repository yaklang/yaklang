// mock-dumb-upstream 模拟一个不识别 OpenAI tool_calls 协议的 wrapper:
//   - 收到含 tools=[...] 的 round1 请求: 不调工具, 只回纯文本 (经典 z-deepseek 行为)
//   - 收到含 assistant.tool_calls / role=tool 的 round2 请求: 立即 finish_reason=stop
//     + content="" 空回 (复刻线上 z-deepseek-v4-pro round2 故障行为)
//   - 收到 ReAct flatten 后的请求 (没有 tools 字段 + 没有 role=tool + 出现
//     [tool_call ...] / [tool_result ...] 文本): 走 ReAct 处理流程, 返回自然语言
//
// 关键词: aibalance mock dumb upstream, hostile wrapper 复刻, z-deepseek-v4-pro round2 故障
//
// 启动: go run common/aibalance/cmd/mocks/mock-dumb-upstream/main.go --addr :18802
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18802", "listen address")
	verbose := flag.Bool("verbose", false, "print every request body")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if *verbose {
			log.Printf("[dumb] %s %s body=%s", r.Method, r.URL.Path, string(body))
		}
		raw := string(body)
		isStream := strings.Contains(raw, `"stream":true`) || strings.Contains(raw, `"stream": true`)
		hasRoleTool := strings.Contains(raw, `"role":"tool"`) || strings.Contains(raw, `"role": "tool"`)
		hasToolCallsField := strings.Contains(raw, `"tool_calls"`)
		hasToolsField := strings.Contains(raw, `"tools"`)
		// 注意: ReAct 标记探测必须只在非 system 角色的 content 里 grep,
		// 因为 round1 inject 后 system content 自身也会包含 "[tool_call" / "[tool_result"
		// 这两个字面量作为格式说明, 不能把它当成 round2 信号. 关键词: mock dumb ReAct 检测误判修复.
		hasReactToolCallText := nonSystemContentContains(body, "[tool_call")
		hasReactToolResultText := nonSystemContentContains(body, "[tool_result")
		// round1 inject 探针: aibalance 把 tools 转为 system prompt + 清空 tools 字段后, 上游需要按
		// system prompt 的 ReAct 描述回 [tool_call ...] 文本. 探测条件: system content 里出现
		// "Available tools:" 列表 + "- name:" 至少一项.
		isReactRound1Inject := systemContainsReactInject(body)

		model := extractField(body, "model")
		if model == "" {
			model = "mock-dumb"
		}

		switch {
		case hasRoleTool || hasToolCallsField:
			// 复刻线上故障: tool_calls round-trip -> 立即空回
			respondEmpty(w, model, isStream)
		case hasReactToolResultText:
			// round2 ReAct flatten: 已经携带 [tool_result ...] 文本, 模拟模型读懂后回答
			respondReactRound2(w, model, isStream)
		case isReactRound1Inject || hasReactToolCallText || hasToolsField:
			// round1 inject 后 system prompt 里出现 ReAct 工具描述, 或裸 tools 字段:
			// 模拟模型"遵循 system prompt"按 ReAct 格式输出 [tool_call ...] 文本
			respondNoTool(w, model, isStream, body)
		default:
			// 纯文本对话, 正常回答
			respondPlain(w, model, isStream)
		}
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("[mock-dumb-upstream] listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("listen error: %v", err)
		os.Exit(1)
	}
}

// 关键词: dumb respondEmpty, 复刻 z-deepseek-v4-pro round2 故障
func respondEmpty(w http.ResponseWriter, model string, isStream bool) {
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-dumb-empty",
  "object": "chat.completion",
  "created": 1717000000,
  "model": %q,
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": ""},
    "finish_reason": "stop"
  }]
}`, model)))
		return
	}
	streamSSE(w, []string{
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-empty","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-empty","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model),
	})
}

// 关键词: dumb respondReactRound2, ReAct 文本风格 round2
func respondReactRound2(w http.ResponseWriter, model string, isStream bool) {
	finalText := "Based on the tool result, here is the answer: ok."
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-dumb-react",
  "object": "chat.completion",
  "created": 1717000000,
  "model": %q,
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": %q},
    "finish_reason": "stop"
  }]
}`, model, finalText)))
		return
	}
	pieces := []string{
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-react","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
	}
	for _, p := range []string{"Based on the tool result, ", "here is the answer: ok."} {
		pieces = append(pieces, fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-react","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":%q}}]}`, model, p))
	}
	pieces = append(pieces, fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-react","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model))
	streamSSE(w, pieces)
}

// 关键词: dumb respondNoTool, 不识别 tools 字段, 直接回 NL (注意: 这种情况下 aibalance
// round1 react inject 模式会把 tools 转成 system prompt + 触发模型按 ReAct 文本格式
// 输出 [tool_call ...][/tool_call], 但裸 dumb upstream 不会自动遵循, 这里模拟"模型
// 看到 ReAct system prompt 后理解并输出 [tool_call] 文本"的友好行为, 让 e2e 测试链
// 路完整闭环).
func respondNoTool(w http.ResponseWriter, model string, isStream bool, body []byte) {
	// 如果请求里出现 ReAct system prompt 关键词, 说明 aibalance 已经把 tools 转换为
	// ReAct 文本格式; 我们模拟模型"遵循 system prompt"输出 [tool_call ...] 文本.
	// 否则就当作普通对话, 不调任何工具.
	respondReactRound1ToolCall(w, model, isStream, body)
}

// 关键词: dumb respondReactRound1ToolCall, ReAct 文本工具调用输出
func respondReactRound1ToolCall(w http.ResponseWriter, model string, isStream bool, body []byte) {
	// 输出 ReAct 风格的 tool_call 文本, 让 aibalance 的 react_tool_extractor 反解析.
	// 优先从 ReAct system prompt 注入文本里 grep tool name (round1 inject 后用 "- name: X" 列出),
	// 若没有再 fallback 到默认 get_current_weather.
	toolNames := extractReactInjectedToolNames(body)
	if len(toolNames) == 0 {
		toolNames = []string{"get_current_weather"}
	}
	defaultArgs := `{"city":"Beijing"}`
	parts := make([]string, 0, len(toolNames))
	for _, n := range toolNames {
		parts = append(parts, fmt.Sprintf(`[tool_call name=%s]%s[/tool_call]`, n, defaultArgs))
	}
	toolCallText := strings.Join(parts, "\n")
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-dumb-r1",
  "object": "chat.completion",
  "created": 1717000000,
  "model": %q,
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": %q},
    "finish_reason": "stop"
  }]
}`, model, toolCallText)))
		return
	}
	pieces := []string{
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-r1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-r1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":%q}}]}`, model, toolCallText),
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-r1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model),
	}
	streamSSE(w, pieces)
}

// 关键词: dumb extractReactInjectedToolNames, ReAct system prompt 解析 tool name 列表
func extractReactInjectedToolNames(body []byte) []string {
	// 注入格式约定: 在 system content 里逐行写 "- name: TOOL_NAME"
	probe := map[string]any{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil
	}
	msgs, _ := probe["messages"].([]any)
	var names []string
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		if mm == nil {
			continue
		}
		role, _ := mm["role"].(string)
		if role != "system" {
			continue
		}
		content, _ := mm["content"].(string)
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- name:") {
				continue
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "- name:"))
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// 关键词: dumb respondPlain, 纯文本对话
func respondPlain(w http.ResponseWriter, model string, isStream bool) {
	finalText := "Hello from mock-dumb-upstream."
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-dumb-plain",
  "object": "chat.completion",
  "created": 1717000000,
  "model": %q,
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": %q},
    "finish_reason": "stop"
  }]
}`, model, finalText)))
		return
	}
	pieces := []string{
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-plain","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-plain","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":%q}}]}`, model, finalText),
		fmt.Sprintf(`{"id":"chatcmpl-mock-dumb-plain","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model),
	}
	streamSSE(w, pieces)
}

// 关键词: dumb streamSSE
func streamSSE(w http.ResponseWriter, frames []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, f := range frames {
		fmt.Fprintf(w, "data: %s\n\n", f)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(3 * time.Millisecond)
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// 关键词: dumb systemContainsReactInject, system 内是否含 round1 react inject 描述
func systemContainsReactInject(body []byte) bool {
	probe := map[string]any{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	msgs, _ := probe["messages"].([]any)
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		if mm == nil {
			continue
		}
		role, _ := mm["role"].(string)
		if role != "system" {
			continue
		}
		c, _ := mm["content"].(string)
		if strings.Contains(c, "Available tools:") && strings.Contains(c, "- name:") {
			return true
		}
	}
	return false
}

// 关键词: dumb nonSystemContentContains, 非 system 消息内容里是否含 needle
func nonSystemContentContains(body []byte, needle string) bool {
	probe := map[string]any{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	msgs, _ := probe["messages"].([]any)
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		if mm == nil {
			continue
		}
		role, _ := mm["role"].(string)
		if role == "system" {
			continue
		}
		if c, ok := mm["content"].(string); ok && strings.Contains(c, needle) {
			return true
		}
	}
	return false
}

// 关键词: dumb extractField
func extractField(body []byte, key string) string {
	probe := map[string]any{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	if v, ok := probe[key].(string); ok {
		return v
	}
	return ""
}
