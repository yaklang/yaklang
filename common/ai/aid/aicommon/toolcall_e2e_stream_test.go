package aicommon

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// mockSSEToolCallAction simulates an SSE response where the model
// streams tool_call arguments containing a complete action JSON.
// The arguments are split across multiple delta chunks to test
// the streaming pipe path.
const mockSSEToolCallAction = `HTTP/1.1 200 OK
Content-Type: text/event-stream
Connection: close

data: {"choices":[{"delta":{"content":"Let me help"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"execute_action","arguments":"{\"@action\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"require_tool\""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"identifier\":\"req_tool\""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"tool_require_payload\":\"read_file\"}"}}]}}]}

data: [DONE]

`

// aispecWithMockServer creates a ChatBaseOption that routes the HTTP
// request to the mock server at the given host:port.
func aispecWithMockServer(host string, port int) aispec.ChatBaseOption {
	return aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
		return []poc.PocConfigOption{
			poc.WithHost(host),
			poc.WithPort(port),
			poc.WithForceHTTPS(false),
			poc.WithTimeout(10),
		}, nil
	})
}

// TestE2E_SSE_ToolCall_To_ExtractAction verifies the full chain:
// SSE deltas → processAIResponse → toolCallArgumentsWriter (pipe) →
// toolCallArgsReader → ToolCallArgumentsStreamHandler →
// (forward to ExtractActionFromStream) → parsed Action with
// correct @action, identifier, and params.
//
// This test bridges the aispec layer (SSE parsing + pipe) with the
// aicommon layer (ExtractActionFromStream) to verify that the
// unified action parsing pipeline works end-to-end in functioncall mode.
func TestE2E_SSE_ToolCall_To_ExtractAction(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockSSEToolCallAction))

	var parsedAction *Action
	var parseErr error
	var wg sync.WaitGroup
	wg.Add(1)

	_, err := aispec.ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		aispec.WithChatBase_ToolCallCallback(func(toolCalls []*aispec.ToolCall) {
			// Logging only — arguments are streamed via the handler
		}),
		aispec.WithChatBase_ToolCallArgumentsStreamHandler(func(reader io.Reader) {
			defer wg.Done()
			// The reader contains the full tool_call arguments,
			// which is a complete action JSON. Feed it directly to
			// ExtractActionFromStream, exactly as the functioncall
			// mode postHandler would do.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			parsedAction, parseErr = ExtractValidActionFromStream(
				ctx,
				reader,
				"object", // action magic key
				WithActionAlias("require_tool", "directly_answer", "finish"),
			)
		}),
	)
	require.NoError(t, err)

	// Wait for the handler goroutine to finish parsing
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ToolCallArgumentsStreamHandler did not complete in time")
	}

	require.NoError(t, parseErr, "ExtractValidActionFromStream should succeed")
	require.NotNil(t, parsedAction, "parsed action should not be nil")

	// Verify the action type
	actionType := parsedAction.ActionType()
	require.Equal(t, "require_tool", actionType,
		"action type should be require_tool")

	// Verify identifier
	require.Equal(t, "req_tool", parsedAction.GetString("identifier"),
		"identifier should be parsed correctly")

	// Verify tool_require_payload
	require.Equal(t, "read_file", parsedAction.GetString("tool_require_payload"),
		"tool_require_payload should be parsed correctly")
}

// TestE2E_SSE_ToolCall_NoHandler_NoLeakage verifies that without a
// ToolCallArgumentsStreamHandler, tool_call arguments do not appear
// in the content stream (StreamHandler), and the request completes
// without blocking.
func TestE2E_SSE_ToolCall_NoHandler_NoLeakage(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockSSEToolCallAction))

	var streamContent string
	var streamMu sync.Mutex

	_, err := aispec.ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		aispec.WithChatBase_ToolCallCallback(func(toolCalls []*aispec.ToolCall) {
			// Just logging
		}),
		aispec.WithChatBase_StreamHandler(func(reader io.Reader) {
			data, _ := io.ReadAll(reader)
			streamMu.Lock()
			streamContent = string(data)
			streamMu.Unlock()
		}),
		// NO ToolCallArgumentsStreamHandler — drain path
	)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	streamMu.Lock()
	defer streamMu.Unlock()

	// Text content should be present
	require.True(t, strings.Contains(streamContent, "Let me help"),
		"stream should contain text content, got: %q", streamContent)

	// Tool call arguments must NOT leak into the stream
	require.False(t, strings.Contains(streamContent, "@action"),
		"stream must not contain tool_call arguments (no leakage)")
	require.False(t, strings.Contains(streamContent, "require_tool"),
		"stream must not contain tool_call arguments (no leakage)")
	require.False(t, strings.Contains(streamContent, "read_file"),
		"stream must not contain tool_call arguments (no leakage)")
}

// TestE2E_SSE_ToolCall_HandlerReceivesCompleteJSON verifies that
// the ToolCallArgumentsStreamHandler receives the complete
// concatenated arguments JSON from all streaming delta chunks.
func TestE2E_SSE_ToolCall_HandlerReceivesCompleteJSON(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockSSEToolCallAction))

	var handlerContent string
	var handlerMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	_, err := aispec.ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		aispec.WithChatBase_ToolCallCallback(func(toolCalls []*aispec.ToolCall) {
			// Logging only
		}),
		aispec.WithChatBase_ToolCallArgumentsStreamHandler(func(reader io.Reader) {
			defer wg.Done()
			data, _ := io.ReadAll(reader)
			handlerMu.Lock()
			handlerContent = string(data)
			handlerMu.Unlock()
		}),
	)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not complete in time")
	}

	handlerMu.Lock()
	defer handlerMu.Unlock()

	// The handler should have received the complete arguments JSON
	// assembled from all the streaming delta chunks
	require.Contains(t, handlerContent, "@action")
	require.Contains(t, handlerContent, "require_tool")
	require.Contains(t, handlerContent, "identifier")
	require.Contains(t, handlerContent, "req_tool")
	require.Contains(t, handlerContent, "tool_require_payload")
	require.Contains(t, handlerContent, "read_file")
}

// TestE2E_NonStreaming_ToolCall_To_ExtractAction verifies the
// non-streaming path: complete tool_call arguments in a single
// JSON response body flow through the pipe and handler to
// ExtractActionFromStream.
const mockNonStreamingToolCallAction = `HTTP/1.1 200 OK
Content-Type: application/json
Connection: close

{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"test","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"execute_action","arguments":"{\"@action\":\"directly_answer\",\"identifier\":\"answer_user\",\"answer_payload\":\"Hello from non-streaming\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`

func TestE2E_NonStreaming_ToolCall_To_ExtractAction(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockNonStreamingToolCallAction))

	var parsedAction *Action
	var parseErr error
	var wg sync.WaitGroup
	wg.Add(1)

	_, err := aispec.ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		aispec.WithChatBase_ToolCallCallback(func(toolCalls []*aispec.ToolCall) {
			// Logging only
		}),
		aispec.WithChatBase_ToolCallArgumentsStreamHandler(func(reader io.Reader) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			parsedAction, parseErr = ExtractValidActionFromStream(
				ctx,
				reader,
				"object",
				WithActionAlias("require_tool", "directly_answer", "finish"),
			)
		}),
	)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not complete in time")
	}

	require.NoError(t, parseErr)
	require.NotNil(t, parsedAction)

	require.Equal(t, "directly_answer", parsedAction.ActionType())
	require.Equal(t, "answer_user", parsedAction.GetString("identifier"))
	require.Equal(t, "Hello from non-streaming", parsedAction.GetString("answer_payload"))
}

// TestE2E_SSE_ToolCall_BufferReader verifies that the reader
// received by the handler can be used as a bytes.Reader (i.e.
// the pipe correctly buffers all data before the handler reads).
// This tests that io.ReadAll works correctly on the pipe reader.
func TestE2E_SSE_ToolCall_BufferReader(t *testing.T) {
	host, port := utils.DebugMockHTTP([]byte(mockSSEToolCallAction))

	var readData []byte
	var wg sync.WaitGroup
	wg.Add(1)

	_, err := aispec.ChatBase(
		"http://example.com/v1/chat/completions",
		"test-model",
		"test",
		aispecWithMockServer(host, port),
		aispec.WithChatBase_ToolCallCallback(func(toolCalls []*aispec.ToolCall) {}),
		aispec.WithChatBase_ToolCallArgumentsStreamHandler(func(reader io.Reader) {
			defer wg.Done()
			readData, _ = io.ReadAll(reader)
		}),
	)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not complete in time")
	}

	// Verify the data is valid JSON (concatenated delta fragments)
	require.NotEmpty(t, readData)
	require.True(t, bytes.Contains(readData, []byte("@action")))
	require.True(t, bytes.Contains(readData, []byte("require_tool")))
}