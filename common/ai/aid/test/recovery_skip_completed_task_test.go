package test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

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

func collectTaskProgressByIndex(task map[string]any, result map[string]string) {
	if task == nil {
		return
	}
	index := utils.InterfaceToString(task["index"])
	if index != "" {
		result[index] = utils.InterfaceToString(task["progress"])
	}
	subtasks, _ := task["subtasks"].([]any)
	for _, sub := range subtasks {
		subTask, _ := sub.(map[string]any)
		collectTaskProgressByIndex(subTask, result)
	}
}

func coordinatorRootTaskForTest(t *testing.T, cod *aid.Coordinator) *aid.AiTask {
	t.Helper()
	require.NotNil(t, cod)
	v := reflect.ValueOf(cod).Elem().FieldByName("rootTask")
	require.True(t, v.IsValid())
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Interface().(*aid.AiTask)
}

func TestRecovery_SkipCompletedTasks(t *testing.T) {
	sessionID := uuid.NewString()

	root := newRawTaskForRecovery("root", "root-goal")
	doneMarker := uuid.NewString()
	abortedMarker := uuid.NewString()
	todoMarker := uuid.NewString()
	doneTask := newRawTaskForRecovery("done-task-"+doneMarker, "done-goal-"+doneMarker)
	abortedTask := newRawTaskForRecovery("aborted-task-"+abortedMarker, "aborted-goal-"+abortedMarker)
	todoTask := newRawTaskForRecovery("todo-task-"+todoMarker, "todo-goal-"+todoMarker)

	doneTask.ParentTask = root
	abortedTask.ParentTask = root
	todoTask.ParentTask = root
	root.Subtasks = []*aid.AiTask{doneTask, abortedTask, todoTask}
	root.GenerateIndex()

	doneTask.SetStatus(aicommon.AITaskState_Completed)
	doneTask.SetSummary("done task summary")
	abortedTask.SetStatus(aicommon.AITaskState_Aborted)
	abortedTask.SetSummary("aborted task should be retried")

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
		mu                      sync.Mutex
		pushed                  = make(map[string]int)
		popped                  = make(map[string]int)
		firstPlanTaskProgress   map[string]string
		firstPlanProgressRecord bool
		aiDoneCalls             int
		aiAbortedCalls          int
		aiTodoCalls             int
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
			if event == nil {
				return
			}

			if event.Type == schema.EVENT_TYPE_PLAN {
				var payload map[string]any
				if err := json.Unmarshal(event.Content, &payload); err != nil {
					return
				}
				rootTask, _ := payload["root_task"].(map[string]any)
				if rootTask == nil {
					return
				}
				mu.Lock()
				if !firstPlanProgressRecord {
					firstPlanTaskProgress = make(map[string]string)
					collectTaskProgressByIndex(rootTask, firstPlanTaskProgress)
					firstPlanProgressRecord = true
				}
				mu.Unlock()
				return
			}

			if event.Type != schema.EVENT_TYPE_STRUCTURED {
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
			if strings.Contains(block, abortedMarker) {
				mu.Lock()
				aiAbortedCalls++
				mu.Unlock()
			}
			if strings.Contains(block, todoMarker) {
				mu.Lock()
				aiTodoCalls++
				mu.Unlock()
			}
			rsp := config.NewAIResponse()
			if utils.MatchAllOfSubString(prompt, "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "summary",
    "status_summary": "ok",
    "task_short_summary": "ok",
    "task_long_summary": "ok"
}`))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "directly_answer",
    "answer_payload": "ok"
}`))
			}
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
	require.Equal(t, 1, pushed["1-2"], "aborted task should be pushed exactly once in recovery")
	require.Equal(t, 1, popped["1-2"], "aborted task should be popped exactly once in recovery")
	require.Equal(t, 1, pushed["1-3"], "pending task should be pushed exactly once in recovery")
	require.Equal(t, 1, popped["1-3"], "pending task should be popped exactly once in recovery")
	require.Equal(t, string(aicommon.AITaskState_Completed), firstPlanTaskProgress["1-1"], "completed task should stay completed in recovered task tree")
	require.Equal(t, string(aicommon.AITaskState_Aborted), firstPlanTaskProgress["1-2"], "aborted task should stay aborted in recovered task tree before retry")

	require.Equal(t, 0, aiDoneCalls, "completed task should not trigger AI calls in recovery")
	require.Greater(t, aiAbortedCalls, 0, "aborted task should trigger AI calls in recovery")
	require.Greater(t, aiTodoCalls, 0, "pending task should trigger AI calls in recovery")
}

func TestRecovery_StartFromSpecifiedTask(t *testing.T) {
	sessionID := uuid.NewString()

	root := newRawTaskForRecovery("root", "root-goal")
	firstMarker := uuid.NewString()
	startMarker := uuid.NewString()
	lastMarker := uuid.NewString()
	firstTask := newRawTaskForRecovery("first-task-"+firstMarker, "first-goal-"+firstMarker)
	startTask := newRawTaskForRecovery("start-task-"+startMarker, "start-goal-"+startMarker)
	lastTask := newRawTaskForRecovery("last-task-"+lastMarker, "last-goal-"+lastMarker)

	firstTask.ParentTask = root
	startTask.ParentTask = root
	lastTask.ParentTask = root
	root.Subtasks = []*aid.AiTask{firstTask, startTask, lastTask}
	root.GenerateIndex()

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
		mu                    sync.Mutex
		pushed                = make(map[string]int)
		popped                = make(map[string]int)
		firstPlanTaskProgress map[string]string
		firstPlanRecorded     bool
		aiFirstCalls          int
		aiStartCalls          int
		aiLastCalls           int
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ins, err := aid.NewCoordinator(
		"recovery-start-from-specified-task",
		aicommon.WithContext(ctx),
		aicommon.WithID(coordinatorID),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithAgreePolicy(aicommon.AgreePolicyYOLO),
		aid.WithRecoveryStartTaskIndex(startTask.Index),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil {
				return
			}
			if event.Type == schema.EVENT_TYPE_PLAN {
				var payload map[string]any
				if err := json.Unmarshal(event.Content, &payload); err != nil {
					return
				}
				rootTask, _ := payload["root_task"].(map[string]any)
				if rootTask == nil {
					return
				}
				mu.Lock()
				if !firstPlanRecorded {
					firstPlanTaskProgress = make(map[string]string)
					collectTaskProgressByIndex(rootTask, firstPlanTaskProgress)
					firstPlanRecorded = true
				}
				mu.Unlock()
				return
			}

			if event.Type != schema.EVENT_TYPE_STRUCTURED {
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
			} else {
				popped[idx]++
			}
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			block := extractCurrentTaskContent(request.GetPrompt())
			mu.Lock()
			switch {
			case strings.Contains(block, firstMarker):
				aiFirstCalls++
			case strings.Contains(block, startMarker):
				aiStartCalls++
			case strings.Contains(block, lastMarker):
				aiLastCalls++
			}
			mu.Unlock()

			rsp := config.NewAIResponse()
			if utils.MatchAllOfSubString(request.GetPrompt(), "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "summary",
    "status_summary": "ok",
    "task_short_summary": "ok",
    "task_long_summary": "ok"
}`))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "directly_answer",
    "answer_payload": "ok"
}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	require.NoError(t, ins.Run())

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, 0, pushed[firstTask.Index], "tasks before the specified start task should not execute")
	require.Equal(t, 0, popped[firstTask.Index], "tasks before the specified start task should not execute")
	require.Equal(t, 1, pushed[startTask.Index], "specified start task should execute")
	require.Equal(t, 1, popped[startTask.Index], "specified start task should execute")
	require.Equal(t, 1, pushed[lastTask.Index], "tasks after the specified start task should continue executing")
	require.Equal(t, 1, popped[lastTask.Index], "tasks after the specified start task should continue executing")
	require.Equal(t, string(aicommon.AITaskState_Skipped), firstPlanTaskProgress[firstTask.Index], "tasks before the specified start task should be marked skipped in recovered tree")
	require.Equal(t, 0, aiFirstCalls, "tasks before the specified start task should not trigger AI calls")
	require.Greater(t, aiStartCalls, 0, "specified start task should trigger AI calls")
	require.Greater(t, aiLastCalls, 0, "tasks after the specified start task should trigger AI calls")
}

func TestRecovery_CancelledTaskPersistsAbortedState(t *testing.T) {
	sessionID := uuid.NewString()
	coordinatorID := uuid.NewString()

	root := newRawTaskForRecovery("root", "root-goal")
	cancelMarker := uuid.NewString()
	cancelTask := newRawTaskForRecovery("cancel-task-"+cancelMarker, "cancel-goal-"+cancelMarker)
	cancelTask.ParentTask = root
	root.Subtasks = []*aid.AiTask{cancelTask}
	root.GenerateIndex()

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	t.Cleanup(func() {
		_ = db.Unscoped().
			Where("session_id = ?", sessionID).
			Delete(&schema.AISessionPlanAndExec{}).Error
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cancelOnce sync.Once
	ins, err := aid.NewCoordinator(
		"recovery-cancel-state-test",
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
			if utils.InterfaceToString(payload["type"]) != "push_task" {
				return
			}
			taskMap, _ := payload["task"].(map[string]any)
			if utils.InterfaceToString(taskMap["index"]) == "1-1" {
				cancelOnce.Do(cancel)
			}
		}),
		aid.WithPlanMocker(func(_ *aid.Coordinator) *aid.PlanResponse {
			return &aid.PlanResponse{RootTask: root}
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if strings.Contains(extractCurrentTaskContent(request.GetPrompt()), cancelMarker) {
				return nil, context.Canceled
			}
			rsp := config.NewAIResponse()
			if utils.MatchAllOfSubString(request.GetPrompt(), "status_summary", "task_long_summary", "task_short_summary") {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "summary",
    "status_summary": "ok",
    "task_short_summary": "ok",
    "task_long_summary": "ok"
}`))
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{
    "@action": "directly_answer",
    "answer_payload": "ok"
}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	runErr := ins.Run()
	if runErr != nil {
		require.Contains(t, strings.ToLower(runErr.Error()), "context")
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	require.NoError(t, err)
	require.NotNil(t, record)

	var recoveredTree map[string]any
	require.NoError(t, json.Unmarshal([]byte(record.TaskTree), &recoveredTree))
	progressByIndex := make(map[string]string)
	collectTaskProgressByIndex(recoveredTree, progressByIndex)
	require.Equal(t, string(aicommon.AITaskState_Aborted), progressByIndex["1-1"], "cancelled task should persist as aborted in task tree")

	var persistedProgress aid.PlanAndExecProgress
	require.NoError(t, json.Unmarshal([]byte(record.TaskProgress), &persistedProgress))
	require.Equal(t, 1, persistedProgress.AbortedTasks, "cancelled task should be counted as aborted")
	require.Equal(t, 0, persistedProgress.CompletedTasks, "cancelled task should not be counted as completed")
	require.Equal(t, "1-1", persistedProgress.CurrentTaskIndex)

	inMemoryRoot := coordinatorRootTaskForTest(t, ins)
	require.NotNil(t, inMemoryRoot)
	require.Len(t, inMemoryRoot.Subtasks, 1)
	require.Equal(t, aicommon.AITaskState_Aborted, inMemoryRoot.Subtasks[0].GetStatus(), "cancelled task should remain aborted in coordinator rootTask")
}
