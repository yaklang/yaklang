package aispec

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestProcessNonStreamResponse(t *testing.T) {
	// 测试非流式响应处理函数
	nonStreamData := `{"choices":[{"message":{"content":"Hello World","reasoning_content":"This is my reasoning process"}}]}`

	mockResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n")
	mockCloser := io.NopCloser(strings.NewReader(nonStreamData))

	outBuffer := &bytes.Buffer{}
	reasonBuffer := &bytes.Buffer{}

	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer)

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

	processAIResponse(mockResponse, mockCloser, outBuffer, reasonBuffer)

	t.Logf("流式内容输出: %s", outBuffer.String())
	t.Logf("流式推理输出: %s", reasonBuffer.String())
}

func TestAppendStreamHandlerPoCOptionEx(t *testing.T) {
	// 测试流式和非流式选项创建
	outReader, reasonReader, opts, cancel := appendStreamHandlerPoCOptionEx(true, []poc.PocConfigOption{})
	defer cancel()

	if outReader == nil || reasonReader == nil {
		t.Error("输出读取器不应为nil")
	}

	if len(opts) == 0 {
		t.Error("应该添加了处理选项")
	}

	// 测试非流式
	outReader2, reasonReader2, opts2, cancel2 := appendStreamHandlerPoCOptionEx(false, []poc.PocConfigOption{})
	defer cancel2()

	if outReader2 == nil || reasonReader2 == nil {
		t.Error("非流式输出读取器不应为nil")
	}

	if len(opts2) == 0 {
		t.Error("非流式应该添加了处理选项")
	}
}
