package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPublishDetachedPlan_PersistsSessionAndEmitsEvent(t *testing.T) {
	sessionID := uuid.NewString()
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)

	out := make(chan *ypb.AIOutputEvent, 8)
	reactIns, err := NewTestReAct(
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	planData := string(utils.Jsonify(map[string]any{
		"@action":        "plan",
		"main_task":      "test-plan",
		"main_task_goal": "verify detached plan",
		"tasks": []map[string]any{
			{"subtask_name": "step-1", "subtask_goal": "do something"},
		},
	}))
	input := &aicommon.ExecutePlanInput{
		PlanPayload:  "user query",
		PlanData:     planData,
		PlanDocument: "document",
	}

	coordinatorID, err := reactIns.PublishDetachedPlan(context.Background(), input, "react-task-1")
	require.NoError(t, err)
	require.NotEmpty(t, coordinatorID)

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, sessionID, record.SessionID)
	require.Contains(t, record.TaskTree, "test-plan")
	require.False(t, aicommon.ShouldExposePlanExecTaskRecord(record.TaskProgress))

	var detachedEvent *ypb.AIOutputEvent
	for evt := range out {
		if evt.GetType() == string(schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE) {
			detachedEvent = evt
			break
		}
	}
	require.NotNil(t, detachedEvent)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(evtContent(detachedEvent), &payload))
	require.Equal(t, true, payload["detached"])
	require.Equal(t, coordinatorID, payload["coordinator_id"])

	timelineText := collectTimelineText(reactIns)
	require.Contains(t, timelineText, "[DETACHED_PLAN]")
	require.Contains(t, timelineText, coordinatorID)
	require.Contains(t, timelineText, "PLAN DEFINITION")
	require.NotContains(t, timelineText, "Plan data:")
	require.Contains(t, record.TaskTree, "step-1")
	require.Contains(t, record.TaskTree, "do something")
}

func TestPublishDetachedPlan_PreservesNestedDefinitionAndDocument(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessionID := uuid.NewString()
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	reactIns, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithPersistentSessionId(sessionID), aicommon.WithWorkdir(t.TempDir()))
	require.NoError(t, err)
	input := &aicommon.ExecutePlanInput{
		PlanPayload: "user query",
		PlanData: `{"name":"main-plan","goal":"main goal","subtasks":[
			{"name":"parent-task","goal":"parent goal","subtasks":[
				{"name":"child-task","goal":"child goal"}
			]}
		]}`,
		PlanDocument: "# Nested plan document",
	}
	coordinatorID, err := reactIns.PublishDetachedPlan(ctx, input, "react-task-1")
	require.NoError(t, err)
	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	require.NoError(t, err)
	require.Equal(t, sessionID, record.SessionID)
	content := collectTimelineText(reactIns)
	require.Contains(t, content, "[DETACHED_PLAN]")
	require.Contains(t, content, coordinatorID)
	// The native snapshot owns the document and definition used to rebuild
	// stable context partitions. The Timeline carries the publication receipt.
	var progress coordinator.Progress
	require.NoError(t, json.Unmarshal([]byte(record.TaskProgress), &progress))
	require.Equal(t, coordinator.Name, progress.PlanEngine)
	require.NotNil(t, progress.CoordinatorState)
	require.NotNil(t, progress.CoordinatorState.Plan)
	require.True(t, progress.CoordinatorState.ReviewPending)
	definition := progress.CoordinatorState.PlanDefinition()
	for _, name := range []string{"main-plan", "parent-task", "child-task"} {
		require.Contains(t, record.TaskTree, name)
		require.Contains(t, definition, name)
	}
	require.Contains(t, definition, "PLAN DEFINITION")
	require.Equal(t, input.PlanDocument, progress.CoordinatorState.Plan.Document)
	require.NotContains(t, content, "child goal", "the publication receipt must not duplicate the plan tree")
}

func collectTimelineText(reactIns *ReAct) string {
	if reactIns == nil || reactIns.config == nil || reactIns.config.Timeline == nil {
		return ""
	}
	items := reactIns.config.Timeline.GetTimelineOutput()
	var sb strings.Builder
	for _, item := range items {
		if item == nil {
			continue
		}
		sb.WriteString(item.Content)
		sb.WriteRune('\n')
	}
	return sb.String()
}

func TestHandleSyncTypeExecuteDetachedPlanEvent_UsesRecoveryPath(t *testing.T) {
	sessionID := uuid.NewString()
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)

	out := make(chan *ypb.AIOutputEvent, 32)
	reactIns, err := NewTestReAct(
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	require.NoError(t, err)

	planData := string(utils.Jsonify(map[string]any{
		"@action":        "plan",
		"main_task":      "test-plan",
		"main_task_goal": "verify detached recovery execute",
		"tasks": []map[string]any{
			{"subtask_name": "step-1", "subtask_goal": "do something"},
		},
	}))
	input := &aicommon.ExecutePlanInput{
		PlanPayload:  "user query",
		PlanData:     planData,
		PlanDocument: "document",
	}
	coordinatorID, err := reactIns.PublishDetachedPlan(context.Background(), input, "react-task-async")
	require.NoError(t, err)

	reactIns.config.HijackPERequest = func(ctx context.Context, payload string) error {
		return nil
	}

	syncPayload, err := json.Marshal(map[string]any{
		"coordinator_id": coordinatorID,
		"session_id":     sessionID,
		"react_task_id":  "react-task-async",
		"plan_payload":   input.PlanPayload,
		"plan_data":      input.PlanData,
		"plan_document":  input.PlanDocument,
	})
	require.NoError(t, err)

	require.NoError(t, reactIns.HandleSyncTypeExecuteDetachedPlanEvent(&ypb.AIInputEvent{
		SyncJsonInput: string(syncPayload),
		SyncID:        uuid.NewString(),
	}))

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Contains(t, record.TaskProgress, `"phase":"NotCompleted"`)
	require.True(t, aicommon.ShouldExposePlanExecTaskRecord(record.TaskProgress))

	var sawRecoveryTask bool
	deadline := time.After(10 * time.Second)
	for !sawRecoveryTask {
		select {
		case evt := <-out:
			if evt.GetType() != string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(evtContent(evt), &payload); err != nil {
				continue
			}
			reactTaskID := utils.InterfaceToString(payload["re-act_task"])
			if strings.HasPrefix(reactTaskID, recoveryTaskIDPrefix) {
				sawRecoveryTask = true
			}
		case <-deadline:
			t.Fatal("expected detached plan execute to start via recovery task")
		}
	}
}

func evtContent(evt *ypb.AIOutputEvent) []byte {
	if evt == nil {
		return nil
	}
	return []byte(evt.GetContent())
}

func TestHandleSyncTypeExecuteDetachedPlanEvent_LegacyStoredAndEditedTree(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored_tree", true: "edited_tree"}[edited], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var syncError, visibleError bool
			cfg := aicommon.NewConfig(ctx, aicommon.WithPersistentSessionId(uuid.NewString()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithNoOpMemoryTriage(), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				var data map[string]any
				_ = json.Unmarshal(e.Content, &data)
				if e.NodeId == "execute_detached_plan" && e.IsSync && e.SyncID == "approve-legacy" && data["error"] != nil {
					syncError = true
				}
				if data["level"] == "error" && strings.Contains(fmt.Sprint(data["message"]), "计划未能开始执行") {
					visibleError = true
				}
			}))
			require.NoError(t, cfg.GetDB().AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
			// Retired records stay intact for inspection. Approval must reject
			// them rather than silently constructing the old execution runtime.
			r := &ReAct{config: cfg, Emitter: cfg.GetEmitter(), taskQueue: NewTaskQueue(MainTaskQueueName)}
			id := uuid.NewString()
			tree := `{"name":"Plan","goal":"Check sources","semantic_identifier":"plan","subtasks":[{"name":"Group","goal":"Collect","semantic_identifier":"group","subtasks":[{"name":"Read","goal":"Original brief","semantic_identifier":"read"}]},{"name":"Verify","goal":"Check result","semantic_identifier":"verify","depends_on":["read"]}]}`
			require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(cfg.GetDB(), &schema.AISessionPlanAndExec{SessionID: cfg.PersistentSessionId, CoordinatorID: id, TaskTree: tree, TaskProgress: string(utils.Jsonify(map[string]any{"plan_engine": "coordinator_legacy", "phase": aicommon.PlanExecPhaseDetachedPendingApproval, "plan_document": "Keep document"}))}))
			payload := map[string]any{"coordinator_id": id}
			if edited {
				payload["plans"] = map[string]any{"root_task": json.RawMessage(strings.ReplaceAll(tree, "Original brief", "Approved edited brief"))}
			}
			raw, err := json.Marshal(payload)
			require.NoError(t, err)
			require.NotPanics(t, func() {
				require.NoError(t, r.HandleSyncTypeExecuteDetachedPlanEvent(&ypb.AIInputEvent{SyncJsonInput: string(raw), SyncID: "approve-legacy"}))
			})
			require.Empty(t, r.taskQueue.GetQueueingTasks())
			require.True(t, syncError, "approval must receive an error acknowledgement")
			require.True(t, visibleError, "clients that close the review panel need a visible failure")
			record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(cfg.GetDB(), id)
			require.NoError(t, err)
			require.Contains(t, record.TaskTree, `"semantic_identifier":"read"`)
			require.Contains(t, record.TaskTree, `"depends_on":["read"]`)
			require.Equal(t, tree, record.TaskTree)
			require.Contains(t, record.TaskProgress, aicommon.PlanExecPhaseDetachedPendingApproval)
		})
	}
}

func TestParseExecuteDetachedPlanParams_RejectsEmptyEditedDocument(t *testing.T) {
	for _, value := range []any{"", "  \n", nil, 42} {
		raw, err := json.Marshal(map[string]any{"coordinator_id": "pending-plan", "plans": map[string]any{"document": value}})
		require.NoError(t, err)
		_, _, _, _, err = parseExecuteDetachedPlanParams(string(raw))
		require.Error(t, err, "an explicitly invalid edit must not fall back to the stored document")
	}
	_, _, _, input, err := parseExecuteDetachedPlanParams(`{"coordinator_id":"pending-plan","plans":{"document":"# 用户最终文档"}}`)
	require.NoError(t, err)
	require.Equal(t, "# 用户最终文档", input.PlanDocument)
	_, _, _, input, err = parseExecuteDetachedPlanParams(`{"coordinator_id":"pending-plan"}`)
	require.NoError(t, err, "omitted fields retain the existing stored-plan contract")
	require.Empty(t, input.PlanDocument)
}
