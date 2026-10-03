package coordinator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionCreatePlan(t *testing.T) {
	f := newActionFixture(t, false)
	var plan map[string]any
	require.NoError(t, json.Unmarshal([]byte(nestedPresetPlan), &plan))
	f.invoke("create_plan", map[string]any{"plan": plan, "plan_document": "large static document"}, false)
	require.EqualValues(t, 1, f.c.Snapshot().DraftVersion)
	require.Nil(t, f.c.Snapshot().Approved)
	require.NotContains(t, f.cfg.GetSessionEvidenceRendered(), "large static document")
}
