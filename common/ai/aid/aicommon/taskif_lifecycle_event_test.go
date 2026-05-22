package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAIStatefulTaskBase_EmitsLifecycleEventsByDefault(t *testing.T) {
	var events []*schema.AiOutputEvent
	emitter := NewEmitter("task-lifecycle-default", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, e)
		return e, nil
	})

	task := NewStatefulTaskBase("task-1", "user input", nil, emitter)
	task.SetStatus(AITaskState_Processing)

	require.Equal(t, []string{"react_task_created", "react_task_status_changed"}, taskLifecycleNodeIDs(events))
	require.Equal(t, task.GetUUID(), events[0].TaskUUID)
	require.Equal(t, task.GetUUID(), events[1].TaskUUID)

	createdPayload := requireStructuredPayload(t, events[0])
	require.Equal(t, string(AITaskState_Created), createdPayload["react_task_status"])
	require.Equal(t, "user input", createdPayload["react_user_input"])
	require.Equal(t, "task-1", createdPayload["react_task_id"])
	require.Equal(t, task.GetUUID(), createdPayload["react_task_uuid"])

	statusPayload := requireStructuredPayload(t, events[1])
	require.Equal(t, "task-1", statusPayload["react_task_id"])
	require.Equal(t, string(AITaskState_Created), statusPayload["react_task_old_status"])
	require.Equal(t, string(AITaskState_Processing), statusPayload["react_task_now_status"])
}

func TestAIStatefulTaskBase_SkipLifecycleEventsKeepsEmitterUsable(t *testing.T) {
	var events []*schema.AiOutputEvent
	emitter := NewEmitter("task-lifecycle-skip", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, e)
		return e, nil
	})

	task := NewStatefulTaskBase("task-skip", "user input", nil, emitter, true)
	task.SetStatus(AITaskState_Processing)
	require.Empty(t, events)

	_, err := task.GetEmitter().EmitStructured("custom_task_event", map[string]any{"ok": true})
	require.NoError(t, err)
	require.Equal(t, []string{"custom_task_event"}, taskLifecycleNodeIDs(events))
	require.Equal(t, task.GetUUID(), events[0].TaskUUID)
}

func TestNewSubTaskBase_SkipLifecycleEvents(t *testing.T) {
	var events []*schema.AiOutputEvent
	emitter := NewEmitter("subtask-lifecycle-skip", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, e)
		return e, nil
	})

	parent := NewStatefulTaskBase("parent-task", "parent input", nil, emitter, true)
	subTask := NewSubTaskBase(parent, "sub-task", "sub input", true)
	subTask.SetStatus(AITaskState_Processing)
	require.Empty(t, events)

	_, err := subTask.GetEmitter().EmitStructured("subtask_custom_event", map[string]any{"ok": true})
	require.NoError(t, err)
	require.Equal(t, []string{"subtask_custom_event"}, taskLifecycleNodeIDs(events))
	require.NotEmpty(t, events[0].TaskUUID)
}

func taskLifecycleNodeIDs(events []*schema.AiOutputEvent) []string {
	nodeIDs := make([]string, 0, len(events))
	for _, event := range events {
		nodeIDs = append(nodeIDs, event.NodeId)
	}
	return nodeIDs
}

func requireStructuredPayload(t *testing.T, event *schema.AiOutputEvent) map[string]any {
	t.Helper()

	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, event.Type)
	require.True(t, event.IsJson)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Content, &payload))
	return payload
}
