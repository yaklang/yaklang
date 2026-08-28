package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestExecuteToolBatch_IntervalReviewEventsStayWithChildAndCancelIsIsolated(t *testing.T) {
	const (
		cancelCallID    = "interval-review-cancel-child"
		continueCallID  = "interval-review-continue-child"
		cancelMarker    = "cancel-only-this-child-marker"
		continueMarker  = "continue-sibling-marker"
		intervalNodeID  = "interval-review"
		callbackTimeout = 15 * time.Second
	)

	cancelContextObserved := make(chan struct{})
	var cancelContextOnce sync.Once
	continueReviewCompleted := make(chan struct{})
	var continueReviewOnce sync.Once
	var eventsMu sync.Mutex
	var events []*schema.AiOutputEvent

	tool, err := aitool.New(
		"batch_interval_review_owner_tool",
		aitool.WithStringParam("marker", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			switch params.GetString("marker") {
			case cancelMarker:
				select {
				case <-ctx.Done():
					cancelContextOnce.Do(func() { close(cancelContextObserved) })
					return nil, ctx.Err()
				case <-time.After(callbackTimeout):
					return nil, errors.New("cancel child did not receive its interval-review cancellation")
				}
			case continueMarker:
				select {
				case <-continueReviewCompleted:
					return "continue child completed independently", nil
				case <-ctx.Done():
					return nil, fmt.Errorf("continue child was cancelled with its sibling: %w", ctx.Err())
				case <-time.After(callbackTimeout):
					return nil, errors.New("continue child did not complete an interval review")
				}
			default:
				return nil, fmt.Errorf("unexpected marker %q", params.GetString("marker"))
			}
		}),
	)
	require.NoError(t, err)

	react, err := NewTestReAct(
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithToolCallerIntervalReviewDuration(10*time.Millisecond),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			decision := ""
			reason := ""
			switch prompt := request.GetPrompt(); {
			case strings.Contains(prompt, cancelMarker):
				decision = "cancel"
				reason = "cancel only the first child"
			case strings.Contains(prompt, continueMarker):
				decision = "continue"
				reason = "the sibling remains healthy"
			default:
				return nil, errors.New("interval-review prompt did not contain either child marker")
			}

			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(
				`{"@action":"interval-toolcall-review","decision":%q,"reason":%q,"progress_summary":"running","estimated_remaining_time":"1s"}`,
				decision,
				reason,
			)))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			copyEvent := *event
			copyEvent.Content = append([]byte(nil), event.Content...)
			copyEvent.StreamDelta = append([]byte(nil), event.StreamDelta...)
			copyEvent.ProcessesId = append([]string(nil), event.ProcessesId...)
			eventsMu.Lock()
			events = append(events, &copyEvent)
			eventsMu.Unlock()

			if event.Type != schema.EVENT_TOOL_CALL_PROGRESS_REVIEW || event.CallToolID != continueCallID {
				return
			}
			var payload aicommon.ToolCallProgressReviewPayload
			if json.Unmarshal(event.Content, &payload) == nil &&
				payload.Phase == schema.TOOL_CALL_PROGRESS_REVIEW_PHASE_COMPLETED &&
				payload.Decision == "continue" {
				continueReviewOnce.Do(func() { close(continueReviewCompleted) })
			}
		}),
	)
	require.NoError(t, err)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)

	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{
			Mode:            aicommon.ToolCallModeDirect,
			ToolName:        tool.Name,
			Reason:          "exercise cancellation for only one child",
			Params:          aitool.InvokeParams{"marker": cancelMarker},
			ExecutionCallID: cancelCallID,
		},
		{
			Mode:            aicommon.ToolCallModeDirect,
			ToolName:        tool.Name,
			Reason:          "prove the sibling continues independently",
			Params:          aitool.InvokeParams{"marker": continueMarker},
			ExecutionCallID: continueCallID,
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 2)
	select {
	case <-cancelContextObserved:
		// The cancel decision reached the first ToolCaller's private context.
	default:
		t.Fatal("cancel child did not observe context cancellation")
	}
	require.Contains(t, []aicommon.ToolCallStage{
		aicommon.ToolCallStageCancelled,
		aicommon.ToolCallStageInvokeFailed,
	}, result.Outcomes[0].Stage)
	require.Error(t, result.Outcomes[0].Err)
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
	require.NoError(t, result.Outcomes[1].Err)
	require.NotNil(t, result.Outcomes[1].Result)
	require.True(t, result.Outcomes[1].Result.Success)

	react.Emitter.WaitForStream()
	eventsMu.Lock()
	snapshot := append([]*schema.AiOutputEvent(nil), events...)
	eventsMu.Unlock()

	seenStream := map[string]int{
		cancelCallID:   0,
		continueCallID: 0,
	}
	seenProgress := map[string]int{
		cancelCallID:   0,
		continueCallID: 0,
	}
	for _, event := range snapshot {
		isReviewStream := event.NodeId == intervalNodeID
		isReviewProgress := event.Type == schema.EVENT_TOOL_CALL_PROGRESS_REVIEW
		if !isReviewStream && !isReviewProgress {
			continue
		}
		_, isKnownChild := seenStream[event.CallToolID]
		require.True(t, isKnownChild, "review event escaped child ownership: type=%s node=%s call_tool_id=%q", event.Type, event.NodeId, event.CallToolID)
		require.Contains(t, event.ProcessesId, event.CallToolID)
		if isReviewStream {
			seenStream[event.CallToolID]++
		}
		if isReviewProgress {
			seenProgress[event.CallToolID]++
			var payload aicommon.ToolCallProgressReviewPayload
			require.NoError(t, json.Unmarshal(event.Content, &payload))
			require.Equal(t, event.CallToolID, payload.CallToolID)
		}
	}
	for _, callID := range []string{cancelCallID, continueCallID} {
		require.Positive(t, seenStream[callID], "child %s must own its interval-review stream", callID)
		require.Positive(t, seenProgress[callID], "child %s must own its progress-review lifecycle", callID)
	}
}
