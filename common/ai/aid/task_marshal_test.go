package aid

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestAiTask_MarshalJSON_IncludesTaskId(t *testing.T) {
	root := &AiTask{
		TaskId: "root-id",
		Index:  "1",
		Name:   "root",
		Goal:   "root goal",
		Subtasks: []*AiTask{
			{
				TaskId: "sub-id",
				Index:  "1-1",
				Name:   "sub",
				Goal:   "sub goal",
			},
		},
	}
	root.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("root-id", "root goal", nil, nil, true)
	root.Subtasks[0].AIStatefulTaskBase = aicommon.NewStatefulTaskBase("sub-id", "sub goal", nil, nil, true)

	raw, err := json.Marshal(root)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, "root-id", payload["task_id"])

	subtasks, ok := payload["subtasks"].([]any)
	require.True(t, ok)
	require.Len(t, subtasks, 1)

	sub, ok := subtasks[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "sub-id", sub["task_id"])
}
