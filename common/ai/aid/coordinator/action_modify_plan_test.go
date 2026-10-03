package coordinator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionModifyPlan(t *testing.T) {
	f := newActionFixture(t, true)
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(nestedPresetPlan), &plan))
	args := map[string]any{"plan_version": 2, "plan": plan, "plan_document": "replacement"}
	f.invoke("modify_plan", args, true)
	require.EqualValues(t, 1, f.c.Snapshot().DraftVersion)
	args["plan_version"] = 1
	f.invoke("modify_plan", args, false)
	require.EqualValues(t, 2, f.c.Snapshot().DraftVersion)
	require.EqualValues(t, 1, f.c.Snapshot().ApprovedVersion)
}
