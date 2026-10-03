package coordinator

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatorActionModifyPlan(t *testing.T) {
	f := newActionFixture(t, false)
	_, err := f.c.CreatePlan(context.Background(), "plan", "old")
	require.NoError(t, err)
	f.invoke("modify_plan", map[string]any{"document": "replacement"}, false)
	require.Equal(t, "replacement", f.c.Snapshot().Plan.Document)
	require.Equal(t, PhasePlan, f.c.Snapshot().Phase)
	before := f.c.Snapshot()
	f.invoke("modify_plan", map[string]any{"document": " "}, true)
	require.Equal(t, before, f.c.Snapshot())
}
