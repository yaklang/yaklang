package coordinator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatorActionSubmitReportHostRechecksMessages(t *testing.T) {
	f := completedActionFixture(t)
	f.invoke("create_report", map[string]any{"title": "报告", "document": "# Report\nverified"}, false)
	f.c.Wake()
	f.invoke("submit_report", map[string]any{"summary": "stale"}, true)
	require.False(t, f.c.Snapshot().Report.Submitted)
	require.NotEmpty(t, f.c.Snapshot().Report.Document)
	through, err := f.c.deliverMessages()
	require.NoError(t, err)
	f.c.checkedMessages(through)
	f.loop.Set("coordinator_observed_user_revision", f.c.Snapshot().UserRevision)
	f.loop.Set("coordinator_message_cursor", through)
	f.invoke("submit_report", map[string]any{"summary": "needs revision"}, true)
	f.invoke("modify_report", map[string]any{"document": "# Report\nIncludes new requirements."}, false)
	f.invoke("submit_report", map[string]any{"summary": "current"}, false)
	require.False(t, f.c.Snapshot().Finished, "submission is not normal exit authority")
	f.c.Wake()
	require.Error(t, f.c.FinalizeReport())
	require.False(t, f.c.Snapshot().Finished)
}
