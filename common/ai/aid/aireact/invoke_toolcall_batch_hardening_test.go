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

func batchHardeningRequest(targetTool, siblingTool string, params aitool.InvokeParams) *aicommon.ToolBatchRequest {
	return &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{
			Mode:     aicommon.ToolCallModeDirect,
			ToolName: targetTool,
			Params:   params,
			Reason:   "exercise the review checkpoint",
		},
		{
			Mode:     aicommon.ToolCallModeDirect,
			ToolName: siblingTool,
			Params:   aitool.InvokeParams{"id": 2},
			Reason:   "exercise the batch barrier",
		},
	}}
}

func TestExecuteToolBatch_RequireGuardRejectionSkipsOnlyRejectedChild(t *testing.T) {
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
				return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":1}}`)
			case isToolParamGenerationPrompt(prompt, allowedTool.Name):
				atomic.AddInt32(&allowedParamCalls, 1)
				return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":2}}`)
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

	result, execErr := react.ExecuteToolBatch(
		context.Background(),
		react.config.DefaultTask,
		&aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: rejectedTool.Name, Reason: "must be rejected"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: allowedTool.Name, Reason: "must continue"},
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
	require.Equal(t, int32(1), atomic.LoadInt32(&allowedParamCalls), "valid sibling must still prepare once")
	require.Equal(t, int32(1), atomic.LoadInt32(&allowedInvokes), "valid sibling must still invoke once")
}

func TestExecuteToolBatch_ManualReviewMaterialsCarryStableChildIdentity(t *testing.T) {
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
	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
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

func TestExecuteToolBatch_ManualReviewCheckpointReplay_DirectAnswer(t *testing.T) {
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
	firstResult, firstErr := first.ExecuteToolBatch(
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
	secondResult, secondErr := second.ExecuteToolBatch(
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

func TestExecuteToolBatch_ManualReviewCheckpointReplay_WrongParams(t *testing.T) {
	var targetInvoked int32
	var siblingInvoked int32
	var finalID int64
	var wrongParamsAICalls int32
	target, err := aitool.New(
		"batch_hardening_wrong_params_target",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&targetInvoked, 1)
			atomic.StoreInt64(&finalID, params.GetInt("id"))
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_hardening_wrong_params_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "sibling", nil
		}),
	)
	require.NoError(t, err)

	callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		switch {
		case request.GetCallerLabel() == "toolcall-review-wrongparams":
			atomic.AddInt32(&wrongParamsAICalls, 1)
			return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":42}}`)
		case aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()):
			return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
		default:
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}
	}
	runtimeID := "batch-hardening-wrong-params-" + ksuid.New().String()
	const sequenceStart int64 = 12200
	firstReviews := new(batchHardeningReviewRecorder)
	first := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, callback, firstReviews,
		func(index int, _ batchHardeningReviewMaterial) string {
			if index == 0 {
				return `{"suggestion":"wrong_params","extra_prompt":"use id 42"}`
			}
			return `{"suggestion":"continue"}`
		},
		target, sibling,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	firstResult, firstErr := first.ExecuteToolBatch(
		ctx,
		first.config.DefaultTask,
		batchHardeningRequest(target.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, firstErr)
	require.False(t, firstResult.DirectlyAnswer)
	require.Equal(t, int32(2), atomic.LoadInt32(&firstReviews.count))
	firstMaterials := firstReviews.snapshot()
	require.Len(t, firstMaterials, 2)
	require.Equal(t, target.Name, firstMaterials[0].Tool)
	require.Equal(t, int64(1), firstMaterials[0].Params.GetInt("id"))
	require.Equal(t, target.Name, firstMaterials[1].Tool)
	require.Equal(t, int64(42), firstMaterials[1].Params.GetInt("id"))
	require.Equal(t, int32(1), atomic.LoadInt32(&wrongParamsAICalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&targetInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, int64(42), atomic.LoadInt64(&finalID))
	require.Equal(t, target.Name, firstResult.Outcomes[0].FinalTool)
	require.Equal(t, int64(42), batchTestResultParamInt(t, firstResult.Outcomes[0].Result, "id"))

	secondReviews := new(batchHardeningReviewRecorder)
	second := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, callback, secondReviews, nil, target, sibling,
	)
	secondResult, secondErr := second.ExecuteToolBatch(
		ctx,
		second.config.DefaultTask,
		batchHardeningRequest(target.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, secondErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&secondReviews.count), "both persisted review decisions must replay without cards")
	require.Equal(t, int32(1), atomic.LoadInt32(&wrongParamsAICalls), "the finished wrong-params AI transaction must replay")
	require.Equal(t, int32(1), atomic.LoadInt32(&targetInvoked), "finished tool checkpoint must suppress a second plugin callback")
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked), "sibling checkpoint must suppress a second plugin callback")
	require.Equal(t, target.Name, secondResult.Outcomes[0].FinalTool)
	require.Equal(t, int64(42), batchTestResultParamInt(t, secondResult.Outcomes[0].Result, "id"))
	require.Equal(t, aicommon.ToolCallStageDone, secondResult.Outcomes[0].Stage)
}

func TestExecuteToolBatch_ManualReviewCheckpointReplay_WrongTool(t *testing.T) {
	var originalInvoked int32
	var replacementInvoked int32
	var siblingInvoked int32
	var replacementFinalID int64
	var wrongToolAICalls int32
	var paramGenerationAICalls int32
	original, err := aitool.New(
		"batch_hardening_wrong_tool_original",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&originalInvoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	replacement, err := aitool.New(
		"batch_hardening_wrong_tool_replacement",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&replacementInvoked, 1)
			atomic.StoreInt64(&replacementFinalID, params.GetInt("id"))
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_hardening_wrong_tool_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "sibling", nil
		}),
	)
	require.NoError(t, err)

	callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		switch {
		case request.GetCallerLabel() == "toolcall-review-wrongtool":
			atomic.AddInt32(&wrongToolAICalls, 1)
			return batchHardeningAIResponse(config, fmt.Sprintf(`{"@action":"require-tool","tool":%q}`, replacement.Name))
		case request.GetCallerLabel() == "toolcall-params":
			atomic.AddInt32(&paramGenerationAICalls, 1)
			return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":77}}`)
		case aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()):
			return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
		default:
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}
	}
	runtimeID := "batch-hardening-wrong-tool-" + ksuid.New().String()
	const sequenceStart int64 = 12300
	firstReviews := new(batchHardeningReviewRecorder)
	first := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, callback, firstReviews,
		func(index int, _ batchHardeningReviewMaterial) string {
			if index == 0 {
				return fmt.Sprintf(`{"suggestion":"wrong_tool","suggestion_tool":%q}`, replacement.Name)
			}
			return `{"suggestion":"continue"}`
		},
		original, replacement, sibling,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	firstResult, firstErr := first.ExecuteToolBatch(
		ctx,
		first.config.DefaultTask,
		batchHardeningRequest(original.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, firstErr)
	require.False(t, firstResult.DirectlyAnswer)
	require.Equal(t, int32(2), atomic.LoadInt32(&firstReviews.count))
	firstMaterials := firstReviews.snapshot()
	require.Len(t, firstMaterials, 2)
	require.Equal(t, original.Name, firstMaterials[0].Tool)
	require.Equal(t, int64(1), firstMaterials[0].Params.GetInt("id"))
	require.Equal(t, replacement.Name, firstMaterials[1].Tool)
	require.Equal(t, int64(77), firstMaterials[1].Params.GetInt("id"))
	require.Equal(t, int32(1), atomic.LoadInt32(&wrongToolAICalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&paramGenerationAICalls))
	require.Equal(t, int32(0), atomic.LoadInt32(&originalInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&replacementInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, int64(77), atomic.LoadInt64(&replacementFinalID))
	require.Equal(t, replacement.Name, firstResult.Outcomes[0].FinalTool)
	require.Equal(t, int64(77), batchTestResultParamInt(t, firstResult.Outcomes[0].Result, "id"))

	secondReviews := new(batchHardeningReviewRecorder)
	second := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart, callback, secondReviews, nil, original, replacement, sibling,
	)
	secondResult, secondErr := second.ExecuteToolBatch(
		ctx,
		second.config.DefaultTask,
		batchHardeningRequest(original.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, secondErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&secondReviews.count), "outer and replacement-tool reviews must both replay")
	require.Equal(t, int32(1), atomic.LoadInt32(&wrongToolAICalls), "tool re-selection transaction must replay")
	require.Equal(t, int32(1), atomic.LoadInt32(&paramGenerationAICalls), "replacement param transaction must replay")
	require.Equal(t, int32(0), atomic.LoadInt32(&originalInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&replacementInvoked), "finished replacement-tool checkpoint must suppress a second callback")
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, replacement.Name, secondResult.Outcomes[0].FinalTool)
	require.Equal(t, int64(77), batchTestResultParamInt(t, secondResult.Outcomes[0].Result, "id"))
	require.Equal(t, aicommon.ToolCallStageDone, secondResult.Outcomes[0].Stage)
}

func TestExecuteToolBatch_DirectWrongToolMutatesEachFinalProposalExactlyOnce(t *testing.T) {
	const markerParam = "proposal_marker"
	var originalMutations int32
	var replacementMutations int32
	var replacementSawLeakedMarker int32
	var originalInvoked int32
	var replacementInvoked int32
	var replacementCallbackMarker atomic.Value

	original, err := aitool.New(
		"batch_hardening_mutator_original",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithStringParam(markerParam),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&originalInvoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	replacement, err := aitool.New(
		"batch_hardening_mutator_replacement",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithStringParam(markerParam),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&replacementInvoked, 1)
			replacementCallbackMarker.Store(params.GetString(markerParam))
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_hardening_mutator_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return "sibling", nil
		}),
	)
	require.NoError(t, err)

	callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		switch {
		case request.GetCallerLabel() == "toolcall-review-wrongtool":
			return batchHardeningAIResponse(config, fmt.Sprintf(`{"@action":"require-tool","tool":%q}`, replacement.Name))
		case request.GetCallerLabel() == "toolcall-params":
			return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":77}}`)
		case aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()):
			return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
		default:
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}
	}
	reviews := new(batchHardeningReviewRecorder)
	react := newBatchHardeningReplayRuntime(
		t,
		"batch-hardening-mutator-"+ksuid.New().String(),
		12400,
		callback,
		reviews,
		func(index int, _ batchHardeningReviewMaterial) string {
			if index == 0 {
				return fmt.Sprintf(`{"suggestion":"wrong_tool","suggestion_tool":%q}`, replacement.Name)
			}
			return `{"suggestion":"continue"}`
		},
		original, replacement, sibling,
	)
	loop, err := reactloops.NewReActLoop(
		"batch-hardening-mutator-loop",
		react,
		reactloops.WithToolInvokeParamsMutator(func(toolName string, params aitool.InvokeParams) aitool.InvokeParams {
			switch toolName {
			case original.Name:
				atomic.AddInt32(&originalMutations, 1)
				params.Set(markerParam, "original")
			case replacement.Name:
				atomic.AddInt32(&replacementMutations, 1)
				if params.GetString(markerParam) != "" {
					atomic.StoreInt32(&replacementSawLeakedMarker, 1)
				}
				params.Set(markerParam, "replacement")
			}
			return params
		}),
	)
	require.NoError(t, err)
	react.config.DefaultTask.SetReActLoop(loop)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	result, execErr := react.ExecuteToolBatch(
		ctx,
		react.config.DefaultTask,
		batchHardeningRequest(original.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
	)
	require.NoError(t, execErr)
	require.False(t, result.DirectlyAnswer)
	require.Equal(t, int32(1), atomic.LoadInt32(&originalMutations), "original direct proposal is admission-mutated exactly once")
	require.Equal(t, int32(1), atomic.LoadInt32(&replacementMutations), "replacement's generated proposal is mutated exactly once")
	require.Equal(t, int32(0), atomic.LoadInt32(&replacementSawLeakedMarker), "replacement mutator must receive fresh replacement params")
	require.Equal(t, int32(0), atomic.LoadInt32(&originalInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&replacementInvoked))
	require.Equal(t, "replacement", replacementCallbackMarker.Load())
	require.Equal(t, replacement.Name, result.Outcomes[0].FinalTool)
	finalParams := batchHardeningResultParams(t, result.Outcomes[0].Result)
	require.Equal(t, int64(77), finalParams.GetInt("id"))
	require.Equal(t, "replacement", finalParams.GetString(markerParam))
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[0].Stage)

	materials := reviews.snapshot()
	require.Len(t, materials, 2)
	require.Equal(t, original.Name, materials[0].Tool)
	require.Equal(t, "original", materials[0].Params.GetString(markerParam))
	require.Equal(t, replacement.Name, materials[1].Tool)
	require.Equal(t, "replacement", materials[1].Params.GetString(markerParam))
}

func TestExecuteToolBatch_ReviewRepairFailuresAreChildLocal(t *testing.T) {
	tests := []struct {
		name            string
		reviewKind      string
		failurePoint    string
		wantReviewCount int32
	}{
		{
			name:            "wrong tool reselection AI failure",
			reviewKind:      "wrong_tool",
			failurePoint:    "repair_ai",
			wantReviewCount: 1,
		},
		{
			name:            "wrong tool recursive parameter generation failure",
			reviewKind:      "wrong_tool",
			failurePoint:    "recursive_param_generation",
			wantReviewCount: 1,
		},
		{
			name:            "wrong params regeneration AI failure",
			reviewKind:      "wrong_params",
			failurePoint:    "repair_ai",
			wantReviewCount: 1,
		},
		{
			name:            "wrong params recursive review failure",
			reviewKind:      "wrong_params",
			failurePoint:    "recursive_review",
			wantReviewCount: 2,
		},
	}

	for testIndex, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var originalInvoked int32
			var replacementInvoked int32
			var siblingInvoked int32

			original, err := aitool.New(
				fmt.Sprintf("batch_review_error_original_%d", testIndex),
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
					atomic.AddInt32(&originalInvoked, 1)
					return "original must not invoke after rejected review repair", nil
				}),
			)
			require.NoError(t, err)
			replacement, err := aitool.New(
				fmt.Sprintf("batch_review_error_replacement_%d", testIndex),
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
					atomic.AddInt32(&replacementInvoked, 1)
					return "replacement must not invoke after recursive failure", nil
				}),
			)
			require.NoError(t, err)
			sibling, err := aitool.New(
				fmt.Sprintf("batch_review_error_sibling_%d", testIndex),
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
					atomic.AddInt32(&siblingInvoked, 1)
					return "sibling executed", nil
				}),
			)
			require.NoError(t, err)

			callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				switch {
				case request.GetCallerLabel() == "toolcall-review-wrongtool":
					if test.reviewKind == "wrong_tool" && test.failurePoint == "repair_ai" {
						return nil, fmt.Errorf("forced wrong-tool reselection failure")
					}
					return batchHardeningAIResponse(config, fmt.Sprintf(`{"@action":"require-tool","tool":%q}`, replacement.Name))
				case request.GetCallerLabel() == "toolcall-review-wrongparams":
					if test.reviewKind == "wrong_params" && test.failurePoint == "repair_ai" {
						return nil, fmt.Errorf("forced wrong-params regeneration failure")
					}
					return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":42}}`)
				case request.GetCallerLabel() == "toolcall-params":
					if test.failurePoint == "recursive_param_generation" {
						return nil, fmt.Errorf("forced recursive parameter generation failure")
					}
					return batchHardeningAIResponse(config, `{"@action":"call-tool","params":{"id":77}}`)
				case aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()):
					return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
				default:
					return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
				}
			}

			reviews := new(batchHardeningReviewRecorder)
			react := newBatchHardeningReplayRuntime(
				t,
				fmt.Sprintf("batch-review-error-%d-%s", testIndex, ksuid.New().String()),
				12500+int64(testIndex*100),
				callback,
				reviews,
				func(index int, _ batchHardeningReviewMaterial) string {
					if index == 0 {
						if test.reviewKind == "wrong_tool" {
							return fmt.Sprintf(`{"suggestion":"wrong_tool","suggestion_tool":%q}`, replacement.Name)
						}
						return `{"suggestion":"wrong_params","extra_prompt":"use a repaired id"}`
					}
					if test.failurePoint == "recursive_review" {
						return `{"suggestion":"unsupported_recursive_decision"}`
					}
					return `{"suggestion":"continue"}`
				},
				original,
				replacement,
				sibling,
			)
			require.NoError(t, aicommon.WithAIAutoRetry(1)(react.config))
			require.NoError(t, aicommon.WithAITransactionAutoRetry(1)(react.config))

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			result, execErr := react.ExecuteToolBatch(
				ctx,
				react.config.DefaultTask,
				batchHardeningRequest(original.Name, sibling.Name, aitool.InvokeParams{"id": 1}),
			)
			require.NoError(t, execErr, "one child repair error must remain an all-settled batch outcome")
			require.False(t, result.DirectlyAnswer, "repair failures are not explicit direct-answer decisions")
			require.Len(t, result.Outcomes, 2)
			require.Equal(t, aicommon.ToolCallStagePrepareFailed, result.Outcomes[0].Stage)
			require.Error(t, result.Outcomes[0].Err)
			require.Nil(t, result.Outcomes[0].Result)
			require.False(t, result.Outcomes[0].DirectlyAnswer)
			require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
			require.NotNil(t, result.Outcomes[1].Result)
			require.Equal(t, int32(0), atomic.LoadInt32(&originalInvoked))
			require.Equal(t, int32(0), atomic.LoadInt32(&replacementInvoked))
			require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked), "a sibling must still invoke after another child fails review repair")
			require.Equal(t, test.wantReviewCount, atomic.LoadInt32(&reviews.count))

			committed := react.config.DefaultTask.GetAllToolCallResults()
			require.Len(t, committed, 1)
			require.Equal(t, sibling.Name, committed[0].Name)
		})
	}
}

func TestToolCaller_CancellationAfterAdmissionBoundariesSkipsCallbacks(t *testing.T) {
	t.Run("parameter generation gate", func(t *testing.T) {
		var aiCalls int32
		var toolCalls int32
		var releases int32
		tool, err := aitool.New(
			"batch_hardening_cancel_after_param_gate",
			aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
			aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				atomic.AddInt32(&toolCalls, 1)
				return "must not invoke", nil
			}),
		)
		require.NoError(t, err)
		react := newBatchTestReAct(t, tool, func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			atomic.AddInt32(&aiCalls, 1)
			return nil, fmt.Errorf("AI callback must not run after gate cancellation")
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		caller, err := aicommon.NewToolCaller(
			ctx,
			aicommon.WithToolCaller_AICallerConfig(react.config),
			aicommon.WithToolCaller_AICaller(react.config),
			aicommon.WithToolCaller_RuntimeId(react.config.GetRuntimeId()),
			aicommon.WithToolCaller_Emitter(react.config.GetEmitter()),
			aicommon.WithToolCaller_Task(react.config.DefaultTask),
			aicommon.WithToolCaller_Reason("cancel after parameter admission"),
			aicommon.WithToolCaller_GenerateToolParamsBuilder(func(_ *aitool.Tool, _ string) (string, error) {
				return "generate params", nil
			}),
			aicommon.WithToolCaller_ParamGenerationGate(func(context.Context) (func(), error) {
				cancel()
				return func() { atomic.AddInt32(&releases, 1) }, nil
			}),
		)
		require.NoError(t, err)
		_, _, callErr := caller.CallTool(tool)
		require.ErrorContains(t, callErr, context.Canceled.Error())
		require.Equal(t, int32(0), atomic.LoadInt32(&aiCalls))
		require.Equal(t, int32(0), atomic.LoadInt32(&toolCalls))
		require.Equal(t, int32(1), atomic.LoadInt32(&releases), "the acquired gate must be released on the cancellation re-check")
	})

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
			aicommon.WithToolCaller_AICaller(react.config),
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
		_, _, callErr := caller.CallToolWithExistedParams(tool, true, aitool.InvokeParams{"id": 1})
		require.ErrorIs(t, callErr, context.Canceled)
		require.Equal(t, int32(0), atomic.LoadInt32(&toolCalls))
		require.Equal(t, int32(1), atomic.LoadInt32(&releases), "beforeInvoke cleanup must run when cancellation wins after admission")
	})
}
