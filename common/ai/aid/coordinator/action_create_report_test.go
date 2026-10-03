package coordinator

import (
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func completedActionFixture(t *testing.T) *actionFixture {
	f := newActionFixture(t, true)
	for _, id := range []string{"a", "b"} {
		_, err := f.c.StartTasks([]string{id})
		require.NoError(t, err)
		a := awaitResult(t, f.c, id)
		require.NoError(t, f.c.ReviewTask(id, a.ID, "accept", "actual evidence checked"))
	}
	through, err := f.c.deliverMessages()
	require.NoError(t, err)
	f.c.checkedMessages(through)
	f.loop.Set("coordinator_observed_user_revision", f.c.Snapshot().UserRevision)
	f.loop.Set("coordinator_message_cursor", through)
	return f
}

func TestCoordinatorActionCreateReport(t *testing.T) {
	f := newActionFixture(t, true)
	f.invoke("create_report", map[string]any{"title": "报告", "document": "too early"}, true)
	require.Empty(t, f.c.Snapshot().Report.Path)
	f = completedActionFixture(t)
	f.invoke("create_report", map[string]any{"title": "报告", "document": "# Report\nverified\n"}, false)
	data, err := os.ReadFile(f.c.Snapshot().Report.Path)
	require.NoError(t, err)
	require.Equal(t, "# Report\nverified\n", string(data))
	f.invoke("create_report", map[string]any{"title": "duplicate", "document": "replacement"}, true)
	require.False(t, f.c.Snapshot().Finished)
}
