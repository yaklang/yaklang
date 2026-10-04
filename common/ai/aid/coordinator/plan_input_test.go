package coordinator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractForgePresetPlanPreservesNestedGroupDAG(t *testing.T) {
	const plan = `{"main_task":"Plan","main_task_goal":"Verify","tasks":[{"subtask_name":"Sources","subtask_goal":"Read","subtask_identifier":"sources","tasks":[{"subtask_name":"A","subtask_goal":"Read A","subtask_identifier":"a"},{"subtask_name":"B","subtask_goal":"Read B","subtask_identifier":"b"}]},{"subtask_name":"Result","subtask_goal":"Compare","subtask_identifier":"result","depends_on":["sources"]}]}`
	for _, input := range []string{plan, `{"@action":"plan",` + plan[1:], "```json\n" + plan + "\n```"} {
		response, err := ExtractPlan(nil, input)
		require.NoError(t, err)
		require.Contains(t, response.Document, "前置任务")
		data, err := json.Marshal(response.RootTask)
		require.NoError(t, err)
		parsed, err := ParsePlan(string(data), response.Document, nil)
		require.NoError(t, err)
		require.Len(t, parsed.Tasks, 3)
		require.ElementsMatch(t, []string{parsed.Tasks[0].ID, parsed.Tasks[1].ID}, parsed.Tasks[2].DependsOn)
	}
}
