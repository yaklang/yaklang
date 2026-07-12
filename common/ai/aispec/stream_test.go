package aispec

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestProcessNonStreamResponse(t *testing.T) {
	// 测试非流式响应处理函数
	nonStreamData := `{"choices":[{"message":{"content":"Hello World","reasoning_content":"This is my reasoning process"}}]}`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(nonStreamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected stream read error: %v", err)
	}

	expectedContent := "Hello World"
	expectedReason := "This is my reasoning process"

	if outBuffer.String() != expectedContent {
		t.Errorf("内容输出不匹配，期望: %s, 实际: %s", expectedContent, outBuffer.String())
	}

	if reasonBuffer.String() != expectedReason {
		t.Errorf("推理输出不匹配，期望: %s, 实际: %s", expectedReason, reasonBuffer.String())
	}

	t.Logf("非流式内容输出: %s", outBuffer.String())
	t.Logf("非流式推理输出: %s", reasonBuffer.String())
}

func TestProcessStreamResponse(t *testing.T) {
	// 测试流式响应处理函数
	streamData := `data: {"choices":[{"delta":{"reasoning_content":"思考中..."}}]}
data: {"choices":[{"delta":{"content":"Hello"}}]}
data: {"choices":[{"delta":{"content":" World"}}]}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected stream read error: %v", err)
	}

	t.Logf("流式内容输出: %s", outBuffer.String())
	t.Logf("流式推理输出: %s", reasonBuffer.String())
}

func TestAppendStreamHandlerPoCOptionEx(t *testing.T) {
	// 测试流式和非流式选项创建
	outReader, reasonReader, opts, cancel, _ := appendStreamHandlerPoCOptionEx(true, []poc.PocConfigOption{}, nil, nil, nil, nil)
	defer cancel()

	if outReader == nil || reasonReader == nil {
		t.Error("输出读取器不应为nil")
	}

	if len(opts) == 0 {
		t.Error("应该添加了处理选项")
	}

	// 测试非流式
	outReader2, reasonReader2, opts2, cancel2, _ := appendStreamHandlerPoCOptionEx(false, []poc.PocConfigOption{}, nil, nil, nil, nil)
	defer cancel2()

	if outReader2 == nil || reasonReader2 == nil {
		t.Error("非流式输出读取器不应为nil")
	}

	if len(opts2) == 0 {
		t.Error("非流式应该添加了处理选项")
	}
}

// TestProcessAIResponse_UsageCallback 验证 SSE 流末帧的 usage 字段
// 能被 processAIResponse 正确抽取并通过 usageCallback 回传，
// 同时确保前面 chunk 里 usage:null 不会污染最终值。
//
// 关键词: SSE 末帧 usage 抽取, usageCallback 单测
func TestProcessAIResponse_UsageCallback(t *testing.T) {
	streamData := `data: {"id":"a","choices":[{"delta":{"content":"Hi"}}],"usage":null}
data: {"id":"a","choices":[{"delta":{"content":" there"}}],"usage":null}
data: {"id":"a","choices":[{"delta":{}}],"usage":{"prompt_tokens":162002,"completion_tokens":2500,"total_tokens":164502}}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	var captured *ChatUsage
	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, func(u *ChatUsage) {
		captured = u
	})

	if captured == nil {
		t.Fatalf("usageCallback should be invoked with a non-nil ChatUsage")
	}
	if captured.PromptTokens != 162002 {
		t.Errorf("prompt_tokens want 162002, got %d", captured.PromptTokens)
	}
	if captured.CompletionTokens != 2500 {
		t.Errorf("completion_tokens want 2500, got %d", captured.CompletionTokens)
	}
	if captured.TotalTokens != 164502 {
		t.Errorf("total_tokens want 164502, got %d", captured.TotalTokens)
	}
	if outBuffer.String() != "Hi there" {
		t.Errorf("content stream mismatch, got %q", outBuffer.String())
	}
}

// TestProcessAIResponse_UsageCallback_NoUsage 验证无 usage 字段时回调被调用且参数为 nil。
//
// 关键词: usageCallback nil, no usage block
func TestProcessAIResponse_UsageCallback_NoUsage(t *testing.T) {
	streamData := `data: {"id":"a","choices":[{"delta":{"content":"Hi"}}]}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	called := false
	var captured *ChatUsage
	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, func(u *ChatUsage) {
		called = true
		captured = u
	})

	if !called {
		t.Fatal("usageCallback must be invoked even when no usage block is present")
	}
	if captured != nil {
		t.Errorf("expected nil usage, got %+v", captured)
	}
}

func TestProcessAIResponse_HeaderCallbackBeforeBodyRead(t *testing.T) {
	nonStreamData := `{"choices":[{"message":{"content":"Hello World"}}]}`
	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n")

	pr, pw := io.Pipe()
	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}
	headerCh := make(chan []byte, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		processAIResponse(mockResponse, pr, outBuffer, reasonBuffer, nil, func(header []byte) {
			headerCh <- append([]byte(nil), header...)
		}, nil, nil)
	}()

	select {
	case header := <-headerCh:
		if string(header) != string(mockResponse) {
			t.Fatalf("header callback mismatch, want %q got %q", string(mockResponse), string(header))
		}
	case <-time.After(time.Second):
		t.Fatal("header callback not triggered before body read")
	}

	select {
	case <-done:
		t.Fatal("response processing finished before body was written")
	default:
	}

	if _, err := pw.Write([]byte(nonStreamData)); err != nil {
		t.Fatalf("write body failed: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatalf("close body writer failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("response processing did not finish after body write")
	}

	if outBuffer.String() != "Hello World" {
		t.Fatalf("content output mismatch, got %q", outBuffer.String())
	}
}

func TestProcessAIResponse_StreamReadErrorReturned(t *testing.T) {
	streamData := "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL\"}}]}\n\n"
	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(&errAfterReadReader{
		data: []byte(streamData),
		err:  io.ErrUnexpectedEOF,
	})

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}
	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected stream read error")
	}
	if !strings.Contains(err.Error(), "ai stream read failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if outBuffer.String() != "PARTIAL" {
		t.Fatalf("expected partial content before error, got %q", outBuffer.String())
	}
}

type errAfterReadReader struct {
	data []byte
	err  error
}

func (r *errAfterReadReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, nil
	}
	return n, nil
}

func (r *errAfterReadReader) Close() error { return nil }

// TestProcessAIResponse_OllamaReasoningField_Stream 验证 Ollama /v1/chat/completions
// 使用 "reasoning" 字段（而非标准 "reasoning_content"）时，SSE 流式思考内容
// 能被正确提取到 reasonBuffer。
func TestProcessAIResponse_OllamaReasoningField_Stream(t *testing.T) {
	streamData := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning":"We need"}}]}
data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"content":"","reasoning":" to answer"}}]}
data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"content":"","reasoning":" 1+1."}}]}
data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"content":"2"}}]}
data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reasonBuffer.String() != "We need to answer 1+1." {
		t.Errorf("reason mismatch, want %q, got %q", "We need to answer 1+1.", reasonBuffer.String())
	}
	if outBuffer.String() != "2" {
		t.Errorf("content mismatch, want %q, got %q", "2", outBuffer.String())
	}
}

// TestProcessAIResponse_OllamaReasoningField_NonStream 验证 Ollama /v1/chat/completions
// 使用 "reasoning" 字段的非流式 JSON 响应能被正确提取。
func TestProcessAIResponse_OllamaReasoningField_NonStream(t *testing.T) {
	nonStreamData := `{"id":"chatcmpl-1","object":"chat.completion","model":"kimi-k2.7-code","choices":[{"index":0,"message":{"role":"assistant","content":"1 + 1 = 2","reasoning":"We need answer simple math. 1+1=2."},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":25,"total_tokens":37}}`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(nonStreamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if outBuffer.String() != "1 + 1 = 2" {
		t.Errorf("content mismatch, want %q, got %q", "1 + 1 = 2", outBuffer.String())
	}
	if reasonBuffer.String() != "We need answer simple math. 1+1=2." {
		t.Errorf("reason mismatch, want %q, got %q", "We need answer simple math. 1+1=2.", reasonBuffer.String())
	}
}

// TestProcessAIResponse_StandardReasoningContent_StillWorks 确保标准的 reasoning_content
// 字段（deepseek/kimi API 等）不受 Ollama fallback 影响。
func TestProcessAIResponse_StandardReasoningContent_StillWorks(t *testing.T) {
	streamData := `data: {"choices":[{"delta":{"reasoning_content":"standard thinking"}}]}
data: {"choices":[{"delta":{"content":"answer"}}]}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reasonBuffer.String() != "standard thinking" {
		t.Errorf("reason mismatch, want %q, got %q", "standard thinking", reasonBuffer.String())
	}
	if outBuffer.String() != "answer" {
		t.Errorf("content mismatch, want %q, got %q", "answer", outBuffer.String())
	}
}

// TestProcessAIResponse_ReasoningContentTakesPrecedence 确保当同一响应同时包含
// reasoning_content 和 reasoning 时，reasoning_content 优先。
func TestProcessAIResponse_ReasoningContentTakesPrecedence(t *testing.T) {
	streamData := `data: {"choices":[{"delta":{"reasoning_content":"from reasoning_content","reasoning":"from reasoning"}}]}
data: {"choices":[{"delta":{"content":"done"}}]}
data: [DONE]`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(streamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	err := processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reasonBuffer.String() != "from reasoning_content" {
		t.Errorf("reason should prefer reasoning_content, got %q", reasonBuffer.String())
	}
	if outBuffer.String() != "done" {
		t.Errorf("content mismatch, got %q", outBuffer.String())
	}
}

// TestHandleResponsesSSEEvent_CompletedFallback 验证当上游不发 output_text.delta、
// 只在 response.completed 里给 output_text 时（packyapi codex 风格），
// handleResponsesSSEEvent 仍能通过 completed 事件补吐文本。
// 关键词: response.completed 补吐, packyapi codex 风格, 流式内容不丢
func TestHandleResponsesSSEEvent_CompletedFallback(t *testing.T) {
	outBuf := &bytes.Buffer{}
	reasonBuf := &bytes.Buffer{}
	state := newResponsesToolCallState()

	handleResponsesSSEEvent(map[string]any{
		"type": "response.output_item.added",
		"item": map[string]any{"type": "message", "id": "msg_1", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": ""}}},
		"output_index": 0,
	}, outBuf, reasonBuf, nil, state)

	handleResponsesSSEEvent(map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{"type": "message", "id": "msg_1", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": ""}}},
		"output_index": 0,
	}, outBuf, reasonBuf, nil, state)

	if outBuf.Len() != 0 {
		t.Fatalf("expected empty output before completed, got %q", outBuf.String())
	}

	handleResponsesSSEEvent(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"output_text": "PONG",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "PONG"}},
			}},
		},
	}, outBuf, reasonBuf, nil, state)

	if outBuf.String() != "PONG" {
		t.Errorf("expected output %q from completed fallback, got %q", "PONG", outBuf.String())
	}
}

// TestHandleResponsesSSEEvent_DeltaSuppressCompleted 验证已有 delta 事件后
// completed 不会重复吐出内容（防重复逻辑不回退）。
// 关键词: response.completed 防重复, anyTextStreamed
func TestHandleResponsesSSEEvent_DeltaSuppressCompleted(t *testing.T) {
	outBuf := &bytes.Buffer{}
	reasonBuf := &bytes.Buffer{}
	state := newResponsesToolCallState()

	handleResponsesSSEEvent(map[string]any{
		"type":         "response.output_text.delta",
		"delta":        "Hello",
		"output_index": 0,
		"item_id":      "msg_1",
	}, outBuf, reasonBuf, nil, state)

	handleResponsesSSEEvent(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"output_text": "Hello",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Hello"}},
			}},
		},
	}, outBuf, reasonBuf, nil, state)

	if outBuf.String() != "Hello" {
		t.Errorf("expected exactly one %q, got %q", "Hello", outBuf.String())
	}
}

// TestHandleResponsesSSEEvent_OutputTextDone 验证 response.output_text.done 事件
// 在没有 delta 的情况下能补吐文本。
// 关键词: response.output_text.done, 流式补吐
func TestHandleResponsesSSEEvent_OutputTextDone(t *testing.T) {
	outBuf := &bytes.Buffer{}
	reasonBuf := &bytes.Buffer{}
	state := newResponsesToolCallState()

	handleResponsesSSEEvent(map[string]any{
		"type": "response.output_text.done",
		"text": "done-text",
	}, outBuf, reasonBuf, nil, state)

	if outBuf.String() != "done-text" {
		t.Errorf("expected %q, got %q", "done-text", outBuf.String())
	}
	if !state.anyTextStreamed {
		t.Error("anyTextStreamed should be true after output_text.done")
	}
}

// TestHandleResponsesSSEEvent_ReasoningTextDone 验证 response.reasoning_text.done
// 在没有 reasoning delta 的情况下能补吐推理文本。
// 关键词: response.reasoning_text.done, 推理补吐
func TestHandleResponsesSSEEvent_ReasoningTextDone(t *testing.T) {
	outBuf := &bytes.Buffer{}
	reasonBuf := &bytes.Buffer{}
	state := newResponsesToolCallState()

	handleResponsesSSEEvent(map[string]any{
		"type": "response.reasoning_text.done",
		"text": "reason-done",
	}, outBuf, reasonBuf, nil, state)

	if reasonBuf.String() != "reason-done" {
		t.Errorf("expected %q, got %q", "reason-done", reasonBuf.String())
	}
	if !state.anyReasonStreamed {
		t.Error("anyReasonStreamed should be true after reasoning_text.done")
	}
}

// TestHandleResponsesSSEEvent_CompletedWithToolCalls 验证 completed 事件含
// function_call output 时 tool_call 不丢。
// 关键词: response.completed tool_calls, function_call 不丢
func TestHandleResponsesSSEEvent_CompletedWithToolCalls(t *testing.T) {
	outBuf := &bytes.Buffer{}
	reasonBuf := &bytes.Buffer{}
	state := newResponsesToolCallState()
	var receivedCalls []*ToolCall
	tcCallback := func(calls []*ToolCall) {
		receivedCalls = append(receivedCalls, calls...)
	}

	handleResponsesSSEEvent(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"output_text": "",
			"output": []any{
				map[string]any{
					"type":      "function_call",
					"id":        "fc_1",
					"call_id":   "call_abc",
					"name":      "get_weather",
					"arguments": `{"city":"Tokyo"}`,
				},
			},
		},
	}, outBuf, reasonBuf, tcCallback, state)

	if len(receivedCalls) == 0 {
		t.Fatal("expected tool call from completed event, got none")
	}
	if receivedCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected tool name %q, got %q", "get_weather", receivedCalls[0].Function.Name)
	}
	if receivedCalls[0].Function.Arguments != `{"city":"Tokyo"}` {
		t.Errorf("expected arguments %q, got %q", `{"city":"Tokyo"}`, receivedCalls[0].Function.Arguments)
	}
}
