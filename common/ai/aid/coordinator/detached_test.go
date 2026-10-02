package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCoordinatorLoopDetachedNativeApprovalAndRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	var calls atomic.Int64
	var eventsMu sync.Mutex
	var events []*schema.AiOutputEvent
	options := []aicommon.ConfigOption{
		aicommon.WithPersistentSessionId("native-detached-session"),
		aicommon.WithEnableDetachedPlan(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			copy := *e
			copy.Content = append([]byte(nil), e.Content...)
			events = append(events, &copy)
		}),
	}
	initialOptions := append(append([]aicommon.ConfigOption(nil), options...), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		switch calls.Add(1) {
		case 1:
			return nativeResponse(c, req, "create_plan", map[string]any{"plan": map[string]any{"name": "Detached", "goal": "Execute only after approval", "tasks": []any{map[string]any{"name": "Check", "goal": "Original task brief", "identifier": "check", "depends_on": []string{}}}}, "plan_document": "# Pending document"})
		case 2:
			return nativeResponse(c, req, "submit_plan", map[string]any{"plan_version": 1})
		case 3:
			// Publication is not execution permission, even under YOLO.
			return nativeResponse(c, req, "start_tasks", map[string]any{})
		default:
			return nativeResponse(c, req, "finish", map[string]any{})
		}
	}))
	initial, err := coordinator.NewSession(ctx, "Prepare a detached plan", initialOptions...)
	require.NoError(t, err)
	initial.Config.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(initial.GetRuntimeId(), db)
	require.NoError(t, initial.RunPlanOnly())
	require.Equal(t, int64(4), calls.Load())
	eventsMu.Lock()
	panels := 0
	for _, e := range events {
		if e.Type == schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE {
			panels++
		}
		require.NotEqual(t, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, e.Type)
		if e.NodeId == "system" {
			require.NotContains(t, string(e.Content), `"type":"push_task"`)
		}
	}
	eventsMu.Unlock()
	require.Equal(t, 1, panels)
	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, initial.GetRuntimeId())
	require.NoError(t, err)
	require.Contains(t, record.TaskProgress, `"plan_engine":"coordinator"`)
	require.Contains(t, record.TaskProgress, aicommon.PlanExecPhaseDetachedPendingApproval)
	var edited map[string]any
	require.NoError(t, json.Unmarshal([]byte(record.TaskTree), &edited))
	edited["subtasks"].([]any)[0].(map[string]any)["goal"] = "Approved edited task brief"
	editedJSON, err := json.Marshal(edited)
	require.NoError(t, err)
	var workerCalls atomic.Int64
	taskPattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+); observed=(true|false)`)
	resumeOptions := append(append([]aicommon.ConfigOption(nil), options...), aicommon.WithID(initial.GetRuntimeId()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if strings.Contains(prompt, "Execute the assigned frozen plan task.") {
			require.Contains(t, prompt, "Approved edited task brief")
			if workerCalls.Add(1) == 1 {
				return nativeResponse(c, req, "submit_task_result", map[string]any{"summary": "Approved edit verified"})
			}
			return nativeResponse(c, req, "finish", map[string]any{})
		}
		match := taskPattern.FindStringSubmatch(prompt)
		if len(match) == 0 {
			return nil, fmt.Errorf("restored plan missing from PLAN STATUS")
		}
		switch match[2] {
		case "pending":
			return nativeResponse(c, req, "start_tasks", map[string]any{})
		case "running":
			return nativeResponse(c, req, "wait_tasks", map[string]any{"timeout_seconds": 1})
		case "awaiting_review":
			if match[4] == "false" {
				return nativeResponse(c, req, "inspect_tasks", map[string]any{})
			}
			id, _ := strconv.ParseUint(match[3], 10, 64)
			return nativeResponse(c, req, "review_task", map[string]any{"task_id": match[1], "attempt_id": id, "decision": "accept", "reason": "The inspected execution verified the approved edit."})
		default:
			return nativeResponse(c, req, "finish", map[string]any{})
		}
	}))
	// A legacy client does not send an engine selector on recovery.
	resumed, err := coordinator.NewSession(ctx, "Execute the approved edit", resumeOptions...)
	require.NoError(t, err)
	resumed.Config.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(resumed.GetRuntimeId(), db)
	root, err := resumed.BuildRootTaskFromPlanData(string(editedJSON), "Execute the approved edit")
	require.NoError(t, err)
	require.NotNil(t, resumed.LiteForgeExecutor)
	require.NoError(t, resumed.CommitApprovedPlan(root, "# Approved edited document"))
	require.NoError(t, resumed.RunExecuteApprovedPlan())
	require.Equal(t, int64(2), workerCalls.Load())
	record, err = yakit.GetAISessionPlanAndExecByCoordinatorID(db, resumed.GetRuntimeId())
	require.NoError(t, err)
	require.Contains(t, record.TaskTree, "Approved edited task brief")
	require.Contains(t, record.TaskProgress, `"finished":true`)
}

func TestCoordinatorLoopInputNotificationFollowsJournal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()))
	done := make(chan struct{})
	cfg.InputEventManager.RegisterAfterInputEvent("test-journal-order", func(e *ypb.AIInputEvent) {
		if e.SyncType != aicommon.SYNC_TYPE_USER_INTERVENTION {
			return
		}
		history := cfg.GetUserInputHistory()
		require.Len(t, history, 1)
		require.Equal(t, "Reconsider the next task", history[0].UserInput)
		// Callbacks must run outside the processor lock.
		cfg.InputEventManager.UnregisterAfterInputEvent("test-journal-order")
		close(done)
	})
	cfg.StartEventLoop(ctx)
	cfg.EventInputChan.SafeFeed(&ypb.AIInputEvent{IsSyncMessage: true, SyncType: aicommon.SYNC_TYPE_USER_INTERVENTION, SyncJsonInput: `{"content":"Reconsider the next task"}`})
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("input notification did not follow the journal write")
	}
}
