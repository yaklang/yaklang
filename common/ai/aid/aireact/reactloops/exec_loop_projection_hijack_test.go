package reactloops

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// This exercises the send path rather than calling ProjectAndObserve directly:
// callAITransaction -> AI callback -> ChatBase hook -> projection -> HTTP and
// the raw-request callback. The two action tags sit between message text in
// semi-dynamic-2, as they do in the generated function-call prompt.
func TestCallAITransactionProjectsInterleavedActionSchemasAtSend(t *testing.T) {
	aispec.ResetChatBaseHijackHooksForTest()
	aiprojection.ResetForTest()
	restoreThreshold := aiprojection.SetMinCachableUserSegmentBytesForTest(0)
	t.Cleanup(func() {
		restoreThreshold()
		aispec.ResetChatBaseHijackHooksForTest()
		aispec.RegisterChatBaseHijackHook(aiprojection.ProjectAndObserve)
	})

	var hookCalls atomic.Int32
	hookInputs := make(chan string, 1)
	hookResults := make(chan *aispec.ChatBaseHijackResult, 1)
	aispec.RegisterChatBaseHijackHook(func(model, msg string) *aispec.ChatBaseHijackResult {
		hookCalls.Add(1)
		hookInputs <- msg
		result := aiprojection.ProjectAndObserve(model, msg)
		hookResults <- result
		return result
	})

	serverBodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		serverBodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"accept\",\"text\":\"ok\"}"}}]}`))
	}))
	defer server.Close()

	actionTag := func(name string) string {
		tool := aispec.Tool{Type: "function", Function: aispec.ToolFunction{
			Name: name, Description: "schema-for-" + name,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		}}
		encoded, err := json.Marshal(tool)
		require.NoError(t, err)
		return "<|FUNCTION_CALL_ACTION_SCHEMA_" + name + "|>\n" + string(encoded) +
			"\n<|FUNCTION_CALL_ACTION_SCHEMA_END_" + name + "|>"
	}
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>stable-system<|PROMPT_SECTION_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>frozen-before<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|><|PROMPT_SECTION_semi-dynamic-1|>semi-one-text<|PROMPT_SECTION_END_semi-dynamic-1|><|AI_CACHE_SEMI_END_semi|>",
		"<|AI_CACHE_SEMI2_semi|><|PROMPT_SECTION_semi-dynamic-2|>before-first " + actionTag("accept") + " between-actions " + actionTag("inspect") + " after-second<|PROMPT_SECTION_END_semi-dynamic-2|><|AI_CACHE_SEMI2_END_semi|>",
		"<|PROMPT_SECTION_dynamic_n|>final-question<|PROMPT_SECTION_dynamic_END_n|>",
	}, "\n\n")

	ctx := context.Background()
	base := mock.NewMockedAIConfig(ctx).(*mock.MockedAIConfig)
	base.SetConfig("AiTransactionAutoRetry", 1)
	callbackPackets := make(chan []byte, 1)
	config := &fcTestConfig{MockedAIConfig: base}
	config.aiCallback = func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		require.Equal(t, prompt, req.GetPrompt(), "AI callback receives the unprojected prompt")
		output, err := aispec.ChatBase(server.URL, "projection-test-model", req.GetPrompt(),
			aispec.WithChatBase_DisableStream(true),
			aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }),
			aispec.WithChatBase_RawHTTPRequestResponseCallback(func(packet, _, _ []byte, _ *aispec.ChatUsage) {
				callbackPackets <- append([]byte(nil), packet...)
			}),
		)
		if err != nil {
			return nil, err
		}
		response := base.NewAIResponse()
		response.EmitOutputStream(strings.NewReader(output))
		response.Close()
		return response, nil
	}
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)
	loop.functionCallMode = true
	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
	loop.actions.Set("inspect", &LoopAction{ActionType: "inspect"})

	action, handler, err := loop.callAITransaction(&sync.WaitGroup{}, prompt, "n", nil)
	require.NoError(t, err)
	require.Equal(t, "accept", action.ActionType())
	require.Equal(t, "accept", handler.ActionType)
	require.EqualValues(t, 1, hookCalls.Load(), "ChatBase must dispatch ProjectAndObserve once")
	require.Len(t, hookInputs, 1)
	require.Equal(t, prompt, <-hookInputs, "the hook receives the same prompt as the AI callback")
	require.Len(t, hookResults, 1)
	hijacked := <-hookResults
	require.NotNil(t, hijacked)
	require.True(t, hijacked.IsHijacked)
	require.Len(t, serverBodies, 1)
	require.Len(t, callbackPackets, 1, "AI callback must receive the serialized request")

	var sent aispec.ChatMessage
	require.NoError(t, json.Unmarshal(<-serverBodies, &sent))
	packet := <-callbackPackets
	_, callbackBody, found := bytes.Cut(packet, []byte("\r\n\r\n"))
	require.True(t, found, "raw-request callback must carry HTTP headers and body")
	var callbackRequest aispec.ChatMessage
	require.NoError(t, json.Unmarshal(callbackBody, &callbackRequest))
	assert.Equal(t, sent, callbackRequest, "callback ChatDetails must match the provider request")

	// Cache boundaries remain in order while action schemas are extracted from
	// the semi-dynamic-2 user message and become native functions in prompt order.
	require.Len(t, sent.Messages, 5)
	require.Len(t, hijacked.Messages, 5)
	assert.Equal(t, []string{"system", "user", "user", "user", "user"}, []string{
		sent.Messages[0].Role, sent.Messages[1].Role, sent.Messages[2].Role, sent.Messages[3].Role, sent.Messages[4].Role,
	})
	for index, marker := range []string{"stable-system", "frozen-before", "semi-one-text", "before-first", "final-question"} {
		encoded, marshalErr := json.Marshal(sent.Messages[index].Content)
		require.NoError(t, marshalErr)
		assert.Contains(t, string(encoded), marker)
	}
	for _, marker := range []string{"between-actions", "after-second"} {
		index := 3
		encoded, marshalErr := json.Marshal(sent.Messages[index].Content)
		require.NoError(t, marshalErr)
		assert.Contains(t, string(encoded), marker)
	}
	for index, hasCacheControl := range []bool{true, true, false, true, false} {
		encoded, marshalErr := json.Marshal(sent.Messages[index].Content)
		require.NoError(t, marshalErr)
		assert.Equal(t, hasCacheControl, bytes.Contains(encoded, []byte(`"cache_control"`)), "cache boundary at message %d", index)
	}
	projectedJSON, err := json.Marshal(hijacked.Messages)
	require.NoError(t, err)
	sentJSON, err := json.Marshal(sent.Messages)
	require.NoError(t, err)
	assert.JSONEq(t, string(projectedJSON), string(sentJSON), "projection must return the details actually sent")
	assert.NotContains(t, string(sentJSON), "FUNCTION_CALL_ACTION_SCHEMA")
	assert.NotContains(t, string(sentJSON), "schema-for-accept")
	assert.NotContains(t, string(sentJSON), "schema-for-inspect")
	if assert.Len(t, sent.Tools, 2, "one native function per action tag") {
		assert.Equal(t, "accept", sent.Tools[0].Function.Name)
		assert.Equal(t, "inspect", sent.Tools[1].Function.Name)
	}
}
