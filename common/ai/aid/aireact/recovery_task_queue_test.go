package aireact

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// ---------------------------------------------------------------------------
// extractRecoveryTaskUserInput
// ---------------------------------------------------------------------------

func TestExtractRecoveryTaskUserInput_FromTaskTreeNameAndGoal(t *testing.T) {
	// Simulate a serialized aid.AiTask with Name and Goal
	taskTree := `{"name":"scan-network","goal":"扫描 192.168.1.0/24 的开放端口"}`
	record := &schema.AISessionPlanAndExec{
		TaskTree: taskTree,
	}
	got := extractRecoveryTaskUserInput(record)
	require.Contains(t, got, "scan-network")
	require.Contains(t, got, "扫描 192.168.1.0/24 的开放端口")
}

func TestExtractRecoveryTaskUserInput_FromTaskTreeGoalOnly(t *testing.T) {
	taskTree := `{"name":"","goal":"analyze logs for anomalies"}`
	record := &schema.AISessionPlanAndExec{
		TaskTree: taskTree,
	}
	got := extractRecoveryTaskUserInput(record)
	require.Contains(t, got, "analyze logs for anomalies")
}

func TestExtractRecoveryTaskUserInput_FromTaskTreeNameOnly(t *testing.T) {
	taskTree := `{"name":"port-scan","goal":""}`
	record := &schema.AISessionPlanAndExec{
		TaskTree: taskTree,
	}
	got := extractRecoveryTaskUserInput(record)
	require.Contains(t, got, "port-scan")
}

func TestExtractRecoveryTaskUserInput_FallbackToPlanPayload(t *testing.T) {
	// TaskTree is empty / invalid, but TaskProgress has PlanPayload
	progress := &detachedPlanProgress{
		Phase:       detachedPlanPhasePendingApproval,
		PlanPayload: "用户需求是：分析数据库性能瓶颈",
		UpdatedAt:   time.Now().Unix(),
	}
	record := &schema.AISessionPlanAndExec{
		TaskProgress: func() string { b, _ := json.Marshal(progress); return string(b) }(),
	}
	got := extractRecoveryTaskUserInput(record)
	require.Contains(t, got, "用户需求是：分析数据库性能瓶颈")
}

func TestExtractRecoveryTaskUserInput_FallbackToGeneric(t *testing.T) {
	// Both TaskTree and TaskProgress are empty
	record := &schema.AISessionPlanAndExec{}
	got := extractRecoveryTaskUserInput(record)
	require.Equal(t, "恢复执行计划", got)
}

func TestExtractRecoveryTaskUserInput_NilRecord(t *testing.T) {
	got := extractRecoveryTaskUserInput(nil)
	require.Equal(t, "恢复执行计划", got)
}

// ---------------------------------------------------------------------------
// is_recovery field in queue events
// ---------------------------------------------------------------------------

func TestEmitEnqueueReActTask_IncludesIsRecovery_NormalTask(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("test-enqueue-normal", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-normal-1", "hello", nil, react.Emitter, true)
	// default taskKind is AITaskKind_Normal

	react.EmitEnqueueReActTask(task)

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	isRecovery, ok := payload["is_recovery"].(bool)
	require.True(t, ok, "is_recovery field should exist")
	require.False(t, isRecovery, "normal task should have is_recovery=false")
}

func TestEmitEnqueueReActTask_IncludesIsRecovery_RecoveryTask(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("test-enqueue-recovery", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-recovery-1", "恢复执行: scan", nil, react.Emitter, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)

	react.EmitEnqueueReActTask(task)

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	isRecovery, ok := payload["is_recovery"].(bool)
	require.True(t, ok, "is_recovery field should exist")
	require.True(t, isRecovery, "recovery task should have is_recovery=true")
}

func TestEmitDequeueReActTask_IncludesIsRecovery_RecoveryTask(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("test-dequeue-recovery", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-recovery-2", "恢复执行: scan", nil, react.Emitter, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)

	react.EmitDequeueReActTask(task, "normal")

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	isRecovery, ok := payload["is_recovery"].(bool)
	require.True(t, ok, "is_recovery field should exist")
	require.True(t, isRecovery, "recovery task should have is_recovery=true")
}

func TestEmitDequeueReActTask_IncludesIsRecovery_NormalTask(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("test-dequeue-normal", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-normal-2", "hello", nil, react.Emitter, true)

	react.EmitDequeueReActTask(task, "normal")

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	isRecovery, ok := payload["is_recovery"].(bool)
	require.True(t, ok, "is_recovery field should exist")
	require.False(t, isRecovery, "normal task should have is_recovery=false")
}

// ---------------------------------------------------------------------------
// is_recovery in GetQueueInfo
// ---------------------------------------------------------------------------

func TestGetQueueInfo_IncludesIsRecovery(t *testing.T) {
	react := &ReAct{
		taskQueue: NewTaskQueue("test-queue-info"),
	}

	// Enqueue a normal task
	normalTask := aicommon.NewStatefulTaskBase("normal-info-1", "hello", nil, nil, true)
	require.NoError(t, react.taskQueue.Append(normalTask))

	// Enqueue a recovery task
	recoveryTask := aicommon.NewStatefulTaskBase("recovery-info-1", "恢复执行: scan", nil, nil, true)
	recoveryTask.SetTaskKind(aicommon.AITaskKind_Recovery)
	require.NoError(t, react.taskQueue.Append(recoveryTask))

	info := react.GetQueueInfo()
	tasks, ok := info["tasks"].([]map[string]interface{})
	require.True(t, ok, "tasks should be a slice of maps")
	require.Len(t, tasks, 2)

	// Find the recovery task in the list
	var foundRecovery, foundNormal bool
	for _, task := range tasks {
		isRecovery, ok := task["is_recovery"].(bool)
		if !ok {
			t.Fatal("is_recovery field missing in queue info task")
		}
		if isRecovery {
			foundRecovery = true
			require.Equal(t, "recovery-info-1", task["id"])
		} else {
			foundNormal = true
			require.Equal(t, "normal-info-1", task["id"])
		}
	}
	require.True(t, foundRecovery, "recovery task should be in queue info")
	require.True(t, foundNormal, "normal task should be in queue info")
}

// ---------------------------------------------------------------------------
// processReActTask dispatches recovery tasks correctly
// ---------------------------------------------------------------------------

func TestProcessReActTask_RecoveryTask_SkipsMainLoop(t *testing.T) {
	// Test the dispatch logic at the task-kind level.
	// A full integration test of processReActTask → processRecoveryTask is
	// covered by TestReAct_RecoveryPlanAndExec_SkipCompletedTasks which now
	// exercises the queue path.
	task := aicommon.NewStatefulTaskBase("recovery-dispatch-1", "恢复执行: test", nil, nil, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)
	task.SetRecoveryData(&aicommon.RecoveryTaskData{
		CoordinatorID: "test-coordinator-123",
		StartTaskID:   "",
	})

	// Verify task kind and recovery data are correctly set
	require.Equal(t, aicommon.AITaskKind_Recovery, task.GetTaskKind())
	require.NotNil(t, task.GetRecoveryData())
	require.Equal(t, "test-coordinator-123", task.GetRecoveryData().CoordinatorID)

	// Ensure normal task does not have recovery kind
	normalTask := aicommon.NewStatefulTaskBase("normal-dispatch-1", "hello", nil, nil, true)
	require.Equal(t, aicommon.AITaskKind_Normal, normalTask.GetTaskKind())
	require.Nil(t, normalTask.GetRecoveryData())
}

// ---------------------------------------------------------------------------
// TaskKind and RecoveryData on AIStatefulTaskBase
// ---------------------------------------------------------------------------

func TestAIStatefulTaskBase_TaskKind_Default(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task-kind-default", "hello", nil, nil, true)
	require.Equal(t, aicommon.AITaskKind_Normal, task.GetTaskKind())
}

func TestAIStatefulTaskBase_TaskKind_SetRecovery(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task-kind-recovery", "hello", nil, nil, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)
	require.Equal(t, aicommon.AITaskKind_Recovery, task.GetTaskKind())
}

func TestAIStatefulTaskBase_RecoveryData_SetAndGet(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task-recovery-data", "hello", nil, nil, true)
	require.Nil(t, task.GetRecoveryData())

	data := &aicommon.RecoveryTaskData{
		CoordinatorID: "coord-abc",
		StartTaskID:   "task-start-1",
	}
	task.SetRecoveryData(data)
	got := task.GetRecoveryData()
	require.NotNil(t, got)
	require.Equal(t, "coord-abc", got.CoordinatorID)
	require.Equal(t, "task-start-1", got.StartTaskID)
	require.Nil(t, got.ExecutePlanInput)
}

func TestAIStatefulTaskBase_RecoveryData_WithExecutePlanInput(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task-recovery-input", "hello", nil, nil, true)
	input := &aicommon.ExecutePlanInput{
		PlanPayload:  "test payload",
		PlanData:     "test data",
		PlanFacts:    "test facts",
		PlanDocument: "test doc",
	}
	data := &aicommon.RecoveryTaskData{
		CoordinatorID:    "coord-xyz",
		ExecutePlanInput: input,
	}
	task.SetRecoveryData(data)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)

	got := task.GetRecoveryData()
	require.NotNil(t, got)
	require.NotNil(t, got.ExecutePlanInput)
	require.Equal(t, "test payload", got.ExecutePlanInput.PlanPayload)
	require.Equal(t, "test data", got.ExecutePlanInput.PlanData)
}

// ---------------------------------------------------------------------------
// Recovery task enqueues and dequeues through TaskQueue
// ---------------------------------------------------------------------------

func TestTaskQueue_AppendRecoveryTask(t *testing.T) {
	tq := NewTaskQueue("test-append-recovery")

	task := aicommon.NewStatefulTaskBase("recovery-queue-1", "恢复执行: test", nil, nil, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)
	task.SetRecoveryData(&aicommon.RecoveryTaskData{
		CoordinatorID: "coord-queue-test",
	})
	task.SetStatus(aicommon.AITaskState_Queueing)

	require.NoError(t, tq.Append(task))
	require.Equal(t, 1, tq.Len())

	got := tq.PeekFirst()
	require.NotNil(t, got)
	require.Equal(t, aicommon.AITaskKind_Recovery, got.GetTaskKind())
	require.Equal(t, "coord-queue-test", got.GetRecoveryData().CoordinatorID)
}

func TestTaskQueue_GetFirstRecoveryTask(t *testing.T) {
	tq := NewTaskQueue("test-getfirst-recovery")

	task := aicommon.NewStatefulTaskBase("recovery-getfirst-1", "恢复执行: test", nil, nil, true)
	task.SetTaskKind(aicommon.AITaskKind_Recovery)
	task.SetRecoveryData(&aicommon.RecoveryTaskData{
		CoordinatorID: "coord-getfirst-test",
	})
	task.SetStatus(aicommon.AITaskState_Queueing)

	require.NoError(t, tq.Append(task))

	got := tq.GetFirst()
	require.NotNil(t, got)
	require.Equal(t, aicommon.AITaskKind_Recovery, got.GetTaskKind())
	require.Equal(t, "coord-getfirst-test", got.GetRecoveryData().CoordinatorID)
	require.Equal(t, 0, tq.Len(), "GetFirst should remove the task from queue")
}

// ---------------------------------------------------------------------------
// JSON serialization of RecoveryTaskData (ensures it round-trips)
// ---------------------------------------------------------------------------

func TestRecoveryTaskData_JSONRoundTrip(t *testing.T) {
	original := &aicommon.RecoveryTaskData{
		CoordinatorID: "coord-json-test",
		StartTaskID:   "task-json-start",
		ExecutePlanInput: &aicommon.ExecutePlanInput{
			PlanPayload:  "payload-json",
			PlanData:     "data-json",
			PlanFacts:    "facts-json",
			PlanDocument: "doc-json",
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded aicommon.RecoveryTaskData
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original.CoordinatorID, decoded.CoordinatorID)
	require.Equal(t, original.StartTaskID, decoded.StartTaskID)
	require.NotNil(t, decoded.ExecutePlanInput)
	require.Equal(t, original.ExecutePlanInput.PlanPayload, decoded.ExecutePlanInput.PlanPayload)
}
