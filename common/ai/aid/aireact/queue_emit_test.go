package aireact

import (
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
