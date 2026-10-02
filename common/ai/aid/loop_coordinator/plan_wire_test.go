package loop_coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorPlanWireRetainsIDsAndExpandsGroupDependencies(t *testing.T) {
	data := `{"task_id":"root","name":"Plan","goal":"Verify","semantic_identifier":"plan","subtasks":[{"task_id":"group","name":"Sources","goal":"Read both","semantic_identifier":"sources","subtasks":[{"task_id":"a","name":"First","goal":"Read A","semantic_identifier":"first"},{"task_id":"b","name":"Second","goal":"Read B","semantic_identifier":"second"}]},{"task_id":"result","name":"Result","goal":"Check sources","semantic_identifier":"result","depends_on":["sources"]}]}`
	plan, err := ParsePlan(data, "Document", nil)
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 3)
	require.Equal(t, []string{"a", "b"}, plan.Tasks[2].DependsOn)
	changed := `{"name":"Plan","goal":"Verify","tasks":[{"name":"Result","goal":"Review again","identifier":"result","depends_on":[]}]}`
	updated, err := ParsePlan(changed, "New document", plan)
	require.NoError(t, err)
	require.Equal(t, "result", updated.Tasks[0].ID)
}

func TestCoordinatorPlanWireRejectsInvalidTasks(t *testing.T) {
	for _, data := range []string{`{"name":"Plan","goal":"Check","tasks":[]}`, `{"name":"Plan","goal":"Check","tasks":[{"name":"A","goal":"A","identifier":"a","depends_on":["missing"]}]}`, `{"name":"Plan","goal":"Check","tasks":[{"name":"A","goal":"A","identifier":"a","depends_on":["a"]}]}`, `{"name":"Plan","goal":"Check","tasks":[{"name":"A","goal":"A","identifier":"same"},{"name":"B","goal":"B","identifier":"same"}]}`, `{"name":"Plan","goal":"Check","subtasks":[null]}`} {
		_, err := ParsePlan(data, "", nil)
		require.Error(t, err, data)
	}
}
