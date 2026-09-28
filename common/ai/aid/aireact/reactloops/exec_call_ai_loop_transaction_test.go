package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
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
	require.Equal(t, "A", calls[0].Action.GetParams().GetString("value"))
	require.Equal(t, "call_b", calls[1].ToolCallID)
	require.Equal(t, "B", calls[1].Action.GetString("value"))
	require.Equal(t, "B", calls[1].Action.GetParams().GetString("value"))
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

func TestNativeFunctionCallStatusNamesAppearBeforeProviderFinishes(t *testing.T) {
	var statusMu sync.Mutex
	var statuses []aicommon.StatusPayload
	var sawFirst, sawBatch bool
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, Type: "function",
			Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName}}})
		statusMu.Lock()
		sawFirst = len(statuses) > 0 && statuses[len(statuses)-1].Value == "正在准备调整待办事项"
		statusMu.Unlock()
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "todo", Function: aispec.FuncReturn{Arguments: `{"todo_delta":{}`}}})
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 1, ID: "answer", Type: "function",
			Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"你好"}`}}})
		statusMu.Lock()
		sawBatch = len(statuses) > 0 && statuses[len(statuses)-1].Code == "action.batch.preparing" &&
			strings.Contains(statuses[len(statuses)-1].Value, "调整待办事项、回复用户")
		statusMu.Unlock()
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, Function: aispec.FuncReturn{Arguments: `}`}}})
		cfg.FinishReasonCallback("tool_calls", nil)
		resp.Close()
		return resp, nil
	})
	loop.actions.Set(nativeAdjustTodolistActionName, loopAction_AdjustTodolistNative)
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer"})
	loop.emitter = aicommon.NewEmitter("native-status-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.NodeId == "status" {
			var status aicommon.StatusPayload
			if err := json.Unmarshal(event.Content, &status); err != nil {
				return nil, err
			}
			statusMu.Lock()
			statuses = append(statuses, status)
			statusMu.Unlock()
		}
		return event, nil
	})
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		func(io.Reader, io.Reader) {}, func(_, _ string, description, arguments io.Reader) {
			_, _ = io.Copy(io.Discard, description)
			_, _ = io.Copy(io.Discard, arguments)
		})
	require.NoError(t, err)
	require.Len(t, calls, 2)
	require.True(t, sawFirst, "first action should be visible during argument generation")
	require.True(t, sawBatch, "batch names should be visible before provider completion")
	statusMu.Lock()
	defer statusMu.Unlock()
	var batchPreparing int
	var localizedAction bool
	for _, status := range statuses {
		require.NotEqual(t, "正在梳理思路", status.Value)
		if status.Value == "正在准备调整待办事项" {
			require.NotNil(t, status.ValueI18n)
			require.Equal(t, "Preparing: updating the task list", status.ValueI18n.En)
			localizedAction = true
		}
		if status.Code == "action.batch.preparing" {
			batchPreparing++
		}
	}
	require.True(t, localizedAction)
	require.Equal(t, 2, batchPreparing, "one update on discovery and one after validation")
}

func TestCallAILoopTransactionNativeArgumentsEmitDeclaredFields(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{
			{Index: 0, ID: "answer", Type: "function", Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"identifier":"greet_reply","human_readable_thought":"planning","nested":{"answer_payload":"do not show"},"answer_payload":"你好，\n# 报告"}`}},
			{Index: 1, ID: "status", Type: "function", Function: aispec.FuncReturn{Name: "status", Arguments: `{"status_payload":{"message":"ready"}}`}},
		})
		cfg.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
		resp.Close()
		return resp, nil
	})
	loop.SetCurrentTask(newMockSimpleTask("native-answer", "1"))
	loop.streamFields = omap.NewEmptyOrderedMap[string, *LoopStreamField]()
	loop.streamFields.Set("human_readable_thought", &LoopStreamField{FieldName: "human_readable_thought", AINodeId: "re-act-loop-thought"})
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer", StreamFields: loopAction_DirectlyAnswer.StreamFields})
	loop.actions.Set("status", &LoopAction{ActionType: "status", StreamFields: []*LoopStreamField{{
		FieldName: "message", AINodeId: "native-status", Prefix: "state", ContentType: aicommon.TypeTextPlain,
	}}})
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	loop.emitter = aicommon.NewEmitter("native-fields", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return event, nil
	})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.NoError(t, err)
	require.Len(t, calls, 2)
	mu.Lock()
	defer mu.Unlock()
	var answer, status, thought string
	var answerStarts, statusStarts, thoughtStarts int
	for _, event := range events {
		switch event.NodeId {
		case "re-act-loop-answer-payload":
			require.Equal(t, aicommon.TypeTextMarkdown, event.ContentType)
			require.Equal(t, "answer_payload", event.VizSource)
			require.Equal(t, "1", event.TaskIndex)
			if event.Type == schema.EVENT_TYPE_STREAM_START {
				answerStarts++
			}
			answer += string(event.StreamDelta)
		case "native-status":
			if event.Type == schema.EVENT_TYPE_STREAM_START {
				statusStarts++
			}
			status += string(event.StreamDelta)
		case "re-act-loop-thought":
			require.Equal(t, "human_readable_thought", event.VizSource)
			if event.Type == schema.EVENT_TYPE_STREAM_START {
				thoughtStarts++
			}
			thought += string(event.StreamDelta)
		}
	}
	require.Equal(t, 1, answerStarts)
	require.Equal(t, "你好，\n# 报告", answer)
	require.Equal(t, 1, statusStarts)
	require.Equal(t, "state: ready", status)
	require.Equal(t, 1, thoughtStarts)
	require.Equal(t, "planning", thought)
}

func TestCallAILoopTransactionNativeRejectedArgumentsReportFailure(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "answer", Type: "function",
			Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"candidate answer"}`}}})
		cfg.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
		resp.Close()
		return resp, nil
	})
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer",
		StreamFields:   loopAction_DirectlyAnswer.StreamFields,
		ActionVerifier: func(*ReActLoop, *aicommon.Action) error { return fmt.Errorf("rejected") },
	})
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	loop.emitter = aicommon.NewEmitter("native-rejected", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return event, nil
	})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	_, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.ErrorContains(t, err, "rejected")
	mu.Lock()
	defer mu.Unlock()
	var failureStatus bool
	for _, event := range events {
		if event.NodeId == "status" && strings.Contains(string(event.Content), `"code":"action.failed"`) {
			failureStatus = true
		}
		require.NotEqual(t, schema.EVENT_TYPE_RESULT, event.Type)
	}
	require.True(t, failureStatus)
}

func TestCallAILoopTransactionNativeArgumentsStreamBeforeFinish(t *testing.T) {
	firstDelta := make(chan string, 1)
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "answer", Type: "function",
			Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"first `}}})
		select {
		case got := <-firstDelta:
			require.Equal(t, "first", got)
		case <-time.After(2 * time.Second):
			t.Error("first argument fragment was not emitted before the provider finished")
		}
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "answer",
			Function: aispec.FuncReturn{Arguments: `second"}`}}})
		cfg.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
		resp.Close()
		return resp, nil
	})
	loop.SetCurrentTask(newMockSimpleTask("native-progressive", "1"))
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer", StreamFields: loopAction_DirectlyAnswer.StreamFields})
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	loop.emitter = aicommon.NewEmitter("native-progressive", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		if event.NodeId == "re-act-loop-answer-payload" && event.Type == schema.EVENT_TYPE_STREAM {
			select {
			case firstDelta <- string(event.StreamDelta):
			default:
			}
		}
		return event, nil
	})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	_, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	var answer string
	var deltas, finishes int
	for _, event := range events {
		if event.NodeId == "re-act-loop-answer-payload" && event.Type == schema.EVENT_TYPE_STREAM {
			answer += string(event.StreamDelta)
			deltas++
		}
		if event.NodeId == "stream-finished" && strings.Contains(string(event.Content), `"node_id":"re-act-loop-answer-payload"`) {
			finishes++
		}
	}
	require.Equal(t, "first second", answer)
	require.GreaterOrEqual(t, deltas, 2)
	require.Equal(t, 1, finishes)
}

func TestCallAILoopTransactionNativeArgumentsFragmentedEscapesMatchAcceptedAction(t *testing.T) {
	const arguments = `{"answer_payload":"\u4f60\u597d\n# \"quoted\" \\ path \ud83d\ude00"}`
	const expected = "你好\n# \"quoted\" \\ path 😀"
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		// Split even inside escapes and Unicode surrogate pairs, as providers
		// can choose arbitrary tool-call argument chunk boundaries.
		for i := 0; i < len(arguments); i++ {
			delta := &aispec.ToolCall{Index: 0, ID: "answer", Function: aispec.FuncReturn{Arguments: arguments[i : i+1]}}
			if i == 0 {
				delta.Type = "function"
				delta.Function.Name = "directly_answer"
			}
			cfg.ToolCallCallback([]*aispec.ToolCall{delta})
		}
		cfg.FinishReasonCallback("tool_calls", nil)
		resp.Close()
		return resp, nil
	})
	var verified string
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer",
		StreamFields: loopAction_DirectlyAnswer.StreamFields,
		ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
			verified = action.GetString("answer_payload")
			return nil
		},
	})
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	loop.emitter = aicommon.NewEmitter("native-fragmented", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		return event, nil
	})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.Equal(t, expected, verified)
	require.Equal(t, expected, calls[0].Action.GetString("answer_payload"))
	mu.Lock()
	defer mu.Unlock()
	var displayed string
	var starts, finishes int
	for _, event := range events {
		if event.NodeId == "re-act-loop-answer-payload" {
			require.Equal(t, aicommon.TypeTextMarkdown, event.ContentType)
			require.Equal(t, "answer_payload", event.VizSource)
			if event.Type == schema.EVENT_TYPE_STREAM_START {
				starts++
			}
			if event.Type == schema.EVENT_TYPE_STREAM {
				displayed += string(event.StreamDelta)
			}
		}
		if event.NodeId == "stream-finished" && strings.Contains(string(event.Content), `"node_id":"re-act-loop-answer-payload"`) {
			finishes++
		}
	}
	require.Equal(t, expected, displayed)
	require.Equal(t, 1, starts)
	require.Equal(t, 1, finishes)
}

func TestCallAILoopTransactionNativeArgumentsRejectDuplicateVisibleField(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "answer", Type: "function",
			Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"first","answer_payload":"second"}`}}})
		cfg.FinishReasonCallback("tool_calls", nil)
		resp.Close()
		return resp, nil
	})
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer", StreamFields: loopAction_DirectlyAnswer.StreamFields})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.ErrorContains(t, err, `duplicate top-level native argument "answer_payload"`)
	require.Empty(t, calls, "the action must not execute with a different value from its displayed field")
}

func TestCallAILoopTransactionNativeArgumentsDoNotDisplayUnadvertisedAction(t *testing.T) {
	loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(nil)
		cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "answer", Type: "function",
			Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"unadvertised"}`}}})
		cfg.FinishReasonCallback("tool_calls", nil)
		resp.Close()
		return resp, nil
	})
	loop.lastNativeActionNames = []string{"finish"}
	loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer", StreamFields: loopAction_DirectlyAnswer.StreamFields})
	var mu sync.Mutex
	var displayed bool
	loop.emitter = aicommon.NewEmitter("native-unadvertised", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.NodeId == "re-act-loop-answer-payload" {
			mu.Lock()
			displayed = true
			mu.Unlock()
		}
		return event, nil
	})
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
		drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
	require.ErrorContains(t, err, "native function was not advertised")
	require.Empty(t, calls)
	mu.Lock()
	defer mu.Unlock()
	require.False(t, displayed)
}

func TestCallAILoopTransactionNativeTodoAdjustmentGuardsAnswer(t *testing.T) {
	drain := func(a, b io.Reader) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
		go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
		wg.Wait()
	}
	for _, tc := range []struct {
		name      string
		calls     []*aispec.ToolCall
		wantNames []string
	}{
		{
			name: "adjust before answer",
			calls: []*aispec.ToolCall{
				{Index: 0, ID: "todo", Type: "function", Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName, Arguments: `{"todo_delta":{"add":[{"id":"followup","text":"Inspect the next file"}],"current":"followup"}}`}},
				{Index: 1, ID: "answer", Type: "function", Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"progress"}`}},
			},
			wantNames: []string{nativeAdjustTodolistActionName, "directly_answer"},
		},
		{
			name: "adjust after answer",
			calls: []*aispec.ToolCall{
				{Index: 0, ID: "answer", Type: "function", Function: aispec.FuncReturn{Name: "directly_answer", Arguments: `{"answer_payload":"progress"}`}},
				{Index: 1, ID: "todo", Type: "function", Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName, Arguments: `{"todo_delta":{"add":[{"id":"followup","text":"Inspect the next file"}]}}`}},
			},
			wantNames: []string{"directly_answer", nativeAdjustTodolistActionName},
		},
		{
			name: "standalone empty adjustment is a no-op",
			calls: []*aispec.ToolCall{
				{Index: 0, ID: "todo", Type: "function", Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName, Arguments: `{"todo_delta":{}}`}},
			},
			wantNames: []string{nativeAdjustTodolistActionName},
		},
		{
			name: "later adjustment may target earlier addition",
			calls: []*aispec.ToolCall{
				{Index: 0, ID: "todo_add", Type: "function", Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName, Arguments: `{"todo_delta":{"add":[{"id":"followup","text":"Inspect the next file"}]}}`}},
				{Index: 1, ID: "todo_focus", Type: "function", Function: aispec.FuncReturn{Name: nativeAdjustTodolistActionName, Arguments: `{"todo_delta":{"current":"followup"}}`}},
			},
			wantNames: []string{nativeAdjustTodolistActionName, nativeAdjustTodolistActionName},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				resp := aicommon.NewAIResponse(nil)
				cfg.ToolCallCallback(tc.calls)
				cfg.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
				resp.Close()
				return resp, nil
			})
			loop.SetCurrentTask(newMockSimpleTask("native-todo", "1"))
			loop.Set(loopVarDirectlyAnswerDeliveredWithoutTodoDelta, true)
			loop.actions.Set(nativeAdjustTodolistActionName, loopAction_AdjustTodolistNative)
			loop.actions.Set("directly_answer", &LoopAction{ActionType: "directly_answer", ActionVerifier: RejectDuplicateDirectlyAnswerWithoutTodoDelta})
			calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
				drain, func(_, _ string, description, arguments io.Reader) { drain(description, arguments) })
			require.NoError(t, err)
			require.Len(t, calls, len(tc.wantNames))
			for index, wantName := range tc.wantNames {
				require.Equal(t, wantName, calls[index].Action.Name())
			}
			if tc.name == "later adjustment may target earlier addition" {
				delta, parseErr := aicommon.NormalizeTodoDelta(calls[1].Action)
				require.NoError(t, parseErr)
				require.NotNil(t, delta)
			}
			require.Nil(t, loop.GetVariable(loopVarNativeTodoBatchAdjusted), "provisional batch state must not leak into another response")
		})
	}
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
