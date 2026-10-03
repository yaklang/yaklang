package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionSubmitPlan(t *testing.T) {
	f := newActionFixture(t, false)
	v, err := f.c.CreatePlan(f.task.GetContext(), "plan", "doc")
	require.NoError(t, err)
	f.invoke("submit_plan", map[string]any{"plan_version": v + 1}, true)
	require.Nil(t, f.c.Snapshot().Approved)
	f.invoke("submit_plan", map[string]any{"plan_version": v}, false)
	before := f.c.Snapshot()
	id := f.cfg.Timeline.GetMaxID()
	f.invoke("submit_plan", map[string]any{"plan_version": v}, false)
	require.Equal(t, before, f.c.Snapshot())
	require.Equal(t, id, f.cfg.Timeline.GetMaxID())
}
