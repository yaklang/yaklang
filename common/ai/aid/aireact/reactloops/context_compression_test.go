package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type compressionPromptInvoker struct {
	*mock.MockInvoker
	timeline        *aicommon.Timeline
	inputs          []aicommon.LoopPromptAssemblyInput
	overflowPadding string
	compressionErr  error
}

func (i *compressionPromptInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	i.inputs = append(i.inputs, *input)
	if input.PromptMode == aicommon.LoopPromptModeCompressContext && i.compressionErr != nil {
		return nil, i.compressionErr
	}
	view := aicommon.RenderTimelineFrozenOpen(i.timeline)
	prompt := view.PromptText() + input.TaskInstruction + input.FrozenUserContext + input.TodoSnapshot + input.ReactiveData
	prompt += i.overflowPadding
	return &aicommon.LoopPromptAssemblyResult{Prompt: prompt + input.CompressionGuide}, nil
}

// compressionReasoningMarker identifies a compression response's reasoning so
// tests can prove it never leaks into a later normal request.
var compressionReasoningMarker = uuid.NewString()

// Loop-level mode selection with a stub assembler: it verifies the threshold
// decision and the guide the loop appends. Real prompt rendering is covered by
// the aireact main-loop tests.
func TestContextCompressionPromptSelectsModeAndOneSixthTarget(t *testing.T) {
	for _, words := range []int{100, 1500} {
		t.Run(fmt.Sprint(words), func(t *testing.T) {
			calls := 0
			cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000),
				aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls++
					return nil, fmt.Errorf("prompt construction must not call AI")
				}))
			loop := makeSchemaStabilityTestLoop(cfg)
			loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
			invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), timeline: cfg.Timeline, overflowPadding: strings.Repeat("unfolded ", 9000)}
			loop.invoker = invoker
			for _, text := range []string{strings.Repeat("obsolete ", 9000), strings.Repeat("current ", words)} {
				payload, err := json.Marshal(map[string]string{"id": "e1", "content": text})
				require.NoError(t, err)
				require.True(t, cfg.Timeline.PushPromotable(cfg.AcquireId(), aicommon.TimelinePromotedKindEvidence,
					aicommon.TimelinePromotedTargetSemiDynamic1, "e1", aicommon.TimelinePromotedOperationUpsert, string(payload)))
			}
			input := loop.captureLoopPromptBuildInput("mode", "CURRENT_TASK", "TASK_CONTEXT", nil, "", &LoopActionHandlerOperator{})
			result, err := loop.generateLoopPromptFromInput(input)
			require.NoError(t, err)
			require.Zero(t, calls)
			require.Nil(t, loop.GetLastPromptObservation())
			require.Contains(t, result.Prompt, "TASK_CONTEXT")
			require.Equal(t, aicommon.LoopPromptModeCompressContext, result.Mode)
			require.Equal(t, 2, len(invoker.inputs), "overflow goes directly from the normal assembly to compression mode")
			require.Contains(t, result.Prompt, "接近 1000 tokens")
			require.Contains(t, result.Prompt, "六分之一")
			require.Equal(t, invoker.inputs[0].Schema, invoker.inputs[1].Schema, "switching mode must not rewrite the schema")
			require.Less(t, aicommon.MeasureTokens(invoker.inputs[1].CompressionGuide), 6000)
		})
	}
}

// The stub assembler returns everything it receives, so this test proves the
// loop measures the assembled prompt rather than only timeline history.
func TestContextCompressionMeasuresWholePrompt(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000))
	cfg.Timeline.PushText(cfg.AcquireId(), "short history")
	require.Less(t, aicommon.MeasureTokens(aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromptText()), 1000)
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
	invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), timeline: cfg.Timeline}
	invoker.SetConfig(cfg)
	loop.invoker = invoker

	input := loop.captureLoopPromptBuildInput("whole-prompt", "", strings.Repeat("TASK_CONTEXT ", 9000), nil, "", &LoopActionHandlerOperator{})
	result, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Equal(t, aicommon.LoopPromptModeCompressContext, result.Mode, "the full prompt, not only timeline history, must trigger compression")
	require.Contains(t, result.Prompt, input.frozenUserContext)
	require.Contains(t, result.Prompt, "接近 1000 tokens")
}

func TestContextCompressionSkipsModeWhenTimelineHasNoCompressibleContent(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000))
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
	invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), timeline: cfg.Timeline}
	invoker.SetConfig(cfg)
	loop.invoker = invoker
	capture := captureActivityStatus(loop)

	input := loop.captureLoopPromptBuildInput("no-compressible-input", "", strings.Repeat("frozen context ", 9000), nil, "", &LoopActionHandlerOperator{})
	result, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Equal(t, aicommon.LoopPromptModeNormal, result.Mode)
	require.True(t, capture.waitStream(contextCompressionWarningNodeID, "Context compression is unavailable for this turn"))
}

func TestContextCompressionGuideErrorsFallbackToNormalMode(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000))
	cfg.Timeline.PushText(cfg.AcquireId(), "compressible history")
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
	invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), timeline: cfg.Timeline}
	invoker.SetConfig(cfg)
	loop.invoker = invoker
	invalidThroughID := int64(-1)
	loop.lastFittingTimelineThroughID = &invalidThroughID
	capture := captureActivityStatus(loop)

	input := loop.captureLoopPromptBuildInput("guide-error", "", "", nil, "", &LoopActionHandlerOperator{})
	input.overflowCompression = true
	result, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Equal(t, aicommon.LoopPromptModeNormal, result.Mode)
	require.True(t, capture.waitStream(contextCompressionWarningNodeID, "Context compression is unavailable for this turn"))
}

func TestContextCompressionPromptTokenLimitUsesConfiguredLimitPlusHeadroom(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(40*1024))
	loop := makeSchemaStabilityTestLoop(cfg)
	require.EqualValues(t, 240*1024, loop.contextCompressionPromptTokenLimit())
}

func TestContextCompressionPromptBuildErrorFallsBackToNormalMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithTimelineContentLimit(6000))
	cfg.Timeline.PushText(cfg.AcquireId(), "short history")
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
	buildErr := fmt.Errorf("cannot assemble compression prompt")
	invoker := &compressionPromptInvoker{
		MockInvoker: mock.NewMockInvoker(ctx), timeline: cfg.Timeline, compressionErr: buildErr,
	}
	invoker.SetConfig(cfg)
	loop.invoker = invoker
	capture := captureActivityStatus(loop)

	input := loop.captureLoopPromptBuildInput("build-error", "", strings.Repeat("TASK_CONTEXT ", 9000), nil, "", &LoopActionHandlerOperator{})
	result, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	require.Equal(t, aicommon.LoopPromptModeNormal, result.Mode)
	require.True(t, capture.waitStream(contextCompressionWarningNodeID, "assembling the compression prompt"))
}

func TestContextCompressionActionFailsOnCancelledTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
	cfg.Timeline.PushText(cfg.AcquireId(), "history")
	loop := makeSchemaStabilityTestLoop(cfg)
	action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{
		"summary": "compacted history", "retain_timeline": "", "retire_skill_names": []any{}, "retire_evidence_ids": []any{},
	})
	require.NoError(t, loop.verifyContextCompressionAction(action))
	cancel()

	op := NewActionHandlerOperator(nil)
	handleCompressContext(loop, action, op)
	_, err := op.IsTerminated()
	require.ErrorContains(t, err, context.Canceled.Error())
}

func contextCompressionTestResponse(t *testing.T, c aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, params map[string]any, multiple bool) *aicommon.AIResponse {
	t.Helper()
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	response := c.NewAIResponse()
	if name == aicommon.ContextCompressionAction {
		response.EmitReasonStream(strings.NewReader(compressionReasoningMarker))
	}
	if wire.ToolCallCallback != nil {
		arguments, err := json.Marshal(params)
		require.NoError(t, err)
		calls := []*aispec.ToolCall{{ID: "compression-1", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(arguments)}}}
		if multiple {
			calls = append(calls, &aispec.ToolCall{ID: "compression-2", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(arguments)}})
		}
		wire.ToolCallCallback(calls)
		wire.FinishReasonCallback("tool_calls", nil)
	} else {
		params["@action"] = name
		data, err := json.Marshal(params)
		require.NoError(t, err)
		response.EmitOutputStream(strings.NewReader(string(data)))
		wire.FinishReasonCallback("stop", nil)
	}
	response.Close()
	return response
}

func TestContextCompressionTransactionEnforcesAction(t *testing.T) {
	for _, native := range []bool{false, true} {
		invalidCases := []string{"wrong_action", "missing_action"}
		if native {
			invalidCases = append(invalidCases, "multiple_calls")
		}
		for _, invalid := range invalidCases {
			t.Run(fmt.Sprintf("native=%v/%s", native, invalid), func(t *testing.T) {
				ctx := context.Background()
				qualityCalls, otherCalls := 0, 0
				summary := uuid.NewString()
				historyText := map[int64]string{}
				cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
					aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						otherCalls++
						return nil, fmt.Errorf("compression must use the quality model")
					}),
					aicommon.WithQualityPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						qualityCalls++
						require.Equal(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel())
						params := map[string]any{"summary": summary, "retain_timeline": "3-8,12,20-26,37,41-45", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}}
						name := aicommon.ContextCompressionAction
						multiple := false
						if qualityCalls == 1 {
							switch invalid {
							case "wrong_action":
								name = "unsafe_action"
							case "missing_action":
								name = ""
							case "multiple_calls":
								multiple = true
							}
						} else {
							require.Contains(t, req.GetPrompt(), "# 重试纠正")
						}
						return contextCompressionTestResponse(t, c, req, name, params, multiple), nil
					}))
				for _, id := range []int64{3, 8, 10, 12, 20, 26, 37, 41, 45} {
					historyText[id] = uuid.NewString()
					cfg.Timeline.PushText(id, "%s", historyText[id])
				}
				loop := makeSchemaStabilityTestLoop(cfg)
				// Compression stays on Quality even when the surrounding loop uses Speed.
				loop.functionCallMode, loop.useSpeedPriorityAI = native, true
				loop.invoker = mock.NewMockInvoker(ctx)
				loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
				loop.actions.Set("unsafe_action", &LoopAction{ActionType: "unsafe_action", ActionHandler: func(*ReActLoop, *aicommon.Action, *LoopActionHandlerOperator) { t.Error("wrong action executed") }})
				_, _, err := loop.prepareLoopActionSchemas(&LoopActionHandlerOperator{})
				require.NoError(t, err)
				loop.promptMode = aicommon.LoopPromptModeCompressContext
				var streams sync.WaitGroup
				calls, _, _, err := loop.callAILoopTransaction(&streams, "STATIC_CONTEXT", "compression", &LoopActionHandlerOperator{}, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
				streams.Wait()
				require.NoError(t, err)
				require.Len(t, calls, 1)
				require.Equal(t, aicommon.ContextCompressionAction, calls[0].Action.Name())
				require.Equal(t, 2, qualityCalls)
				require.Zero(t, otherCalls)
				require.Contains(t, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromptText(), historyText[10], "parsing a proposal must not commit it")
				op := NewActionHandlerOperator(nil)
				calls[0].LoopAction.ActionHandler(loop, calls[0].Action, op)
				_, err = op.IsTerminated()
				require.NoError(t, err)
				require.Equal(t, 2, qualityCalls, "the action must not call AI")
				view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
				require.Contains(t, view.Frozen, summary)
				require.NotContains(t, view.Frozen, historyText[10])
				require.Contains(t, view.Frozen, historyText[12])
			})
		}
	}
}

func TestContextCompressionRuntimeReadAndApplyFailuresAreRecoverableActionErrors(t *testing.T) {
	for _, test := range []struct {
		name          string
		throughID     *int64
		itemIDs       []int64
		summary       string
		expectedError string
	}{
		{
			name: "candidate read failure", throughID: func() *int64 { id := int64(-1); return &id }(),
			summary: "summary", expectedError: "invalid compression history boundary",
		},
		{
			name: "apply sees unobserved history", itemIDs: []int64{},
			summary: "", expectedError: "invalid compression selection",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true))
			history := uuid.NewString()
			cfg.Timeline.PushText(12, "%s", history)
			loop := makeSchemaStabilityTestLoop(cfg)
			action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{
				"summary": test.summary, "retain_timeline": "", "retire_skill_names": []any{}, "retire_evidence_ids": []any{},
			})
			op := NewActionHandlerOperator(nil)
			op.requestTimelineThroughID = test.throughID
			op.requestTimelineItemIDs = test.itemIDs

			handleCompressContext(loop, action, op)
			terminated, err := op.IsTerminated()
			require.True(t, terminated)
			require.ErrorContains(t, err, test.expectedError)
			require.Contains(t, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromptText(), history,
				"failed compression must leave the timeline unchanged")
		})
	}
}

func TestContextCompressionCommitsWhenSummaryIsUsable(t *testing.T) {
	for _, test := range []struct {
		name   string
		params aitool.InvokeParams
	}{
		{name: "summary only", params: aitool.InvokeParams{"summary": "summary"}},
		{name: "non ordinary retain id", params: aitool.InvokeParams{"summary": "summary", "retain_timeline": "4498"}},
		{name: "malformed retain and retire values", params: aitool.InvokeParams{
			"summary": "summary", "retain_timeline": "12-abc,4498",
			"retire_skill_names": "not-an-array", "retire_evidence_ids": map[string]any{"invalid": true},
			"memory_entities": "invalid",
		}},
		{name: "structured summary and todo delta", params: aitool.InvokeParams{
			"summary": []any{"summary part one", "summary part two"},
			"todo_delta": map[string]any{"add": []any{
				map[string]any{"id": "bad", "text": "must not apply"},
			}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
			cfg.Timeline.PushText(12, "history")
			loop := makeSchemaStabilityTestLoop(cfg)
			action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, test.params)
			require.NoError(t, loop.verifyContextCompressionAction(action), "only a missing summary is structural")
			_, hasTodoDelta := action.LookupCanonicalParam("todo_delta")
			require.False(t, hasTodoDelta, "compression must ignore todo_delta")

			op := NewActionHandlerOperator(nil)
			handleCompressContext(loop, action, op)
			terminated, err := op.IsTerminated()
			require.NoError(t, err)
			require.False(t, terminated)
			view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
			require.Contains(t, view.Frozen, "summary")
			require.NotContains(t, view.Frozen, "history")
		})
	}
}

func TestContextCompressionWithoutSummaryStillRequestsCorrection(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
	cfg.Timeline.PushText(12, "history")
	loop := makeSchemaStabilityTestLoop(cfg)
	action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{"retain_timeline": "12"})
	require.ErrorContains(t, loop.verifyContextCompressionAction(action), "requires summary")

	op := NewActionHandlerOperator(nil)
	handleCompressContext(loop, action, op)
	terminated, err := op.IsTerminated()
	require.True(t, terminated)
	require.ErrorContains(t, err, "summary 不能为空")
}

func TestParseContextCompressionRetainedIDsIntersectsOrdinaryCandidates(t *testing.T) {
	candidates := []aicommon.TimelineCompressionCandidate{
		{ID: 12, PromptText: "twelve"},
		{ID: 20, PromptText: "twenty"},
		{ID: 30, PromptText: "thirty"},
		{ID: 4498, PromptText: "protected", Kind: aicommon.TimelinePromotedKindUserInput},
	}
	for _, test := range []struct {
		name string
		text string
		want []int64
	}{
		{name: "protected single id", text: "4498", want: nil},
		{name: "range endpoints need not be ordinary", text: "10-21,4498", want: []int64{12, 20}},
		{name: "ordinary single ids", text: "12,30", want: []int64{12, 30}},
		{name: "malformed segments are ignored", text: "12-abc,broken,30", want: []int64{30}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := parseContextCompressionRetainedIDs(test.text, candidates)
			require.Equal(t, test.want, got)
		})
	}
}

func TestContextCompressionSummaryOverAdvisoryLimitWarnsAndCommits(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
	retiredHistory := uuid.NewString()
	cfg.Timeline.PushText(12, "%s", retiredHistory)
	loop := makeSchemaStabilityTestLoop(cfg)
	summary := strings.TrimSpace(strings.Repeat("long summary ", aicommon.TimelineCompressionMaxSummaryTokens+1))
	action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{
		"summary": summary, "retain_timeline": "", "retire_skill_names": []any{}, "retire_evidence_ids": []any{},
	})
	require.Greater(t, aicommon.MeasureTokens(summary), aicommon.TimelineCompressionMaxSummaryTokens)

	capture := captureActivityStatus(loop)
	op := NewActionHandlerOperator(nil)
	handleCompressContext(loop, action, op)
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	require.False(t, terminated)
	require.True(t, capture.waitStream(contextCompressionWarningNodeID, "Context compression exceeded the advisory limit"))
	loop.GetEmitter().WaitForStream()
	capture.mu.Lock()
	allNormalStreams := true
	for _, event := range capture.events {
		if event.NodeId == contextCompressionWarningNodeID && event.IsStream && event.IsSystem {
			allNormalStreams = false
		}
	}
	capture.mu.Unlock()
	require.True(t, allNormalStreams, "context compression warning must use a normal user-visible stream")
	serialized, err := aicommon.MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	require.Contains(t, serialized, summary)
	view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
	require.NotContains(t, view.Frozen, retiredHistory)
}

func TestContextCompressionWarningUsesDedicatedNodeAndEnglishBody(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithLanguage("zh"),
	)
	loop := makeSchemaStabilityTestLoop(cfg)
	capture := captureActivityStatus(loop)
	loop.emitContextCompressionAdvisoryLimit(70_000, 65_536)
	require.Equal(t, "context_compression_warning", contextCompressionWarningNodeID)
	require.True(t, capture.waitStream(contextCompressionWarningNodeID, "Context compression exceeded the advisory limit"))
	loop.GetEmitter().WaitForStream()
}

func TestContextCompressionExamplesSupportScatteredRanges(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			prompt, err := aicommon.RenderPromptTemplate("timeline-compression", compressContextDynamicText,
				map[string]any{"FunctionCallMode": native})
			require.NoError(t, err)
			if native {
				require.NotContains(t, prompt, "@action", "native compression must not teach the text action protocol")
				require.Contains(t, prompt, "content")
			}
			examples := regexp.MustCompile(`(?m)^\{.*\}$`).FindAllString(prompt, -1)
			require.Len(t, examples, 5)
			var selections []string
			for _, example := range examples {
				var params aitool.InvokeParams
				require.NoError(t, json.Unmarshal([]byte(example), &params))
				if native {
					require.NotContains(t, params, "@action")
				} else {
					require.Equal(t, aicommon.ContextCompressionAction, params["@action"])
				}
				selections = append(selections, params["retain_timeline"].(string))
			}
			require.Contains(t, selections, "3-8,20-26,41-45")
			require.Contains(t, selections, "3-8,12,20-26,37,41-45")
			require.Contains(t, selections, "")
		})
	}
}

// A provider may still write an internal compression action to content. It
// remains diagnostic data even when validation rejects the missing native call.
func TestContextCompressionNativeContentStaysInternal(t *testing.T) {
	const content = `{"@action":"compress_context","summary":"internal memory"}`
	for _, toolCall := range []bool{false, true} {
		t.Run(fmt.Sprintf("toolCall=%v", toolCall), func(t *testing.T) {
			loop := newCallAILoopTransactionTestLoop(t, true, func(_ *aicommon.AIRequest, wire *aispec.AIConfig) (*aicommon.AIResponse, error) {
				resp := aicommon.NewAIResponse(nil)
				resp.EmitOutputStream(strings.NewReader(content))
				resp.EmitReasonStream(strings.NewReader(compressionReasoningMarker))
				if toolCall {
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: "compression", Type: "function", Function: aispec.FuncReturn{
						Name:      aicommon.ContextCompressionAction,
						Arguments: `{"summary":"internal memory","retain_timeline":"","retire_skill_names":[],"retire_evidence_ids":[]}`,
					}}})
					wire.FinishReasonCallback("tool_calls", nil)
				} else {
					wire.FinishReasonCallback("stop", nil)
				}
				resp.Close()
				return resp, nil
			})
			loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
			loop.promptMode = aicommon.LoopPromptModeCompressContext
			capture := captureActivityStatus(loop)
			calls, _, descriptor, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
				loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
			if toolCall {
				require.NoError(t, err)
				require.Len(t, calls, 1)
			} else {
				require.Error(t, err)
				require.Empty(t, calls, "content must never execute as a native call")
			}
			var response map[string]any
			require.NoError(t, json.Unmarshal([]byte(descriptor.Snapshot().ResponseJSON), &response))
			require.Equal(t, content, response["content"], "retain the provider response for diagnostics")
			capture.mu.Lock()
			defer capture.mu.Unlock()
			for _, event := range capture.events {
				if event.IsStream {
					require.Empty(t, event.StreamDelta, "internal compression output must not become chat content")
				}
			}
		})
	}
}
