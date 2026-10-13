package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The observation/materials view must agree with what the model sees: only
// bodies that survive into the typed block are reported there, and Open bodies
// are reported in the Open tail instead.
func TestSkillContentMaterialsMatchModelView(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), "ordinary observation")
	tl.FreezeAll()
	before := RenderTimelineFrozenOpen(tl)

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "",
		"OBSERVED_SKILL_BODY", SkillContentSourceAuto))
	after := RenderTimelineFrozenOpen(tl)

	// Model view: Open carries the body; the cached prefix does not change.
	require.Contains(t, after.Open, "OBSERVED_SKILL_BODY")
	require.Equal(t, before.Frozen, after.Frozen)
	// Observation source of truth: promoted renderer must not report it yet.
	require.Empty(t, after.SkillsContext)

	// After a legal freeze commit the typed block becomes the single location.
	tl.FreezeAll()
	final := RenderTimelineFrozenOpen(tl)
	require.Contains(t, final.SkillsContext, "OBSERVED_SKILL_BODY")
	require.NotContains(t, final.Open, "OBSERVED_SKILL_BODY")
	// 模型视图 = Open 尾段 + 类型化技能块; 提升后正文恰好出现一次。
	modelView := final.Open + "\n" + final.SkillsContext
	require.Equal(t, 1, strings.Count(modelView, "OBSERVED_SKILL_BODY"),
		"body must be reported exactly once across the model view")
}
