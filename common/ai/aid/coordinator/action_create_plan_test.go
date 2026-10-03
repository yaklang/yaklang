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
	require.Equal(t, PhasePlan, f.c.Snapshot().Phase)
	require.NotNil(t, f.c.Snapshot().Plan)
	require.NotContains(t, f.cfg.GetSessionEvidenceRendered(), "large static document")
}
