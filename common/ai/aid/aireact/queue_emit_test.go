package aireact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitDequeueReActTask_SetsTaskId(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("react-dequeue-taskid", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-dequeue-1", "hello", nil, react.Emitter, true)

	react.EmitDequeueReActTask(task, "normal")

	require.Len(t, events, 1)
	require.Equal(t, REACT_TASK_dequeue, events[0].NodeId)
	require.Equal(t, "task-dequeue-1", events[0].TaskId)
	require.Equal(t, task.GetUUID(), events[0].TaskUUID)
}

func TestEmitEnqueueReActTask_SetsTaskId(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("react-enqueue-taskid", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-enqueue-1", "hello", nil, react.Emitter, true)

	react.EmitEnqueueReActTask(task)

	require.Len(t, events, 1)
	require.Equal(t, REACT_TASK_enqueue, events[0].NodeId)
	require.Equal(t, "task-enqueue-1", events[0].TaskId)
	require.Equal(t, task.GetUUID(), events[0].TaskUUID)
}

func parsePayload(t *testing.T, content []byte) map[string]any {
	t.Helper()
	payload := map[string]any{}
	require.NoError(t, json.Unmarshal(content, &payload))
	return payload
}

func TestEmitEnqueueReActTask_IncludesUserInputUUID(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("react-enqueue-uuid", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-enqueue-uuid", "hello", nil, react.Emitter, true)
	task.SetUserInputUUID("ui-uuid-enqueue-123")

	react.EmitEnqueueReActTask(task)

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	require.Equal(t, "ui-uuid-enqueue-123", payload["react_task_user_input_uuid"])
}

func TestEmitDequeueReActTask_IncludesUserInputUUID(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("react-dequeue-uuid", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-dequeue-uuid", "hello", nil, react.Emitter, true)
	task.SetUserInputUUID("ui-uuid-dequeue-456")

	react.EmitDequeueReActTask(task, "normal")

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	require.Equal(t, "ui-uuid-dequeue-456", payload["react_task_user_input_uuid"])
}

func TestEmitDequeueReActTask_IncludesInputSource(t *testing.T) {
	var events []*schema.AiOutputEvent
	react := &ReAct{
		Emitter: aicommon.NewEmitter("react-dequeue-source", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events = append(events, e)
			return e, nil
		}),
		taskQueue: NewTaskQueue("test"),
	}
	task := aicommon.NewStatefulTaskBase("task-dequeue-source", "hello", nil, react.Emitter, true)
	task.SetInputSource(aicommon.USER_INPUT_SOURCE_SCHEDULE)

	react.EmitDequeueReActTask(task, "normal")

	require.Len(t, events, 1)
	payload := parsePayload(t, events[0].Content)
	require.Equal(t, aicommon.USER_INPUT_SOURCE_SCHEDULE, payload["react_task_input_source"])
}

func TestGetQueueInfoIncludesUserInputUUID(t *testing.T) {
	queue := NewTaskQueue("test")
	task := aicommon.NewStatefulTaskBase("task-queue-info", "follow up", nil, nil, true)
	task.SetUserInputUUID("ui-uuid-queue-info-789")
	require.NoError(t, queue.Append(task))

	react := &ReAct{taskQueue: queue}
	info := react.GetQueueInfo()
	tasks, ok := info["tasks"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, tasks, 1)
	require.Equal(t, "ui-uuid-queue-info-789", tasks[0]["user_input_uuid"])
}

func TestGetQueueInfoIncludesCurrentTask(t *testing.T) {
	currentTask := aicommon.NewStatefulTaskBase("task-current", "scheduled prompt", nil, nil, true)
	currentTask.SetStatus(aicommon.AITaskState_Processing)
	currentTask.SetInputSource(aicommon.USER_INPUT_SOURCE_SCHEDULE)
	currentTask.SetScheduleUUID("schedule-uuid")
	currentTask.SetScheduleName("daily check")

	react := &ReAct{
		taskQueue:   NewTaskQueue("test"),
		currentTask: currentTask,
		config:      &aicommon.Config{},
	}
	info := react.GetQueueInfo()
	current, ok := info["current_task"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "task-current", current["id"])
	require.Equal(t, aicommon.AITaskState_Processing, current["status"])
	require.Equal(t, aicommon.USER_INPUT_SOURCE_SCHEDULE, current["input_source"])
	require.Equal(t, "schedule-uuid", current["schedule_uuid"])
	require.Equal(t, "daily check", current["schedule_name"])
}

func TestGetQueueInfoCurrentTaskIsNilWhenIdle(t *testing.T) {
	react := &ReAct{taskQueue: NewTaskQueue("test")}
	info := react.GetQueueInfo()

	current, exists := info["current_task"]
	require.True(t, exists)
	require.Nil(t, current)
}
