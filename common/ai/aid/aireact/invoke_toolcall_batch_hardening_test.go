package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type batchHardeningReviewMaterial struct {
	ID         string              `json:"id"`
	Tool       string              `json:"tool"`
	Params     aitool.InvokeParams `json:"params"`
	BatchID    string              `json:"batch_id"`
	CallIndex  int                 `json:"call_index"`
	CallToolID string              `json:"call_tool_id"`
}

type batchHardeningReviewRecorder struct {
	count       int32
	materialsMu sync.Mutex
	materials   []batchHardeningReviewMaterial
}

func (r *batchHardeningReviewRecorder) append(material batchHardeningReviewMaterial) int {
	r.materialsMu.Lock()
	defer r.materialsMu.Unlock()
	r.materials = append(r.materials, material)
	return len(r.materials) - 1
}

func (r *batchHardeningReviewRecorder) snapshot() []batchHardeningReviewMaterial {
	r.materialsMu.Lock()
	defer r.materialsMu.Unlock()
	return append([]batchHardeningReviewMaterial(nil), r.materials...)
}

func newBatchHardeningReplayRuntime(
	t *testing.T,
	runtimeID string,
	sequenceStart int64,
	callback aicommon.AICallbackType,
	recorder *batchHardeningReviewRecorder,
	decision func(index int, material batchHardeningReviewMaterial) string,
	tools ...*aitool.Tool,
) *ReAct {
	t.Helper()
	input := make(chan *ypb.AIInputEvent, 16)
	if callback == nil {
		callback = func(_ aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}
	}
	react, err := NewTestReAct(
		aicommon.WithID(runtimeID),
		aicommon.WithSequence(sequenceStart),
		aicommon.WithAICallback(callback),
		aicommon.WithTools(tools...),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventInputChan(input),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			atomic.AddInt32(&recorder.count, 1)
			var material batchHardeningReviewMaterial
			_ = json.Unmarshal(event.Content, &material)
			index := recorder.append(material)
			response := `{"suggestion":"continue"}`
			if decision != nil {
				response = decision(index, material)
			}
			input <- &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        material.ID,
				InteractiveJSONInput: response,
			}
		}),
	)
	require.NoError(t, err)
	return react
}

func batchHardeningAIResponse(config aicommon.AICallerConfigIf, raw string) (*aicommon.AIResponse, error) {
	response := config.NewAIResponse()
	response.EmitOutputStream(bytes.NewBufferString(raw))
	response.Close()
	return response, nil
}

func batchHardeningResultParams(t *testing.T, result *aitool.ToolResult) aitool.InvokeParams {
	t.Helper()
	require.NotNil(t, result)
	switch params := result.Param.(type) {
	case aitool.InvokeParams:
		return params
	case map[string]any:
		return aitool.InvokeParams(params)
	default:
		t.Fatalf("unexpected result param type %T", result.Param)
		return nil
	}
}

func batchHardeningRequest(targetTool, siblingTool string, params aitool.InvokeParams) *aicommon.ToolCallGroupRequest {
	return &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{
			ToolName: targetTool,
			Params:   params,
			Reason:   "exercise the review checkpoint",
		},
		{
			ToolName: siblingTool,
			Params:   aitool.InvokeParams{"id": 2},
			Reason:   "exercise the batch barrier",
		},
	}}
}

func TestExecuteToolCallGroup_DirectGuardRejectionSkipsOnlyRejectedChild(t *testing.T) {
	var rejectedParamCalls int32
	var allowedParamCalls int32
	var rejectedInvokes int32
	var allowedInvokes int32

	rejectedTool, err := aitool.New(
		"batch_guard_rejected_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&rejectedInvokes, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	allowedTool, err := aitool.New(
		"batch_guard_allowed_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&allowedInvokes, 1)
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			switch {
			case isToolParamGenerationPrompt(prompt, rejectedTool.Name):
				atomic.AddInt32(&rejectedParamCalls, 1)
				return nil, fmt.Errorf("unexpected retired parameter generation for rejected tool")
			case isToolParamGenerationPrompt(prompt, allowedTool.Name):
				atomic.AddInt32(&allowedParamCalls, 1)
				return nil, fmt.Errorf("unexpected retired parameter generation for allowed tool")
			default:
				return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
			}
		}),
		aicommon.WithTools(rejectedTool, allowedTool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	loop, err := reactloops.NewReActLoop(
		"batch-require-guard-test",
		react,
		reactloops.WithToolInvokeGuard(func(toolName string, _ aitool.InvokeParams) (bool, string) {
			if toolName == rejectedTool.Name {
				return false, "blocked by test guard"
			}
			return true, ""
		}),
	)
	require.NoError(t, err)
	loop.SetCurrentTask(react.config.DefaultTask)

	result, execErr := react.ExecuteToolCallGroup(
		context.Background(),
		react.config.DefaultTask,
		&aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
			{Params: aitool.InvokeParams{"id": 1}, ToolName: rejectedTool.Name, Reason: "must be rejected"},
			{Params: aitool.InvokeParams{"id": 2}, ToolName: allowedTool.Name, Reason: "must continue"},
		}},
	)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 2)
	require.Equal(t, aicommon.ToolCallStageValidationFailed, result.Outcomes[0].Stage)
	require.ErrorContains(t, result.Outcomes[0].Err, "blocked by test guard")
	require.Nil(t, result.Outcomes[0].Result)
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
	require.NotNil(t, result.Outcomes[1].Result)
	require.Equal(t, int32(0), atomic.LoadInt32(&rejectedParamCalls), "guarded require child must not generate params")
	require.Equal(t, int32(0), atomic.LoadInt32(&rejectedInvokes), "guarded require child must not invoke")
	require.Equal(t, int32(0), atomic.LoadInt32(&allowedParamCalls), "explicit sibling must not generate arguments")
	require.Equal(t, int32(1), atomic.LoadInt32(&allowedInvokes), "valid sibling must still invoke once")
}

func TestExecuteToolCallGroup_ManualReviewMaterialsCarryStableChildIdentity(t *testing.T) {
	tool, err := aitool.New(
		"batch_review_identity_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	recorder := new(batchHardeningReviewRecorder)
	react := newBatchHardeningReplayRuntime(
		t,
		"batch-review-identity-runtime-"+ksuid.New().String(),
		7000,
		nil,
		recorder,
		func(_ int, _ batchHardeningReviewMaterial) string {
			return `{"suggestion":"continue"}`
		},
		tool,
	)

	request := batchHardeningRequest(tool.Name, tool.Name, aitool.InvokeParams{"id": 1})
	result, execErr := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 2)
	materials := recorder.snapshot()
	require.Len(t, materials, 2)

	for index, material := range materials {
		require.NotEmpty(t, material.ID)
		require.Equal(t, request.Calls[index].ToolName, material.Tool)
		require.Equal(t, int64(index+1), material.Params.GetInt("id"))
		require.Equal(t, result.BatchID, material.BatchID)
		require.Equal(t, index, material.CallIndex)
		require.Equal(t, request.Calls[index].ExecutionCallID, material.CallToolID)
		require.NotEmpty(t, material.CallToolID)
		require.Equal(t, material.CallToolID, result.Outcomes[index].CallID)
		require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[index].Stage)
	}
}

func TestExecuteToolCallGroup_ManualReviewCheckpointReplay_DirectAnswer(t *testing.T) {
	var targetInvoked int32
	var siblingInvoked int32
	target, err := aitool.New(
		"batch_hardening_direct_answer_target",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&targetInvoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_hardening_direct_answer_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)

	runtimeID := "batch-hardening-direct-answer-" + ksuid.New().String()
	const sequenceStart int64 = 12100
	firstReviews := new(batchHardeningReviewRecorder)
	first := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, nil, firstReviews,
		func(_ int, _ batchHardeningReviewMaterial) string {
			return `{"suggestion":"direct_answer"}`
		},
		target, sibling,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstResult, firstErr := first.ExecuteToolCallGroup(
		ctx,
		first.config.DefaultTask,
		batchHardeningRequest(target.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, firstErr)
	require.True(t, firstResult.DirectlyAnswer)
	require.Equal(t, int32(1), atomic.LoadInt32(&firstReviews.count))
	firstMaterials := firstReviews.snapshot()
	require.Len(t, firstMaterials, 1)
	require.Equal(t, target.Name, firstMaterials[0].Tool)
	require.Equal(t, int64(1), firstMaterials[0].Params.GetInt("id"))
	require.Equal(t, int32(0), atomic.LoadInt32(&targetInvoked))
	require.Equal(t, int32(0), atomic.LoadInt32(&siblingInvoked))

	secondReviews := new(batchHardeningReviewRecorder)
	second := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, nil, secondReviews, nil, target, sibling,
	)
	secondResult, secondErr := second.ExecuteToolCallGroup(
		ctx,
		second.config.DefaultTask,
		batchHardeningRequest(target.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, secondErr)
	require.True(t, secondResult.DirectlyAnswer, "the persisted direct-answer decision must be applied")
	require.Equal(t, int32(0), atomic.LoadInt32(&secondReviews.count), "fresh runtime must not emit the review card again")
	require.Equal(t, int32(0), atomic.LoadInt32(&targetInvoked))
	require.Equal(t, int32(0), atomic.LoadInt32(&siblingInvoked))
	for _, outcome := range secondResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
		require.Nil(t, outcome.Result)
	}
}

// A rejected review settles only its own proposal. Recovery replays the same
// rejection and completed sibling; a corrected explicit batch is a new proposal.
func TestExecuteToolCallGroup_ReviewReconsiderAndReplay(t *testing.T) {
	for _, kind := range []string{"wrong_tool", "wrong_params"} {
		t.Run(kind, func(t *testing.T) {
			var originalCalls, replacementCalls, siblingCalls, aiCalls atomic.Int32
			makeTool := func(name string, calls *atomic.Int32, skipReview bool) *aitool.Tool {
				tool, err := aitool.New(name, aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
					aitool.WithDangerousNoNeedUserReview(skipReview),
					aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
						calls.Add(1)
						return params.GetInt("id"), nil
					}))
				require.NoError(t, err)
				return tool
			}
			original := makeTool("reconsider_original", &originalCalls, false)
			replacement := makeTool("reconsider_replacement", &replacementCalls, false)
			sibling := makeTool("reconsider_sibling", &siblingCalls, true)
			callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				aiCalls.Add(1)
				return nil, fmt.Errorf("review must not call auxiliary AI: %s", request.GetCallerLabel())
			}
			runtimeID := "review-reconsider-" + ksuid.New().String()
			reviews := new(batchHardeningReviewRecorder)
			first := newBatchHardeningReplayRuntime(t, runtimeID, 12200, callback, reviews,
				func(int, batchHardeningReviewMaterial) string {
					return fmt.Sprintf(`{"suggestion":%q,"suggestion_tool":%q,"extra_prompt":"use id 42"}`, kind, replacement.Name)
				}, original, replacement, sibling)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			request := func() *aicommon.ToolCallGroupRequest {
				return batchHardeningRequest(original.Name, sibling.Name, aitool.InvokeParams{"id": 1})
			}
			result, err := first.ExecuteToolCallGroup(ctx, first.config.DefaultTask, request())
			require.NoError(t, err)
			var rejected *aicommon.ToolReviewReconsiderError
			require.ErrorAs(t, result.Outcomes[0].Err, &rejected)
			require.Contains(t, rejected.Feedback, "use id 42")
			require.Nil(t, result.Outcomes[0].Result)
			require.False(t, result.DirectlyAnswer)
			require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
			require.Zero(t, originalCalls.Load())
			require.Zero(t, replacementCalls.Load())
			require.EqualValues(t, 1, siblingCalls.Load())
			require.EqualValues(t, 1, reviews.count)
			require.Zero(t, aiCalls.Load())
			require.True(t, first.config.GetAiToolManager().IsRecentlyUsedTool(original.Name))
			if kind == "wrong_tool" {
				require.True(t, first.config.GetAiToolManager().IsRecentlyUsedTool(replacement.Name))
			}
			replayReviews := new(batchHardeningReviewRecorder)
			replay := newBatchHardeningReplayRuntime(t, runtimeID, 12200, callback, replayReviews, nil, original, replacement, sibling)
			restored, err := replay.ExecuteToolCallGroup(ctx, replay.config.DefaultTask, request())
			require.NoError(t, err)
			require.ErrorAs(t, restored.Outcomes[0].Err, &rejected)
			require.Zero(t, replayReviews.count)
			require.Zero(t, originalCalls.Load())
			require.Zero(t, replacementCalls.Load())
			require.EqualValues(t, 1, siblingCalls.Load())
			require.Zero(t, aiCalls.Load())
			next := original
			if kind == "wrong_tool" {
				next = replacement
			}
			retry := batchHardeningRequest(next.Name, sibling.Name, aitool.InvokeParams{"id": 42})
			settled, err := replay.ExecuteToolCallGroup(ctx, replay.config.DefaultTask, retry)
			require.NoError(t, err)
			require.Equal(t, aicommon.ToolCallStageDone, settled.Outcomes[0].Stage)
			require.EqualValues(t, 42, batchHardeningResultParams(t, settled.Outcomes[0].Result).GetInt("id"))
			require.EqualValues(t, 1, originalCalls.Load()+replacementCalls.Load())
			require.Zero(t, aiCalls.Load())
		})
	}
}

func TestToolCaller_CancellationAfterAdmissionBoundariesSkipsCallbacks(t *testing.T) {

	t.Run("before invoke hook", func(t *testing.T) {
		var toolCalls int32
		var releases int32
		tool, err := aitool.New(
			"batch_hardening_cancel_after_before_invoke",
			aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
			aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				atomic.AddInt32(&toolCalls, 1)
				return "must not invoke", nil
			}),
		)
		require.NoError(t, err)
		react := newBatchTestReAct(t, tool, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		caller, err := aicommon.NewToolCaller(
			ctx,
			aicommon.WithToolCaller_AICallerConfig(react.config),

			aicommon.WithToolCaller_RuntimeId(react.config.GetRuntimeId()),
			aicommon.WithToolCaller_Emitter(react.config.GetEmitter()),
			aicommon.WithToolCaller_Task(react.config.DefaultTask),
			aicommon.WithToolCaller_Reason("cancel at the final invoke boundary"),
			aicommon.WithToolCaller_BeforeInvoke(func(context.Context, *aitool.Tool, aitool.InvokeParams) (func(), error) {
				cancel()
				return func() { atomic.AddInt32(&releases, 1) }, nil
			}),
		)
		require.NoError(t, err)
		_, _, callErr := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"id": 1})
		require.ErrorIs(t, callErr, context.Canceled)
		require.Equal(t, int32(0), atomic.LoadInt32(&toolCalls))
		require.Equal(t, int32(1), atomic.LoadInt32(&releases), "beforeInvoke cleanup must run when cancellation wins after admission")
	})
}
