package aireact

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func extractCurrentTaskContentFromPrompt(prompt string) string {
	if idx := strings.LastIndex(prompt, "--- CURRENT_TASK ---"); idx >= 0 {
		rest := prompt[idx+len("--- CURRENT_TASK ---"):]
		if end := strings.Index(rest, "--- CURRENT_TASK_END ---"); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
	}
	re := regexp.MustCompile(`(?s)<\\|CURRENT_TASK\\|>\\s*(.*?)\\s*<\\|CURRENT_TASK_END\\|>`)
	matches := re.FindStringSubmatch(prompt)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func newRecoveryTaskForReAct(name, goal string) *aid.AiTask {
	base := aicommon.NewStatefulTaskBase(
		"plan-task-"+uuid.NewString(),
		"任务名称: "+name+"\n任务目标: "+goal,
		context.Background(),
		nil,
		true,
	)
	base.SetName(name)
	return &aid.AiTask{
		AIStatefulTaskBase: base,
		Name:               name,
		Goal:               goal,
	}
}

func TestReAct_RecoveryPlanAndExec_SkipCompletedTasks(t *testing.T) {
	sessionID := uuid.NewString()
	coordinatorID := uuid.NewString()

	doneMarker := uuid.NewString()
	todoMarker := uuid.NewString()

	root := newRecoveryTaskForReAct("root", "root-goal")
	doneTask := newRecoveryTaskForReAct("doneTask-"+doneMarker, "goal-"+doneMarker)
	todoTask := newRecoveryTaskForReAct("todoTask-"+todoMarker, "goal-"+todoMarker)

	doneTask.ParentTask = root
	todoTask.ParentTask = root
	root.Subtasks = []*aid.AiTask{doneTask, todoTask}
	root.GenerateIndex()

	doneTask.SetStatus(aicommon.AITaskState_Completed)
	doneTask.SetSummary("completed-" + doneMarker)
	doneIndex := doneTask.Index
	todoIndex := todoTask.Index

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	t.Cleanup(func() {
		_ = db.Unscoped().
			Where("coordinator_id = ?", coordinatorID).
			Delete(&schema.AISessionPlanAndExec{}).Error
	})

	record := &schema.AISessionPlanAndExec{
		SessionID:     sessionID,
		CoordinatorID: coordinatorID,
		TaskTree:      string(utils.Jsonify(root)),
		TaskProgress:  string(utils.Jsonify(&aid.PlanAndExecProgress{Phase: "executing"})),
	}
	require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, record))

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var mu sync.Mutex
	doneCalls := 0
	todoCalls := 0

	_, err := NewTestReAct(
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			idx := req.GetTaskIndex()
			handledByIndex := false
			if idx == doneIndex {
				mu.Lock()
				doneCalls++
				mu.Unlock()
				handledByIndex = true
			} else if idx == todoIndex {
				mu.Lock()
				todoCalls++
				mu.Unlock()
				handledByIndex = true
			}

			if !handledByIndex {
				current := extractCurrentTaskContentFromPrompt(prompt)
				if strings.Contains(current, doneMarker) {
					mu.Lock()
					doneCalls++
					mu.Unlock()
				}
				if strings.Contains(current, todoMarker) {
					mu.Lock()
					todoCalls++
					mu.Unlock()
				}
			}

			rsp := cfg.NewAIResponse()
			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "summary", "status_summary": "ok", "task_short_summary": "ok", "task_long_summary": "ok"}`))
			} else if strings.Contains(prompt, "directly_answer") {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "directly_answer", "answer_payload": "ok"}`))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{"@action": "direct-answer", "direct_answer": "ok", "direct_answer_long": "ok"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	syncID := ksuid.New().String()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_RECOVERY_PLAN_AND_EXEC,
		SyncJsonInput: `{"coordinator_id":"` + coordinatorID + `"}`,
		SyncID:        syncID,
	}

	var (
		syncStarted    bool
		planStarted    bool
		planEnded      bool
		recoveredIDOK  bool
		sessionIDOK    bool
		recoveryTaskOK bool
	)

	after := time.After(20 * time.Second)
LOOP:
	for {
		select {
		case e := <-out:
			if e.IsSync && e.NodeId == "recover_plan_and_exec" && e.SyncID == syncID {
				var payload map[string]any
				if err := json.Unmarshal(e.Content, &payload); err == nil {
					if errMsg, ok := payload["error"].(string); ok && errMsg != "" {
						t.Fatalf("recovery sync error: %s", errMsg)
					}
					if started, ok := payload["started"].(bool); ok && started {
						syncStarted = true
					}
					if gotID, ok := payload["coordinator_id"].(string); ok && gotID == coordinatorID {
						recoveredIDOK = true
					}
					if gotSession, ok := payload["session_id"].(string); ok && gotSession == sessionID {
						sessionIDOK = true
					}
				}
			}

			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				result := utils.InterfaceToString(jsonpath.FindFirst(e.Content, `$..coordinator_id`))
				if result == coordinatorID {
					planStarted = true
					var payload map[string]any
					if err := json.Unmarshal(e.Content, &payload); err == nil {
						recoveryTaskID := utils.InterfaceToString(payload["re-act_task"])
						if strings.HasPrefix(recoveryTaskID, recoveryTaskIDPrefix) {
							recoveryTaskOK = true
						}
					}
				}
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				result := utils.InterfaceToString(jsonpath.FindFirst(e.Content, `$..coordinator_id`))
				if result == coordinatorID {
					planEnded = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}

	close(in)

	require.True(t, syncStarted, "expected recovery sync event to start")
	require.True(t, recoveredIDOK, "expected recovery sync to carry coordinator_id")
	require.True(t, sessionIDOK, "expected recovery sync to carry session_id")
	require.True(t, planStarted, "expected recovery plan execution to start")
	require.True(t, recoveryTaskOK, "expected recovery plan execution to use recovery task id prefix")
	require.True(t, planEnded, "expected recovery plan execution to end")

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 0, doneCalls, "completed task should not trigger AI calls in recovery")
	require.Greater(t, todoCalls, 0, "pending task should trigger AI calls in recovery")
}
