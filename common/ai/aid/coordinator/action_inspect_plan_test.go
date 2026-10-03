package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestCoordinatorActionInspectPlanPromotionAndDedup(t *testing.T) {
	f := newActionFixture(t, true)
	before := f.c.Snapshot()
	f.invoke("inspect_plan", nil, false)
	m := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Contains(t, m.TimelineOpen, `"approved_version":1`)
	require.Empty(t, m.SessionEvidenceSemiDynamic)
	id := f.cfg.Timeline.GetMaxID()
	f.invoke("inspect_plan", nil, false)
	require.Equal(t, id, f.cfg.Timeline.GetMaxID())
	f.cfg.Timeline.FreezeAll()
	m = aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	require.Empty(t, m.TimelineOpen)
	require.Contains(t, m.SessionEvidenceSemiDynamic, `"action":"inspect_plan"`)
	for i := 0; i < 3; i++ {
		f.invoke("inspect_plan", nil, false)
		require.Equal(t, m, aicommon.BuildPromptFrozenOpenMaterials(f.cfg))
	}
	require.Equal(t, before, f.c.Snapshot())
}
