package reactloops

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func newCallAILoopTransactionTestLoop(t *testing.T, functionMode bool, respond func(*aicommon.AIRequest, *aispec.AIConfig) (*aicommon.AIResponse, error)) *ReActLoop {
	t.Helper()
	base := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
	base.SetConfig("AiTransactionAutoRetry", 1)
	config := &fcTestConfig{MockedAIConfig: base}
	config.aiCallback = func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return respond(req, aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...))
	}
	invoker := mock.NewMockInvoker(context.Background())
	invoker.SetConfig(config)
	loop := NewMinimalReActLoop(config, invoker)
	loop.functionCallMode = functionMode
	loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	return loop
}

func TestCallAILoopTransactionNormalMode(t *testing.T) {
	const raw = `{"action":"accept","text":"ready"}`
	var verified int
	loop := newCallAILoopTransactionTestLoop(t, false, func(req *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		require.Equal(t, "normal prompt", req.GetPrompt())
		require.False(t, req.IsToolCallArgumentsStreamEnabled())
		require.Nil(t, cfg.ToolCallCallback, "normal mode must not intercept native tool calls")
		resp := aicommon.NewAIResponse(nil)
		resp.EmitOutputStream(strings.NewReader(raw))
		cfg.FinishReasonCallback("stop", []byte(`{"choices":[{"finish_reason":"stop"}]}`))
		resp.Close()
		return resp, nil
	})
	loop.actions.Set("accept", &LoopAction{ActionType: "accept", ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
		verified++
		require.Equal(t, "ready", action.GetString("text"))
		return nil
	}})
	var generalCalls, functionCalls int
	calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "normal prompt", "nonce", nil,
		func(io.Reader, io.Reader) { generalCalls++ },
		func(string, string, io.Reader, io.Reader) { functionCalls++ },
	)
	require.NoError(t, err)
	require.Equal(t, LoopStopNormal, stop)
	require.Len(t, calls, 1)
	require.Equal(t, "accept", calls[0].Action.ActionType())
	require.Equal(t, "accept", calls[0].LoopAction.ActionType)
	require.Equal(t, 1, verified)
	require.Zero(t, generalCalls)
	require.Zero(t, functionCalls)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.True(t, descriptor.WaitComplete(ctx))
	snapshot := descriptor.Snapshot()
	require.Equal(t, "normal", snapshot.Mode)
	require.Equal(t, "stop", snapshot.ProviderFinishReason)
	require.JSONEq(t, `{"choices":[{"finish_reason":"stop"}]}`, snapshot.RawResponseBody)
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(snapshot.ResponseJSON), &response))
	require.Equal(t, raw, response["content"])
}

func TestCallAILoopTransactionNormalDoesNotWaitForProviderFinish(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseProvider()
	loop := newCallAILoopTransactionTestLoop(t, false, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		resp.EmitOutputStream(strings.NewReader(`{"@action":"accept","text":"ready"}`))
		resp.Close()
		go func() {
			<-release
			cfg.FinishReasonCallback("stop", []byte(`{"choices":[{"finish_reason":"stop"}]}`))
		}()
		return resp, nil
	})
	loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
	type result struct {
		calls []LoopCall
		stop  LoopStopReason
		desc  *LoopResultDescriptor
		err   error
	}
	done := make(chan result, 1)
	go func() {
		calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
			func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
		done <- result{calls, stop, descriptor, err}
	}()
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.Equal(t, LoopStopNormal, got.stop)
		require.Len(t, got.calls, 1)
		require.False(t, got.desc.Snapshot().Complete, "provenance may still be filling after the action is returned")
		require.Empty(t, got.desc.Snapshot().ProviderFinishReason)
		releaseProvider()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.True(t, got.desc.WaitComplete(ctx))
		require.Equal(t, "stop", got.desc.Snapshot().ProviderFinishReason)
	case <-time.After(2 * time.Second):
		releaseProvider()
		t.Fatal("normal mode waited for the provider's terminal reason")
	}
}

func TestCallAILoopTransactionFunctionModeInterleavedCalls(t *testing.T) {
	// Provider content/reasoning become the general display streams. Each
	// interleaved tool_calls[index] becomes one LoopCall with its own ID and
	// complete JSON arguments; content is never parsed as an action.
	const content = "The next actions follow. This is not action JSON."
	const reason = "Inspect both inputs first."
	loop := newCallAILoopTransactionTestLoop(t, true, func(req *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		require.Equal(t, "function prompt", req.GetPrompt())
		require.False(t, req.IsToolCallArgumentsStreamEnabled(), "arguments must not be mixed into content")
		require.NotNil(t, cfg.ToolCallCallback)
		require.NotNil(t, cfg.FinishReasonCallback)
		resp := aicommon.NewAIResponse(nil)
		resp.EmitReasonStream(strings.NewReader(reason))
		resp.EmitOutputStream(strings.NewReader(content))
		cfg.ToolCallCallback([]*aispec.ToolCall{
			{Index: 1, ID: "call_b", Type: "function", Description: "Check B", Function: aispec.FuncReturn{Name: "check_b", Arguments: `{"value":`}},
			{Index: 0, ID: "call_a", Type: "function", Function: aispec.FuncReturn{Name: "check_a", Arguments: `{"value":`}},
		})
		cfg.ToolCallCallback([]*aispec.ToolCall{
			{Index: 0, Function: aispec.FuncReturn{Arguments: `"A"`}},
			{Index: 1, Function: aispec.FuncReturn{Arguments: `"B"`}},
			{Index: 0, Function: aispec.FuncReturn{Arguments: `}`}},
			{Index: 1, Function: aispec.FuncReturn{Arguments: `}`}},
		})
		cfg.FinishReasonCallback("tool_calls", []byte(`{"choices":[{"finish_reason":"tool_calls"}]}`))
		resp.Close()
		return resp, nil
	})
	var verified []string
	for _, name := range []string{"check_a", "check_b"} {
		name := name
		loop.actions.Set(name, &LoopAction{ActionType: name, ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
			verified = append(verified, name)
			require.Equal(t, name, action.ActionType())
			return nil
		}})
	}
	var generalOutput, generalReason string
	var streamedMu sync.Mutex
	streamed := make(map[string][2]string)
	calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "function prompt", "nonce", nil,
		func(outputReader, reasonReader io.Reader) {
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); b, _ := io.ReadAll(outputReader); generalOutput = string(b) }()
			go func() { defer wg.Done(); b, _ := io.ReadAll(reasonReader); generalReason = string(b) }()
			wg.Wait()
		},
		func(id, name string, descriptionReader, argumentReader io.Reader) {
			var description, arguments []byte
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); description, _ = io.ReadAll(descriptionReader) }()
			go func() { defer wg.Done(); arguments, _ = io.ReadAll(argumentReader) }()
			wg.Wait()
			streamedMu.Lock()
			streamed[id] = [2]string{name + ":" + string(description), string(arguments)}
			streamedMu.Unlock()
		},
	)
	require.NoError(t, err)
	require.Equal(t, LoopStopToolCalls, stop)
	require.Equal(t, content, generalOutput)
	require.Equal(t, reason, generalReason)
	require.Equal(t, []string{"check_a", "check_b"}, verified)
	require.Len(t, calls, 2)
	require.Equal(t, "call_a", calls[0].ToolCallID)
	require.Equal(t, "check_a", calls[0].Action.ActionType())
	require.Equal(t, "A", calls[0].Action.GetString("value"))
	require.Equal(t, "call_b", calls[1].ToolCallID)
	require.Equal(t, "B", calls[1].Action.GetString("value"))
	require.Equal(t, "Check B", calls[1].Description)
	require.Equal(t, [2]string{"check_a:", `{"value":"A"}`}, streamed["call_a"])
	require.Equal(t, [2]string{"check_b:Check B", `{"value":"B"}`}, streamed["call_b"])
	snapshot := descriptor.Snapshot()
	require.True(t, snapshot.Complete)
	require.Equal(t, "tool_calls", snapshot.ProviderFinishReason)
	require.JSONEq(t, `{"choices":[{"finish_reason":"tool_calls"}]}`, snapshot.RawResponseBody)
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(snapshot.ResponseJSON), &response))
	require.Equal(t, content, response["content"])
	require.Equal(t, reason, response["reasoning_content"])
	require.Len(t, response["tool_calls"], 2)
	secondCall := response["tool_calls"].([]any)[1].(map[string]any)
	require.Equal(t, "Check B", secondCall["description"])
}

func TestLoopToolCallCollectorReusedIndexKeepsCallsByID(t *testing.T) {
	streamed := make(map[string]string)
	var mu sync.Mutex
	collector := newLoopToolCallCollector(func(id, _ string, description, arguments io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, description) }()
		var payload []byte
		go func() { defer wg.Done(); payload, _ = io.ReadAll(arguments) }()
		wg.Wait()
		mu.Lock()
		streamed[id] = string(payload)
		mu.Unlock()
	})
	// The provider reuses index 0, but each ID denotes a distinct call.
	collector.add([]*aispec.ToolCall{{Index: 0, ID: "call_a", Function: aispec.FuncReturn{Name: "check_a", Arguments: `{"value":`}}})
	collector.add([]*aispec.ToolCall{{Index: 0, ID: "call_b", Function: aispec.FuncReturn{Name: "check_b", Arguments: `{"value":`}}})
	collector.add([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Arguments: `"B"}`}}})
	// An ID-bearing delta can still return to the earlier call, even with a changed index.
	collector.add([]*aispec.ToolCall{{Index: 1, ID: "call_a", Function: aispec.FuncReturn{Arguments: `"A"}`}}})
	calls, err := collector.finish()
	require.NoError(t, err)
	require.Len(t, calls, 2)
	require.Equal(t, "call_a", calls[0].ID)
	require.JSONEq(t, `{"value":"A"}`, calls[0].Function.Arguments)
	require.Equal(t, "call_b", calls[1].ID)
	require.JSONEq(t, `{"value":"B"}`, calls[1].Function.Arguments)
	require.Equal(t, calls[0].Function.Arguments, streamed["call_a"])
	require.Equal(t, calls[1].Function.Arguments, streamed["call_b"])
}

func TestCallAILoopTransactionFunctionModeDiscardsEarlierProviderResponse(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		require.NotNil(t, cfg.RawHTTPResponseHeaderCallback)
		require.NotNil(t, cfg.ToolCallCallback)
		// A provider fallback can produce two HTTP responses within one
		// AIRequest. Only the second response belongs to the accepted result.
		cfg.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "discarded", Type: "function",
			Function: aispec.FuncReturn{Name: "accept", Arguments: `{"value":"old"}`}}})
		cfg.FinishReasonCallback("tool_calls", []byte("discarded response"))
		cfg.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "accepted", Type: "function",
			Function: aispec.FuncReturn{Name: "accept", Arguments: `{"value":"new"}`}}})
		cfg.FinishReasonCallback("tool_calls", []byte("accepted response"))
		resp := aicommon.NewAIResponse(nil)
		resp.EmitOutputStream(strings.NewReader(""))
		resp.Close()
		return resp, nil
	})
	loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.NoError(t, err)
	require.Equal(t, LoopStopToolCalls, stop)
	require.Len(t, calls, 1)
	require.Equal(t, "accepted", calls[0].ToolCallID)
	require.Equal(t, "new", calls[0].Action.GetString("value"))
	require.Equal(t, "accepted response", descriptor.Snapshot().RawResponseBody)
	require.NotContains(t, descriptor.Snapshot().ResponseJSON, "discarded")
}

func TestCallAILoopTransactionFunctionModeRejectsMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		finish  string
		deltas  []*aispec.ToolCall
		wantErr string
	}{
		{"normal stop", "stop", []*aispec.ToolCall{{Index: 0, ID: "a", Function: aispec.FuncReturn{Name: "accept", Arguments: `{}`}}}, "expected tool_calls"},
		{"truncated stop", "length", []*aispec.ToolCall{{Index: 0, ID: "a", Function: aispec.FuncReturn{Name: "accept", Arguments: `{}`}}}, "expected tool_calls"},
		{"malformed arguments", "tool_calls", []*aispec.ToolCall{{Index: 0, ID: "a", Function: aispec.FuncReturn{Name: "accept", Arguments: `{`}}}, "invalid JSON arguments"},
		{"unknown action", "tool_calls", []*aispec.ToolCall{{Index: 0, ID: "a", Function: aispec.FuncReturn{Name: "missing", Arguments: `{}`}}}, "native function has no registered loop action"},
		{"missing id", "tool_calls", []*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Name: "accept", Arguments: `{}`}}}, "incomplete tool call"},
		{"duplicate complete arguments", "tool_calls", []*aispec.ToolCall{{Index: 0, ID: "a", Function: aispec.FuncReturn{Name: "accept", Arguments: `{}`}}, {Index: 1, ID: "a", Function: aispec.FuncReturn{Name: "accept", Arguments: `{}`}}}, "trailing JSON arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				resp := aicommon.NewAIResponse(nil)
				resp.EmitOutputStream(strings.NewReader("display only"))
				cfg.ToolCallCallback(tc.deltas)
				cfg.FinishReasonCallback(tc.finish, nil)
				resp.Close()
				return resp, nil
			})
			loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
			calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
				func(out, reason io.Reader) {
					var wg sync.WaitGroup
					wg.Add(2)
					go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, out) }()
					go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, reason) }()
					wg.Wait()
				},
				func(_, _ string, description, arguments io.Reader) {
					var wg sync.WaitGroup
					wg.Add(2)
					go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, description) }()
					go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, arguments) }()
					wg.Wait()
				},
			)
			require.ErrorContains(t, err, tc.wantErr)
			require.Equal(t, LoopStopAbused, stop)
			require.Empty(t, calls, "no partial batch may escape validation")
			require.True(t, descriptor.Snapshot().Complete)
			require.Contains(t, descriptor.Snapshot().Error, tc.wantErr)
		})
	}
}

func TestCallAILoopTransactionRequiresBothCallbacks(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(*aicommon.AIRequest, *aispec.AIConfig) (*aicommon.AIResponse, error) {
		t.Fatal("request must not be sent without both callbacks")
		return nil, nil
	})
	_, stop, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil, nil,
		func(string, string, io.Reader, io.Reader) {})
	require.Equal(t, LoopStopAbused, stop)
	require.ErrorContains(t, err, "requires a stream waitgroup and both output callbacks")
	_, stop, _, err = loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		func(io.Reader, io.Reader) {}, nil)
	require.Equal(t, LoopStopAbused, stop)
	require.ErrorContains(t, err, "requires a stream waitgroup and both output callbacks")
}

func TestCallAILoopTransactionFunctionModeRetryKeepsAcceptedResponse(t *testing.T) {
	var attempts int
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		attempts++
		id, arguments := "bad_attempt", `{"value":`
		if attempts == 2 {
			id, arguments = "accepted_call", `{"value":"accepted"}`
		}
		resp := aicommon.NewAIResponse(nil)
		resp.EmitOutputStream(strings.NewReader("content-" + id))
		resp.EmitReasonStream(strings.NewReader("reason-" + id))
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: id, Type: "function",
			Function: aispec.FuncReturn{Name: "accept", Arguments: arguments}}})
		cfg.FinishReasonCallback("tool_calls", []byte(id))
		resp.Close()
		return resp, nil
	})
	loop.config.(*fcTestConfig).SetConfig("AiTransactionAutoRetry", 2)
	loop.actions.Set("accept", &LoopAction{ActionType: "accept"})
	drainGeneral := func(output, reason io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, output) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, reason) }()
		wg.Wait()
	}
	drainCall := func(_, _ string, description, arguments io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, description) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, arguments) }()
		wg.Wait()
	}
	calls, stop, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil, drainGeneral, drainCall)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, LoopStopToolCalls, stop)
	require.Len(t, calls, 1)
	require.Equal(t, "accepted_call", calls[0].ToolCallID)
	require.Equal(t, "accepted", calls[0].Action.GetString("value"))
	require.Equal(t, "accepted_call", descriptor.Snapshot().RawResponseBody)
	require.Contains(t, descriptor.Snapshot().ResponseJSON, "content-accepted_call")
	require.NotContains(t, descriptor.Snapshot().ResponseJSON, "bad_attempt")
	require.Equal(t, "reason-accepted_call", loop.takeModelThinkingForTimeline())
}
