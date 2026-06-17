package aid

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestPlanTaskPayload_IncludesTaskId(t *testing.T) {
	task := &AiTask{
		TaskId: "stable-task-id",
		Index:  "1-1",
		Name:   "demo",
		Goal:   "demo goal",
	}
	task.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("other-id", "demo goal", nil, nil, true)

	payload := planTaskPayload(task)
	require.Equal(t, "stable-task-id", payload["task_id"])
	require.NotEmpty(t, payload["task_uuid"])
	require.Equal(t, "1-1", payload["index"])
}

func TestPlanTaskID_FallsBackToStatefulId(t *testing.T) {
	task := &AiTask{
		Index: "1-2",
		Name:  "demo",
		Goal:  "demo goal",
	}
	task.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("fallback-id", "demo goal", nil, nil, true)

	require.Equal(t, "fallback-id", planTaskID(task))
}
