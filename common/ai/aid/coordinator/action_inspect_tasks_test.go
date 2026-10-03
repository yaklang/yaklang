package coordinator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestCoordinatorActionInspectTasks(t *testing.T) {
	f := newActionFixture(t, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return f.c.Snapshot().Attempts["a"].State == AwaitingReview }, time.Second, time.Millisecond)
	require.False(t, f.c.Snapshot().Attempts["a"].Seen)
	f.invoke("inspect_tasks", map[string]any{"task_ids": []string{"a"}}, false)
	require.True(t, f.c.Snapshot().Attempts["a"].Seen)
	f.cfg.Timeline.FreezeAll()
	stable := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Contains(t, stable.SessionEvidenceSemiDynamic, "A verified")
	require.NotContains(t, stable.SessionEvidenceSemiDynamic, "verify A")
	f.invoke("inspect_plan", nil, false)
	require.Equal(t, stable.SessionEvidenceSemiDynamic, aicommon.BuildPromptFrozenOpenMaterials(f.cfg).SessionEvidenceSemiDynamic)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "A verified")
}
