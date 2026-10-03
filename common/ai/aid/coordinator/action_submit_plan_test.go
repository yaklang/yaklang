package coordinator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatorActionSubmitPlan(t *testing.T) {
	f := newActionFixture(t, false)
	f.invoke("submit_plan", map[string]any{}, true)
	_, err := f.c.CreatePlan(f.task.GetContext(), "plan", "doc")
	require.NoError(t, err)
	f.invoke("submit_plan", map[string]any{}, false)
	require.Equal(t, PhaseExec, f.c.Snapshot().Phase)
	before := f.c.Snapshot()
	f.invoke("submit_plan", map[string]any{}, true)
	require.Equal(t, before, f.c.Snapshot())
}
