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
		hasReactToolCallText := strings.Contains(raw, "[tool_call")
		hasReactToolResultText := strings.Contains(raw, "[tool_result")

		model := extractField(body, "model")
		if model == "" {
			model = "mock-dumb"
		}

		switch {
		case hasRoleTool || hasToolCallsField:
			// 复刻线上故障: tool_calls round-trip -> 立即空回
			respondEmpty(w, model, isStream)
		case hasReactToolCallText || hasReactToolResultText:
			// 收到 ReAct 文本风格的 round-trip: 模拟模型读懂 tool_result 后回答
			respondReactRound2(w, model, isStream)
		case hasToolsField:
			// round1 + 不识别 tools: 直接回纯文本 (不调工具)
			respondNoTool(w, model, isStream)
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
func respondNoTool(w http.ResponseWriter, model string, isStream bool) {
	// 如果请求里出现 ReAct system prompt 关键词, 说明 aibalance 已经把 tools 转换为
	// ReAct 文本格式; 我们模拟模型"遵循 system prompt"输出 [tool_call ...] 文本.
	// 否则就当作普通对话, 不调任何工具.
	respondReactRound1ToolCall(w, model, isStream)
}

// 关键词: dumb respondReactRound1ToolCall, ReAct 文本工具调用输出
func respondReactRound1ToolCall(w http.ResponseWriter, model string, isStream bool) {
	// 输出 ReAct 风格的 tool_call 文本, 让 aibalance 的 react_tool_extractor 反解析.
	// name 与 e2e 测试约定一致 (get_current_weather), 参数包含 city=Beijing 这种典型值.
	toolCallText := `[tool_call name=get_current_weather]{"city":"Beijing"}[/tool_call]`
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
