package aicommon

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolCaller_ExplicitEditedParamsValueFeedback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cfg *Config
	var reviewMu sync.Mutex
	reviewCount := 0
	cfg = NewTestConfig(ctx,
		WithID("edited-params-feedback-"+ksuid.New().String()),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			var payload struct {
				ID string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(event.Content, &payload))
			reviewMu.Lock()
			reviewCount++
			index := reviewCount
			reviewMu.Unlock()
			response := aitool.InvokeParams{"suggestion": "continue", "params": map[string]any{"id": 42}}
			if index > 2 {
				t.Errorf("unexpected extra review card %d", index)
			}
			cfg.Epm.Feed(payload.ID, response)
		}),
	)

	valueFeedbackSubmitterMu.Lock()
	previousSubmitter := valueFeedbackSubmitter
	valueFeedbackSubmitterMu.Unlock()
	var feedbackMu sync.Mutex
	var feedback []*ValueFeedbackRecord
	RegisterValueFeedbackSubmitter(func(_ *Config, record *ValueFeedbackRecord) {
		feedbackMu.Lock()
		defer feedbackMu.Unlock()
		feedback = append(feedback, record)
	})
	t.Cleanup(func() { RegisterValueFeedbackSubmitter(previousSubmitter) })

	var callbackParams aitool.InvokeParams
	tool, err := aitool.New(
		"edited_params_feedback_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			callbackParams = cloneEndpointParams(params)
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	caller, err := NewToolCaller(
		ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Task(cfg.DefaultTask),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_Reason("verify explicit edited params feedback"),
	)
	require.NoError(t, err)
	result, directlyAnswer, callErr := caller.CallToolWithExistedParams(tool, true, aitool.InvokeParams{"id": 1})
	require.NoError(t, callErr)
	require.False(t, directlyAnswer)
	require.NotNil(t, result)
	require.Equal(t, int64(42), callbackParams.GetInt("id"))
	require.Equal(t, 2, reviewCount, "edited params must be approved as a fresh proposal")

	feedbackMu.Lock()
	feedbackSnapshot := append([]*ValueFeedbackRecord(nil), feedback...)
	feedbackMu.Unlock()
	var editedApproval *ValueFeedbackApproval
	for _, record := range feedbackSnapshot {
		if record != nil && record.Approval != nil &&
			aitool.InvokeParams(record.Approval.OriginalValue).GetInt("id") == 1 {
			editedApproval = record.Approval
			break
		}
	}
	require.NotNil(t, editedApproval)
	require.Equal(t, ApprovalDecisionApproveWithEdit, editedApproval.Decision)
	require.True(t, editedApproval.Changed)
	require.Equal(t, int64(1), aitool.InvokeParams(editedApproval.OriginalValue).GetInt("id"))
	require.Equal(t, int64(42), aitool.InvokeParams(editedApproval.FinalValue).GetInt("id"))
}

func TestReviewEditedParams_ContinueWithoutParamsRemainsScalarCompatible(t *testing.T) {
	caller := &ToolCaller{ctx: context.Background()}
	tool := aitool.NewWithoutCallback("compat-tool")
	original := aitool.InvokeParams{"id": 1}
	gotTool, gotParams, result, next, err := caller.review(
		tool,
		original,
		aitool.InvokeParams{"suggestion": "continue"},
		func(any) {},
	)
	require.NoError(t, err)
	require.Same(t, tool, gotTool)
	require.Equal(t, original, gotParams)
	require.Nil(t, result)
	require.Equal(t, HandleToolUseNext_Default, next)
}

func TestReviewEditedParams_RejectsNonObject(t *testing.T) {
	_, present, err := reviewEditedParams(aitool.InvokeParams{"params": "not-an-object"})
	require.True(t, present)
	require.ErrorContains(t, err, "must be a JSON object")
}

func TestReviewEditedParams_WrongParamsUnchangedDoesNotLoop(t *testing.T) {
	tool, err := aitool.New(
		"unchanged-edited-params-tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }),
	)
	require.NoError(t, err)
	caller := &ToolCaller{ctx: context.Background()}
	original := aitool.InvokeParams{"id": 1}
	gotTool, gotParams, result, next, reviewErr := caller.review(
		tool,
		original,
		aitool.InvokeParams{
			"suggestion": "wrong_params",
			"params":     map[string]any{"id": 1},
		},
		func(any) {},
	)
	require.NoError(t, reviewErr)
	require.Same(t, tool, gotTool)
	require.Equal(t, original, gotParams)
	require.Nil(t, result)
	require.Equal(t, HandleToolUseNext_Default, next)
}
