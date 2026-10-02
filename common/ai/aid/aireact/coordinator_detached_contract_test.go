package aireact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorDetachedPlanUsesYakitEditedTree(t *testing.T) {
	_, _, _, input, err := parseExecuteDetachedPlanParams(`{"coordinator_id":"plan-owner","plan_data":"old","plans":{"root_task":{"task_id":"root","name":"edited","goal":"approved edits","subtasks":[{"task_id":"leaf","name":"step","goal":"edited task brief","semantic_identifier":"step","depends_on":[]}]}}}`)
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(input.PlanData), &tree))
	require.Equal(t, "edited", tree["name"])
	require.Contains(t, input.PlanData, "edited task brief")
	_, _, _, _, err = parseExecuteDetachedPlanParams(`{"coordinator_id":"owner","plans":{"root_task":[]}}`)
	require.Error(t, err)
}
