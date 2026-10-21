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
	unfoldedPadding string
	compressionErr  error
}

func (i *compressionPromptInvoker) AssembleLoopPrompt(_ []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	i.inputs = append(i.inputs, *input)
	if input.PromptMode == aicommon.LoopPromptModeCompressContext && i.compressionErr != nil {
		return nil, i.compressionErr
	}
	view := aicommon.RenderTimelineFrozenOpen(i.timeline)
	prompt := view.PromptText() + input.TaskInstruction + input.FrozenUserContext + input.TodoSnapshot + input.ReactiveData
	if frozen := i.timeline.FreezeSnapshot(); frozen == nil || frozen.Version == 0 {
		prompt += i.unfoldedPadding
	}
	return &aicommon.LoopPromptAssemblyResult{Prompt: prompt + input.CompressionGuide}, nil
}

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
			invoker := &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), timeline: cfg.Timeline, unfoldedPadding: strings.Repeat("unfolded ", 9000)}
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

func TestContextCompressionReleasesFailedPromptBuild(t *testing.T) {
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

	input := loop.captureLoopPromptBuildInput("build-error", "", strings.Repeat("TASK_CONTEXT ", 9000), nil, "", &LoopActionHandlerOperator{})
	_, err := loop.generateLoopPromptFromInput(input)
	require.ErrorIs(t, err, buildErr)
}

func TestContextCompressionActionReleasesCancelledDecision(t *testing.T) {
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
		response.EmitReasonStream(strings.NewReader("COMPRESSION_MODEL_REASONING"))
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
		for _, invalid := range []string{"wrong_action", "missing_action", "todo_delta"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, invalid), func(t *testing.T) {
				ctx := context.Background()
				qualityCalls, otherCalls := 0, 0
				cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
					aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						otherCalls++
						return nil, fmt.Errorf("compression must use the quality model")
					}),
					aicommon.WithQualityPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						qualityCalls++
						require.Equal(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel())
						params := map[string]any{"summary": "COMPACTED_HISTORY", "retain_timeline": "3-8,12,20-26,37,41-45", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}}
						name := aicommon.ContextCompressionAction
						if qualityCalls == 1 {
							switch invalid {
							case "wrong_action":
								name = "unsafe_action"
							case "missing_action":
								name = ""
							case "todo_delta":
								params["todo_delta"] = map[string]any{"add": []any{map[string]any{"id": "bad", "text": "must not apply"}}}
							}
						} else {
							require.Contains(t, req.GetPrompt(), "# 重试纠正")
						}
						return contextCompressionTestResponse(t, c, req, name, params, false), nil
					}))
				for _, id := range []int64{3, 8, 10, 12, 20, 26, 37, 41, 45} {
					cfg.Timeline.PushText(id, "history-%d", id)
				}
				loop := makeSchemaStabilityTestLoop(cfg)
				loop.functionCallMode, loop.useSpeedPriorityAI = native, false
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
				require.Contains(t, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).PromptText(), "history-10", "parsing a proposal must not commit it")
				op := NewActionHandlerOperator(nil)
				calls[0].LoopAction.ActionHandler(loop, calls[0].Action, op)
				_, err = op.IsTerminated()
				require.NoError(t, err)
				require.Equal(t, 2, qualityCalls, "the action must not call AI")
				view := aicommon.RenderTimelineFrozenOpen(cfg.Timeline)
				require.Contains(t, view.Frozen, "COMPACTED_HISTORY")
				require.NotContains(t, view.Frozen, "history-10")
				require.Contains(t, view.Frozen, "history-12")
			})
		}
	}
}

func TestContextCompressionVerifierIsModeAgnostic(t *testing.T) {
	loop := makeSchemaStabilityTestLoop(aicommon.NewConfig(context.Background()))
	action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{"summary": "x", "retain_timeline": ""})
	require.NoError(t, loopAction_CompressContext.ActionVerifier(loop, action))
	op := NewActionHandlerOperator(nil)
	handleCompressContext(loop, action, op)
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	require.False(t, terminated)
}

func TestContextCompressionHandlerParsesSelection(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
	cfg.Timeline.PushText(12, "history")
	loop := makeSchemaStabilityTestLoop(cfg)
	action := aicommon.NewSimpleAction(aicommon.ContextCompressionAction, aitool.InvokeParams{
		"summary": "summary", "retain_timeline": "99", "retire_skill_names": []any{}, "retire_evidence_ids": []any{},
	})
	require.NoError(t, loop.verifyContextCompressionAction(action), "verification only checks action compliance")
	op := NewActionHandlerOperator(nil)
	handleCompressContext(loop, action, op)
	_, err := op.IsTerminated()
	require.ErrorContains(t, err, "retain_timeline")
}

func TestContextCompressionExamplesSupportScatteredRanges(t *testing.T) {
	examples := regexp.MustCompile(`(?m)^\{.*\}$`).FindAllString(compressContextDynamicText, -1)
	require.Len(t, examples, 5)
	var selections []string
	for _, example := range examples {
		var params aitool.InvokeParams
		require.NoError(t, json.Unmarshal([]byte(example), &params))
		require.Equal(t, aicommon.ContextCompressionAction, params["@action"])
		selections = append(selections, params["retain_timeline"].(string))
	}
	require.Contains(t, selections, "3-8,20-26,41-45")
	require.Contains(t, selections, "3-8,12,20-26,37,41-45")
	require.Contains(t, selections, "")
}
