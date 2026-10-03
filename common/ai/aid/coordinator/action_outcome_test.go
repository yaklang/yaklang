package coordinator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCoordinatorActionOutcomeLifecycle(t *testing.T) {
	f := newActionFixture(t, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, f.c, "a")
	open := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Contains(t, open.TimelineOpen, "A verified")
	require.Empty(t, open.SessionEvidenceSemiDynamic)
	f.cfg.Timeline.FreezeAll()
	sealed := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Contains(t, sealed.SessionEvidenceSemiDynamic, "A verified")
	f.invoke("review_task", map[string]any{"task_id": "a", "attempt_id": a.ID, "decision": "accept", "reason": "source e1 verified"}, false)
	_, err = f.c.StartTasks([]string{"b"})
	require.NoError(t, err)
	awaitResult(t, f.c, "b")
	pending := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Equal(t, sealed.SessionEvidenceSemiDynamic, pending.SessionEvidenceSemiDynamic, "new outcomes stay open until freezing")
	f.cfg.Timeline.FreezeAll()
	promoted := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Contains(t, promoted.SessionEvidenceSemiDynamic, "A verified")
	require.Contains(t, promoted.SessionEvidenceSemiDynamic, "B verified")
	require.Contains(t, promoted.SessionEvidenceSemiDynamic, "source e1 verified")
	raw, err := aicommon.MarshalTimeline(f.cfg.Timeline)
	require.NoError(t, err)
	restored, err := aicommon.UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, aicommon.RenderTimelineFrozenOpen(f.cfg.Timeline), aicommon.RenderTimelineFrozenOpen(restored))
	if root := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR"); root != "" {
		dir := filepath.Join(root, "action-observations")
		require.NoError(t, os.MkdirAll(dir, 0700))
		for name, material := range map[string]any{"01-open": open, "02-promoted": sealed, "03-next-action-open": pending, "04-both-results-promoted": promoted} {
			data, err := json.MarshalIndent(material, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), data, 0600))
		}
	}
}

type discardEvidenceConfig struct{ aicommon.AICallerConfigIf }

func (c discardEvidenceConfig) ApplySessionEvidenceOps([]aicommon.EvidenceOperation) {}
func TestCoordinatorActionOutcomeCannotReportUnpersistedSuccess(t *testing.T) {
	f := newActionFixture(t, true)
	loop := reactloops.NewMinimalReActLoop(discardEvidenceConfig{f.cfg}, f.loop.GetInvoker())
	WithController(f.c)(loop)
	op := reactloops.NewActionHandlerOperator(f.task)
	require.False(t, recordActionOutcome(loop, actionForTest(t, "submit_plan", nil), op, "submit_plan", PlanEditReceipt{Status: "updated"}, nil))
	done, err := op.IsTerminated()
	require.True(t, done)
	require.ErrorContains(t, err, "保存失败")
	require.Empty(t, op.GetFeedback().String())
}
