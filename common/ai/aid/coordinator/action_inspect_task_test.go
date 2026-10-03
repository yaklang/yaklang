package coordinator

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestCoordinatorActionInspectTask(t *testing.T) {
	f := newActionFixture(t, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, f.c, "a")
	f.invoke("inspect_task", map[string]any{"task_id": "a"}, false)
	require.Equal(t, AwaitingReview, f.c.Snapshot().Attempts["a"].State)
	require.NoError(t, f.c.ReviewTask("a", a.ID, "reject", "needs independent verification"))
	_, err = f.c.RetryTask("a", a.ID, "verify deeper")
	require.NoError(t, err)
	awaitResult(t, f.c, "a")
	value, err := f.c.InspectTask("a", a.ID, false)
	require.NoError(t, err)
	require.Equal(t, true, value.(map[string]any)["historical"])
	require.Contains(t, value.(map[string]any)["result_reference"], "coordinator.task.")
	f.invoke("inspect_task", map[string]any{"task_id": "missing"}, true)
}

func TestCoordinatorActionInspectTaskBoundsDefaultAndPreservesExplicitDetails(t *testing.T) {
	f := newActionFixture(t, true)
	f.c.mu.Lock()
	a := f.c.state.Attempts["a"]
	a.State = AwaitingReview
	a.ID = 1
	a.Result = Result{Summary: strings.Repeat("事实", 1000), Error: strings.Repeat("失败原因", 1000)}
	a.ReviewReason = strings.Repeat("复核理由", 1000)
	for i := 0; i < 30; i++ {
		a.Result.Artifacts = append(a.Result.Artifacts, "artifact")
		a.Result.EvidenceIDs = append(a.Result.EvidenceIDs, "evidence")
	}
	f.c.state.Attempts["a"] = a
	f.c.mu.Unlock()
	value, err := f.c.InspectTask("a", 0, false)
	require.NoError(t, err)
	bounded := value.(map[string]any)
	r := bounded["result"].(Result)
	require.LessOrEqual(t, len([]rune(r.Summary)), 601)
	require.LessOrEqual(t, len([]rune(r.Error)), 601)
	require.Len(t, r.Artifacts, 16)
	require.Len(t, r.EvidenceIDs, 16)
	require.Equal(t, 30, bounded["artifact_count"])
	require.Contains(t, bounded["waiting_reason"], "审核")
	value, err = f.c.InspectTask("a", 0, true)
	require.NoError(t, err)
	require.Equal(t, a.Result, value.(map[string]any)["result"])
	require.Equal(t, AwaitingReview, f.c.Snapshot().Attempts["a"].State)
}
