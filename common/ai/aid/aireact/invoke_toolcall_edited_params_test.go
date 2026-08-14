package aireact

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestExecuteToolBatch_ExplicitEditedParamsSecondReviewFeedbackAndReplay(t *testing.T) {
	var invoked int32
	var callbackID int64
	var siblingInvoked int32
	tool, err := aitool.New(
		"batch_explicit_edited_params_target",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			atomic.StoreInt64(&callbackID, params.GetInt("id"))
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_explicit_edited_params_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "sibling", nil
		}),
	)
	require.NoError(t, err)

	runtimeID := "batch-explicit-edited-params-" + ksuid.New().String()
	const sequenceStart int64 = 13100
	firstReviews := new(batchHardeningReviewRecorder)
	first := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart,
		func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()) {
				return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
			}
			return nil, context.Canceled
		},
		firstReviews,
		func(index int, _ batchHardeningReviewMaterial) string {
			if index == 0 {
				return `{"suggestion":"continue","params":{"id":42}}`
			}
			// Some clients echo the visible params when approving. Equal params
			// must approve this proposal instead of creating an infinite review loop.
			return `{"suggestion":"continue","params":{"id":42}}`
		},
		tool, sibling,
	)
	request := batchHardeningRequest(tool.Name, sibling.Name, aitool.InvokeParams{"id": 1})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	firstResult, execErr := first.ExecuteToolBatch(ctx, first.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Equal(t, int32(1), atomic.LoadInt32(&invoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, int64(42), atomic.LoadInt64(&callbackID), "the real callback must receive the edited value")
	require.Equal(t, int64(42), batchHardeningResultParams(t, firstResult.Outcomes[0].Result).GetInt("id"))
	materials := firstReviews.snapshot()
	require.Len(t, materials, 2, "an edited value is a new proposal and requires a second approval")
	require.Equal(t, int64(1), materials[0].Params.GetInt("id"))
	require.Equal(t, int64(42), materials[1].Params.GetInt("id"))

	secondReviews := new(batchHardeningReviewRecorder)
	second := newBatchHardeningReplayRuntime(
		t, runtimeID, sequenceStart,
		func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()) {
				return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
			}
			return nil, context.Canceled
		},
		secondReviews, nil, tool, sibling,
	)
	secondResult, replayErr := second.ExecuteToolBatch(ctx, second.config.DefaultTask, request)
	require.NoError(t, replayErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&secondReviews.count), "both finished approvals replay without duplicate cards")
	require.Equal(t, int32(1), atomic.LoadInt32(&invoked), "finished tool checkpoint suppresses a duplicate callback")
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked), "the sibling checkpoint also suppresses a duplicate callback")
	require.Equal(t, int64(42), batchHardeningResultParams(t, secondResult.Outcomes[0].Result).GetInt("id"))
}

func TestExecuteToolBatch_WrongParamsExplicitEditSkipsAIRepair(t *testing.T) {
	var invoked int32
	var siblingInvoked int32
	tool, err := aitool.New(
		"batch_wrong_params_explicit_edit_target",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_wrong_params_explicit_edit_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "sibling", nil
		}),
	)
	require.NoError(t, err)
	reviews := new(batchHardeningReviewRecorder)
	react := newBatchHardeningReplayRuntime(
		t, "batch-wrong-params-explicit-"+ksuid.New().String(), 13200,
		func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()) {
				return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
			}
			return nil, context.Canceled
		},
		reviews,
		func(index int, _ batchHardeningReviewMaterial) string {
			if index == 0 {
				return `{"suggestion":"wrong_params","params":{"id":24}}`
			}
			return `{"suggestion":"continue"}`
		},
		tool, sibling,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, batchHardeningRequest(
		tool.Name, sibling.Name, aitool.InvokeParams{"id": 1},
	))
	require.NoError(t, execErr)
	require.Equal(t, int32(1), atomic.LoadInt32(&invoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, int64(24), batchHardeningResultParams(t, result.Outcomes[0].Result).GetInt("id"))
	require.Equal(t, int32(2), atomic.LoadInt32(&reviews.count))
}

func TestExecuteToolBatch_InvalidExplicitEditIsChildLocal(t *testing.T) {
	var rejectedInvoked int32
	var siblingInvoked int32
	rejected, err := aitool.New(
		"batch_invalid_explicit_edit_target",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			atomic.AddInt32(&rejectedInvoked, 1)
			return nil, nil
		}),
	)
	require.NoError(t, err)
	sibling, err := aitool.New(
		"batch_invalid_explicit_edit_sibling",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			atomic.AddInt32(&siblingInvoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	reviews := new(batchHardeningReviewRecorder)
	react := newBatchHardeningReplayRuntime(
		t, "batch-invalid-explicit-edit-"+ksuid.New().String(), 13300, nil, reviews,
		func(_ int, _ batchHardeningReviewMaterial) string {
			return `{"suggestion":"continue","params":{"id":"not-an-integer"}}`
		},
		rejected, sibling,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, batchHardeningRequest(
		rejected.Name, sibling.Name, aitool.InvokeParams{"id": 1},
	))
	require.NoError(t, execErr)
	require.Equal(t, aicommon.ToolCallStagePrepareFailed, result.Outcomes[0].Stage)
	require.ErrorContains(t, result.Outcomes[0].Err, "invalid review edited params")
	require.Equal(t, int32(0), atomic.LoadInt32(&rejectedInvoked))
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
	require.Equal(t, int32(1), atomic.LoadInt32(&siblingInvoked))
	require.Equal(t, int32(1), atomic.LoadInt32(&reviews.count), "invalid edits must fail before a second review")
}
