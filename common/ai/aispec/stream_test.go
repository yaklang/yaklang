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

	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)

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

	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer, nil, nil, nil, nil)

	t.Logf("流式内容输出: %s", outBuffer.String())
	t.Logf("流式推理输出: %s", reasonBuffer.String())
}

func TestAppendStreamHandlerPoCOptionEx(t *testing.T) {
	// 测试流式和非流式选项创建
	outReader, reasonReader, opts, cancel := appendStreamHandlerPoCOptionEx(true, []poc.PocConfigOption{}, nil, nil, nil, nil)
	defer cancel()

	if outReader == nil || reasonReader == nil {
		t.Error("输出读取器不应为nil")
	}

	if len(opts) == 0 {
		t.Error("应该添加了处理选项")
	}

	// 测试非流式
	outReader2, reasonReader2, opts2, cancel2 := appendStreamHandlerPoCOptionEx(false, []poc.PocConfigOption{}, nil, nil, nil, nil)
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
