package test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func newRawTaskForRecovery(name, goal string) *aid.AiTask {
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

func TestRecovery_SkipCompletedTasks(t *testing.T) {
	sessionID := uuid.NewString()

	root := newRawTaskForRecovery("root", "root-goal")
	doneMarker := uuid.NewString()
	todoMarker := uuid.NewString()
	doneTask := newRawTaskForRecovery("done-task-"+doneMarker, "done-goal-"+doneMarker)
	todoTask := newRawTaskForRecovery("todo-task-"+todoMarker, "todo-goal-"+todoMarker)

	doneTask.ParentTask = root
	todoTask.ParentTask = root
	root.Subtasks = []*aid.AiTask{doneTask, todoTask}
	root.GenerateIndex()

	doneTask.SetStatus(aicommon.AITaskState_Completed)
	doneTask.SetSummary("done task summary")

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	t.Cleanup(func() {
		_ = db.Unscoped().
			Where("session_id = ?", sessionID).
			Delete(&schema.AISessionPlanAndExec{}).Error
	})

	coordinatorID := uuid.NewString()
	record := &schema.AISessionPlanAndExec{
		SessionID:     sessionID,
		CoordinatorID: coordinatorID,
		TaskTree:      string(utils.Jsonify(root)),
		TaskProgress:  string(utils.Jsonify(&aid.PlanAndExecProgress{Phase: "executing"})),
	}
	require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, record))

	var (
		mu          sync.Mutex
		pushed      = make(map[string]int)
		popped      = make(map[string]int)
		aiDoneCalls int
		aiTodoCalls int
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ins, err := aid.NewCoordinator(
		"recovery-skip-test",
		aicommon.WithContext(ctx),
		aicommon.WithID(coordinatorID),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_STRUCTURED {
				return
			}
			var payload map[string]any
			if err := json.Unmarshal(event.Content, &payload); err != nil {
				return
			}
			eventType := utils.InterfaceToString(payload["type"])
			if eventType != "push_task" && eventType != "pop_task" {
				return
			}
			taskRaw, ok := payload["task"]
			if !ok {
				return
			}
			taskMap, ok := taskRaw.(map[string]any)
			if !ok {
				return
			}
			idx := utils.InterfaceToString(taskMap["index"])
			if idx == "" {
				return
			}
			mu.Lock()
			if eventType == "push_task" {
				pushed[idx]++
			} else if eventType == "pop_task" {
				popped[idx]++
			}
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			block := extractCurrentTaskContent(prompt)
			if strings.Contains(block, doneMarker) {
				mu.Lock()
				aiDoneCalls++
				mu.Unlock()
			}
			if strings.Contains(block, todoMarker) {
				mu.Lock()
				aiTodoCalls++
				mu.Unlock()
			}
			rsp := config.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "direct-answer",
    "direct_answer": "ok",
    "direct_answer_long": "ok"
}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	require.NoError(t, ins.Run())

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, 0, pushed["1-1"], "completed task should not be pushed in recovery")
	require.Equal(t, 0, popped["1-1"], "completed task should not be popped in recovery")
	require.Equal(t, 1, pushed["1-2"], "pending task should be pushed exactly once in recovery")
	require.Equal(t, 1, popped["1-2"], "pending task should be popped exactly once in recovery")

	require.Equal(t, 0, aiDoneCalls, "completed task should not trigger AI calls in recovery")
	require.Greater(t, aiTodoCalls, 0, "pending task should trigger AI calls in recovery")
}
