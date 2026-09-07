package aispec

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// mockStreamingToolCallRsp simulates an SSE response with streaming
// tool_calls deltas. The model emits content "Sure" then a tool_call
// for "execute_action" with arguments streamed in fragments.
const mockStreamingToolCallRsp = `HTTP/1.1 200 OK
Content-Type: text/event-stream
Connection: close

data: {"choices":[{"delta":{"content":"Sure"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"execute_action","arguments":"{\"@action\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"require_tool\",\"tool\":\"read_file\""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"params\":{\"path\":\"/tmp/foo\"}}}"}}]}}]}}

data: [DONE]

`

// mockNonStreamingToolCallRsp simulates a non-streaming response with
// a complete tool_call.
const mockNonStreamingToolCallRsp = `HTTP/1.1 200 OK
Content-Type: application/json
Connection: close

{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"test","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"execute_action","arguments":"{\"@action\":\"require_tool\",\"tool\":\"read_file\",\"params\":{\"path\":\"/tmp/foo\"}}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}
`

// --- Test #16: No leakage when ToolCallArgumentsStreamHandler is nil ---

// TestToolCallArguments_NoLeakage_Streaming verifies that when
// ToolCallArgumentsStreamHandler is NOT set, tool_call arguments do
// NOT appear in the output stream (StreamHandler). Only the
// ToolCallCallback should receive them. This preserves the "no
// leakage" contract for non-functioncall callers.
func TestToolCallArguments_NoLeakage_Streaming(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockStreamingToolCallRsp))

	var callbackArgs string
	var callbackMu sync.Mutex
	var streamContent string
	var streamMu sync.Mutex

	_, err := ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		WithChatBase_StreamHandler(func(reader io.Reader) {
			data, _ := io.ReadAll(reader)
			streamMu.Lock()
			streamContent = string(data)
			streamMu.Unlock()
		}),
		WithChatBase_ToolCallCallback(func(toolCalls []*ToolCall) {
			callbackMu.Lock()
			defer callbackMu.Unlock()
			for _, tc := range toolCalls {
				callbackArgs += tc.Function.Arguments
			}
		}),
		// NO ToolCallArgumentsStreamHandler — must not leak
	)
	require.NoError(t, err)

	// Wait for stream handler to finish
	time.Sleep(100 * time.Millisecond)

	callbackMu.Lock()
	streamMu.Lock()

	// ToolCallCallback should have received the arguments
	assert.Contains(t, callbackArgs, "@action", "ToolCallCallback should receive arguments")
	assert.Contains(t, callbackArgs, "read_file", "ToolCallCallback should receive tool name")

	// Stream content should contain the text content but NOT the tool_call arguments
	assert.Contains(t, streamContent, "Sure", "Stream should contain text content")
	assert.NotContains(t, streamContent, "@action", "Stream must NOT contain tool_call arguments (no leakage)")
	assert.NotContains(t, streamContent, "read_file", "Stream must NOT contain tool name from arguments")

	streamMu.Unlock()
	callbackMu.Unlock()
}

// TestToolCallArguments_NoLeakage_NonStreaming verifies the same
// no-leakage contract for non-streaming responses.
func TestToolCallArguments_NoLeakage_NonStreaming(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockNonStreamingToolCallRsp))

	var callbackArgs string
	var callbackMu sync.Mutex

	res, err := ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		WithChatBase_ToolCallCallback(func(toolCalls []*ToolCall) {
			callbackMu.Lock()
			defer callbackMu.Unlock()
			for _, tc := range toolCalls {
				callbackArgs += tc.Function.Arguments
			}
		}),
		// NO ToolCallArgumentsStreamHandler — must not leak
	)
	require.NoError(t, err)

	callbackMu.Lock()
	defer callbackMu.Unlock()

	// ToolCallCallback should have received the arguments
	assert.Contains(t, callbackArgs, "@action", "ToolCallCallback should receive arguments")
	assert.Contains(t, callbackArgs, "read_file", "ToolCallCallback should receive tool name")

	// Response string should NOT contain tool_call arguments
	assert.NotContains(t, res, "@action", "Response must NOT contain tool_call arguments (no leakage)")
	assert.NotContains(t, res, "read_file", "Response must NOT contain tool name from arguments")
}

// --- Test #2: Handler routing + drain ---

// TestToolCallArguments_StreamHandler_Routing verifies that when
// ToolCallArgumentsStreamHandler IS set, the tool_call arguments
// flow through it as a reader, and the concatenated result is the
// complete arguments JSON.
func TestToolCallArguments_StreamHandler_Routing(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockStreamingToolCallRsp))

	var handlerContent string
	var handlerMu sync.Mutex
	var callbackArgs string
	var callbackMu sync.Mutex

	_, err := ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		WithChatBase_ToolCallCallback(func(toolCalls []*ToolCall) {
			callbackMu.Lock()
			defer callbackMu.Unlock()
			for _, tc := range toolCalls {
				callbackArgs += tc.Function.Arguments
			}
		}),
		WithChatBase_ToolCallArgumentsStreamHandler(func(reader io.Reader) {
			data, _ := io.ReadAll(reader)
			handlerMu.Lock()
			handlerContent = string(data)
			handlerMu.Unlock()
		}),
	)
	require.NoError(t, err)

	// Wait for handler to finish
	time.Sleep(200 * time.Millisecond)

	handlerMu.Lock()
	callbackMu.Lock()
	defer handlerMu.Unlock()
	defer callbackMu.Unlock()

	// The handler should have received the complete arguments JSON
	// (concatenation of all streaming deltas)
	assert.Contains(t, handlerContent, "@action", "Handler should receive @action in arguments")
	assert.Contains(t, handlerContent, "read_file", "Handler should receive tool name in arguments")
	assert.Contains(t, handlerContent, "/tmp/foo", "Handler should receive params in arguments")

	// The callback should also have received the arguments (for logging)
	assert.Contains(t, callbackArgs, "@action", "Callback should also receive arguments")
}

// TestToolCallArguments_Drain_NoBlock verifies that when
// ToolCallArgumentsStreamHandler is nil, the internal pipe is
// properly drained and does not block the request from completing.
func TestToolCallArguments_Drain_NoBlock(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockStreamingToolCallRsp))

	done := make(chan struct{})
	go func() {
		_, err := ChatBase(
			"http://example.com/v1/chat/completions",
			"test-model",
			"test",
			aispecWithMockServer(host, port),
			WithChatBase_ToolCallCallback(func(toolCalls []*ToolCall) {
				// Just consume
			}),
			// NO ToolCallArgumentsStreamHandler — drain path
		)
		assert.NoError(t, err)
		close(done)
	}()

	select {
	case <-done:
		// Success — did not block
	case <-time.After(10 * time.Second):
		t.Fatal("ChatBase blocked when ToolCallArgumentsStreamHandler is nil — drain failed")
	}
}

// --- Helper ---

func aispecWithMockServer(host string, port int) ChatBaseOption {
	return WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
		return []poc.PocConfigOption{
			poc.WithHost(host),
			poc.WithPort(port),
			poc.WithForceHTTPS(false),
			poc.WithTimeout(5),
		}, nil
	})
}
