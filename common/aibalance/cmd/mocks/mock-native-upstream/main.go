// mock-native-upstream 是一个原生 OpenAI Chat Completions 兼容的 mock 上游服务,
// 用于 aibalance tool_calls capability matrix E2E 测试. 它完全支持 OpenAI tool_calls
// 协议: round1 (含 tools=[...]) 会返回结构化 tool_calls + finish_reason=tool_calls;
// round2 (含 assistant.tool_calls + role=tool) 会返回纯文本回答 + finish_reason=stop.
//
// 关键词: aibalance mock native upstream, OpenAI tool_calls compliant, capability matrix test
//
// 启动: go run common/aibalance/cmd/mocks/mock-native-upstream/main.go --addr :18801
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
	addr := flag.String("addr", "127.0.0.1:18801", "listen address")
	verbose := flag.Bool("verbose", false, "print every request body")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if *verbose {
			log.Printf("[native] %s %s body=%s", r.Method, r.URL.Path, string(body))
		}

		// 解析请求, 判断是否含 tools=... 还是 round-trip (role=tool / tool_calls)
		hasTools := strings.Contains(string(body), `"tools"`) && strings.Contains(string(body), `"function"`)
		hasRoleTool := strings.Contains(string(body), `"role":"tool"`) ||
			strings.Contains(string(body), `"role": "tool"`)
		hasToolCalls := strings.Contains(string(body), `"tool_calls"`)

		// 解析 model 名 (尽量保留, 用于响应头)
		model := extractField(body, "model")
		if model == "" {
			model = "mock-native"
		}

		isStream := strings.Contains(string(body), `"stream":true`) ||
			strings.Contains(string(body), `"stream": true`)

		if hasRoleTool || hasToolCalls {
			respondRound2(w, model, isStream)
			return
		}
		if hasTools {
			respondRound1WithToolCall(w, model, isStream, body)
			return
		}
		respondPlain(w, model, isStream)
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("[mock-native-upstream] listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("listen error: %v", err)
		os.Exit(1)
	}
}

// 关键词: native respondRound1WithToolCall, OpenAI tool_calls stream
func respondRound1WithToolCall(w http.ResponseWriter, model string, isStream bool, body []byte) {
	// 通常 client 请求 tool 名为 get_current_weather / get_weather 之类
	toolName := extractToolFunctionName(body)
	if toolName == "" {
		toolName = "echo"
	}
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-native-1",
  "object": "chat.completion",
  "created": 1717000000,
  "model": %q,
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "",
      "tool_calls": [{
        "id": "call_native_1",
        "type": "function",
        "function": {"name": %q, "arguments": "{}"}
      }]
    },
    "finish_reason": "tool_calls"
  }]
}`, model, toolName)))
		return
	}
	streamSSE(w, []string{
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_native_1","type":"function","function":{"name":%q,"arguments":""}}]}}]}`, model, toolName),
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-1","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`, model),
	})
}

// 关键词: native respondRound2, NL 回应
func respondRound2(w http.ResponseWriter, model string, isStream bool) {
	finalText := "The tool returned its result successfully. Summary: ok."
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-native-2",
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
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-2","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
	}
	for _, piece := range []string{"The tool ", "returned its result ", "successfully. ", "Summary: ok."} {
		pieces = append(pieces, fmt.Sprintf(`{"id":"chatcmpl-mock-native-2","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":%q}}]}`, model, piece))
	}
	pieces = append(pieces, fmt.Sprintf(`{"id":"chatcmpl-mock-native-2","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model))
	streamSSE(w, pieces)
}

// 关键词: native respondPlain, 普通对话
func respondPlain(w http.ResponseWriter, model string, isStream bool) {
	finalText := "Hello from mock-native-upstream."
	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
  "id": "chatcmpl-mock-native-3",
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
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-3","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`, model),
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-3","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":%q}}]}`, model, finalText),
		fmt.Sprintf(`{"id":"chatcmpl-mock-native-3","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, model),
	}
	streamSSE(w, pieces)
}

// 关键词: streamSSE helper, OpenAI 兼容 SSE 帧输出
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

// 关键词: extractField, naive JSON field extraction
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

// 关键词: extractToolFunctionName, find first tools[].function.name in body
func extractToolFunctionName(body []byte) string {
	probe := map[string]any{}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	tools, _ := probe["tools"].([]any)
	if len(tools) == 0 {
		return ""
	}
	first, _ := tools[0].(map[string]any)
	if first == nil {
		return ""
	}
	fn, _ := first["function"].(map[string]any)
	if fn == nil {
		return ""
	}
	if name, ok := fn["name"].(string); ok {
		return name
	}
	return ""
}
