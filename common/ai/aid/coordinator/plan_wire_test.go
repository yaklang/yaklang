package coordinator

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

func TestCoordinatorReviewedPlanAppliesRemovedSubtrees(t *testing.T) {
	data := `{"name":"Plan","goal":"Verify","subtasks":[{"task_id":"kept","name":"Keep","goal":"Execute approved task"},{"task_id":"removed","name":"Remove","goal":"Never execute","isRemove":true},{"name":"Group","goal":"Must not become a leaf task","subtasks":[{"name":"Child","goal":"Deleted","isRemove":true}]}]}`
	p, err := ParseReviewedPlan(data, "Approved document", nil)
	require.NoError(t, err)
	require.Len(t, p.Tasks, 1)
	require.Equal(t, "kept", p.Tasks[0].ID)
	require.Equal(t, "Approved document", p.Document)
	require.NotContains(t, string(p.Tree), "isRemove")
	require.NotContains(t, string(p.Tree), "Group")
}

func TestCoordinatorReviewedPlanRejectsInvalidApproval(t *testing.T) {
	for _, data := range []string{
		`null`, `{}`, `{"root_task":null}`,
		`{"name":"Plan","goal":"Verify","isRemove":true}`,
		`{"name":"Plan","goal":"Verify","subtasks":[null]}`,
		`{"name":"Plan","goal":"Verify","subtasks":[{"name":"Only task","goal":"Deleted","isRemove":true}]}`,
		`{"name":"Plan","goal":"Verify","subtasks":[{"task_id":"a","name":"A","goal":"Deleted prerequisite","isRemove":true},{"name":"B","goal":"Still depends on A","depends_on":["a"]}]}`,
	} {
		_, err := ParseReviewedPlan(data, "", nil)
		require.Error(t, err, data)
	}
}
