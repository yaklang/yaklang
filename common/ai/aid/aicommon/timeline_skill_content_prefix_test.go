package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Loading a skill must not disturb the cached prefix. Only the Open tail may
// change until an existing freeze/compression commit promotes the body.
func TestSkillContentLoadingKeepsCachedPrefixStable(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), "ordinary observation")
	// Freeze the earlier history so the new skill cannot ride along with it.
	tl.FreezeAll()
	before := RenderTimelineFrozenOpen(tl)

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "demo-skill", "", "PREFIX_SAFE_SECRET", SkillContentSourceAuto))
	after := RenderTimelineFrozenOpen(tl)

	require.Equal(t, before.Frozen, after.Frozen)
	require.Equal(t, before.EvidenceSemiDynamic, after.EvidenceSemiDynamic)
	require.NotContains(t, after.Frozen, "PREFIX_SAFE_SECRET")
	require.Contains(t, after.Open, "PREFIX_SAFE_SECRET")

	// Repeated rendering must be byte-stable and must not append anything.
	idsBefore := len(tl.idToTimelineItem.Keys())
	again := RenderTimelineFrozenOpen(tl)
	require.Equal(t, after.Open, again.Open)
	require.Equal(t, idsBefore, len(tl.idToTimelineItem.Keys()))
}

// Even a user-forced skill body must pass through the same freeze
// boundary before it may enter the cached prefix.
func TestForcedSkillContentWaitsForFreezeBoundary(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), "ordinary observation")
	tl.FreezeAll()
	before := RenderTimelineFrozenOpen(tl)

	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "forced-skill", "", "FORCED_BODY_SECRET", SkillContentSourceForced))
	after := RenderTimelineFrozenOpen(tl)

	require.Equal(t, before.Frozen, after.Frozen)
	require.NotContains(t, after.Frozen, "FORCED_BODY_SECRET")
	require.Contains(t, after.Open, "FORCED_BODY_SECRET")
}
