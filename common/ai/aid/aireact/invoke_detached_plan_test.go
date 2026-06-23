package aireact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
		PlanFacts:    "facts",
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
}

func TestHandleSyncTypeExecuteDetachedPlanEvent_UsesRecoveryPath(t *testing.T) {
	testHandleSyncTypeExecuteDetachedPlanEvent(t, "legacy")
}

func TestHandleSyncTypeExecuteDetachedPlanEvent_WithCoordinatorIDOnly(t *testing.T) {
	testHandleSyncTypeExecuteDetachedPlanEvent(t, "coordinator_only")
}

func TestHandleSyncTypeExecuteDetachedPlanEvent_WithPlansOnly(t *testing.T) {
	testHandleSyncTypeExecuteDetachedPlanEvent(t, "plans")
}

func testHandleSyncTypeExecuteDetachedPlanEvent(t *testing.T, mode string) {
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
		PlanFacts:    "facts",
		PlanDocument: "document",
	}
	coordinatorID, err := reactIns.PublishDetachedPlan(context.Background(), input, "react-task-async")
	require.NoError(t, err)

	reactIns.config.HijackPERequest = func(ctx context.Context, payload string) error {
		return nil
	}

	syncPayload := map[string]any{
		"coordinator_id": coordinatorID,
	}
	switch mode {
	case "legacy":
		syncPayload["session_id"] = sessionID
		syncPayload["react_task_id"] = "react-task-async"
		syncPayload["plan_payload"] = input.PlanPayload
		syncPayload["plan_data"] = input.PlanData
		syncPayload["plan_facts"] = input.PlanFacts
		syncPayload["plan_document"] = input.PlanDocument
	case "plans":
		syncPayload["plans"] = map[string]any{
			"root_task": map[string]any{
				"task_id": "pe-task-1",
				"index":   "1",
				"name":    "test-plan",
				"goal":    "verify detached recovery execute",
				"subtasks": []map[string]any{
					{
						"task_id": "pe-task-1-1",
						"index":   "1-1",
						"name":    "step-1",
						"goal":    "do something",
					},
				},
			},
			"facts":    "facts",
			"document": "document",
		}
	case "coordinator_only":
	default:
		t.Fatalf("unsupported execute detached plan test mode: %s", mode)
	}
	syncJSON, err := json.Marshal(syncPayload)
	require.NoError(t, err)

	require.NoError(t, reactIns.HandleSyncTypeExecuteDetachedPlanEvent(&ypb.AIInputEvent{
		SyncJsonInput: string(syncJSON),
		SyncID:        uuid.NewString(),
	}))

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
