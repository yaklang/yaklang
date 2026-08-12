package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_JumpQueueCancelsRunningDirectBatchWithoutCommit starts a native
// direct batch from an actual primary-decision response, then jumps a queued
// task to the front while both plugin callbacks are blocked in their contexts.
// It protects the integration boundary between queue cancellation and the
// batch coordinator: cancelled children must settle without becoming task or
// Timeline results, unfinished artifact bundles must be rolled back, and the
// successor task must still run to completion.
func TestReAct_JumpQueueCancelsRunningDirectBatchWithoutCommit(t *testing.T) {
	const (
		firstInput     = "run cancellable direct batch before queue jump"
		successorInput = "run queued successor after direct batch cancellation"
		firstToolName  = "jump_cancel_batch_first"
		secondToolName = "jump_cancel_batch_second"
	)

	runtimeCtx, stopRuntime := context.WithCancel(context.Background())
	defer stopRuntime()
	workdir := t.TempDir()

	callbackStarted := make(chan string, 2)
	callbackCancelled := make(chan string, 2)
	newBlockingTool := func(name string) *aitool.Tool {
		tool, err := aitool.New(
			name,
			aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithNoRuntimeCallback(func(ctx context.Context, _ aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
				_, _ = fmt.Fprintf(stdout, "%s partial stdout before jump\n", name)
				_, _ = fmt.Fprintf(stderr, "%s partial stderr before jump\n", name)
				callbackStarted <- name
				<-ctx.Done()
				// Exercise the late-write guard while the bundle is being cancelled.
				_, _ = fmt.Fprintf(stdout, "%s observed cancellation\n", name)
				callbackCancelled <- name
				return nil, ctx.Err()
			}),
		)
		require.NoError(t, err)
		return tool
	}
	firstTool := newBlockingTool(firstToolName)
	secondTool := newBlockingTool(secondToolName)

	respond := func(config aicommon.AICallerConfigIf, raw string) (*aicommon.AIResponse, error) {
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(raw))
		response.Close()
		return response, nil
	}

	var successorPrimaryCalls int32
	const jumpSyncID = "jump-cancel-running-direct-batch"
	jumpObserved := make(chan struct{}, 1)
	cancelObserved := make(chan struct{}, 1)

	react, err := NewTestReAct(
		aicommon.WithContext(runtimeCtx),
		aicommon.WithWorkdir(workdir),
		aicommon.WithTools(firstTool, secondTool),
		aicommon.WithAgreeYOLO(),
		aicommon.WithToolBatchInvokeConcurrency(2),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.SyncID != jumpSyncID {
				return
			}
			switch event.NodeId {
			case "react_task_jumped_queue":
				select {
				case jumpObserved <- struct{}{}:
				default:
				}
			case REACT_TASK_cancelled:
				select {
				case cancelObserved <- struct{}{}:
				default:
				}
			}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			switch {
			case aicommon.IsVerifySatisfactionPrompt(prompt):
				return respond(config, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"done"}`)

			case aicommon.IsPrimaryDecisionPrompt(prompt):
				if strings.Contains(prompt, successorInput) {
					atomic.AddInt32(&successorPrimaryCalls, 1)
					return respond(config, `{"@action":"finish","identifier":"finish_queued_successor"}`)
				}
				if !strings.Contains(prompt, firstInput) {
					return nil, fmt.Errorf("primary prompt belongs to neither jump-queue task")
				}
				return respond(config, `{
  "@action": "directly_call_tool",
  "identifier": "running_batch_before_jump",
  "human_readable_thought": "Run two cancellable children concurrently",
  "directly_call_tool_calls": [
    {
      "tool_name": "jump_cancel_batch_first",
      "params": {},
      "identifier": "jump_first_child",
      "reason": "Block the first child until queue cancellation"
    },
    {
      "tool_name": "jump_cancel_batch_second",
      "params": {},
      "identifier": "jump_second_child",
      "reason": "Block the second child until queue cancellation"
    }
  ]
}`)

			case aicommon.IsDirectAnswerPrompt(prompt):
				return respond(config, `{"@action":"directly_answer","answer_payload":"queued successor completed"}`)

			default:
				return nil, fmt.Errorf("unexpected AI prompt in batch jump-queue E2E: %.200s", prompt)
			}
		}),
	)
	require.NoError(t, err)

	require.NoError(t, react.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: firstInput}))

	startedNames := make(map[string]bool, 2)
	for len(startedNames) < 2 {
		select {
		case name := <-callbackStarted:
			startedNames[name] = true
		case <-time.After(8 * time.Second):
			t.Fatalf("only %d direct-batch callbacks started before timeout: %v", len(startedNames), startedNames)
		}
	}
	require.Equal(t, map[string]bool{firstToolName: true, secondToolName: true}, startedNames)

	firstTask := react.GetCurrentTask()
	require.NotNil(t, firstTask)
	require.Equal(t, firstInput, firstTask.GetUserInput())

	require.NoError(t, react.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: successorInput}))

	var successorTask aicommon.AIStatefulTask
	queueDeadline := time.After(5 * time.Second)
	queuePoll := time.NewTicker(10 * time.Millisecond)
	defer queuePoll.Stop()
WAIT_FOR_SUCCESSOR:
	for {
		for _, task := range react.taskQueue.GetQueueingTasks() {
			if task != nil && task.GetUserInput() == successorInput {
				successorTask = task
				break WAIT_FOR_SUCCESSOR
			}
		}
		select {
		case <-queuePoll.C:
		case <-queueDeadline:
			t.Fatal("successor task was not queued while the direct batch was running")
		}
	}
	require.Equal(t, aicommon.AITaskState_Queueing, successorTask.GetStatus())

	require.NoError(t, react.SendInputEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_REACT_JUMP_QUEUE,
		SyncJsonInput: fmt.Sprintf(`{"task_id":%q}`, successorTask.GetId()),
		SyncID:        jumpSyncID,
	}))

	select {
	case <-jumpObserved:
	case <-time.After(5 * time.Second):
		t.Fatal("jump-queue success event was not observed")
	}
	select {
	case <-cancelObserved:
	case <-time.After(5 * time.Second):
		t.Fatal("jump-queue cancellation event was not observed")
	}

	cancelledNames := make(map[string]bool, 2)
	for len(cancelledNames) < 2 {
		select {
		case name := <-callbackCancelled:
			cancelledNames[name] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d callbacks observed context cancellation: %v", len(cancelledNames), cancelledNames)
		}
	}
	require.Equal(t, map[string]bool{firstToolName: true, secondToolName: true}, cancelledNames)

	successorDeadline := time.After(15 * time.Second)
	successorPoll := time.NewTicker(10 * time.Millisecond)
	defer successorPoll.Stop()
	for successorTask.GetStatus() != aicommon.AITaskState_Completed {
		select {
		case <-successorPoll.C:
		case <-successorDeadline:
			t.Fatalf("queued successor did not complete after jump; status=%s", successorTask.GetStatus())
		}
	}
	require.Positive(t, atomic.LoadInt32(&successorPrimaryCalls), "queued successor never reached its primary decision")
	require.Equal(t, aicommon.AITaskState_Aborted, firstTask.GetStatus())
	require.Empty(t, firstTask.GetAllToolCallResults(), "cancelled batch children must not commit task results")

	// A text-level cancellation record is expected, but neither cancelled child
	// may become a typed tool result in the shared Timeline.
	for _, id := range react.config.Timeline.GetTimelineItemIDs() {
		item, ok := react.config.Timeline.GetIdToTimelineItem().Get(id)
		if !ok || item == nil {
			continue
		}
		toolResult, ok := item.GetValue().(*aitool.ToolResult)
		if !ok || toolResult == nil {
			continue
		}
		require.NotContains(t, []string{firstToolName, secondToolName}, toolResult.Name,
			"cancelled child was committed as a Timeline tool result")
	}

	toolCallRoot := filepath.Join(
		workdir,
		aicommon.BuildTaskDirName(firstTask.GetIndex(), firstTask.GetSemanticIdentifier()),
		"tool_calls",
	)
	artifactEntries, readErr := os.ReadDir(toolCallRoot)
	if !os.IsNotExist(readErr) {
		require.NoError(t, readErr)
		require.Empty(t, artifactEntries, "cancelled direct-batch artifact bundles must be removed")
	}

	var finishedToolCheckpoints int
	require.NoError(t, react.config.GetDB().Model(&schema.AiCheckpoint{}).
		Where("coordinator_uuid = ? AND type = ? AND finished = ?", react.config.GetRuntimeId(), schema.AiCheckpointType_ToolCall, true).
		Count(&finishedToolCheckpoints).Error)
	require.Zero(t, finishedToolCheckpoints, "cancelled batch callbacks must not leave replayable finished checkpoints")
}
