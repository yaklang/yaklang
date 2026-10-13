package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Save -> restore must preserve skill bodies, their freeze position and the
// promoted/typed view. An Open body must not jump into the cached block on load.
func TestSkillContentSaveRestoreKeepsFreezePosition(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)

	tl.PushText(cfg.AcquireId(), "ordinary observation")
	tl.FreezeAll()
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "open-skill", "SKILL.md",
		"OPEN_BODY_AFTER_RESTORE", SkillContentSourceAuto))
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "promoted-skill", "SKILL.md",
		"PROMOTED_BODY_AFTER_RESTORE", SkillContentSourceAuto))
	// Promote only what is present up to now.
	tl.FreezeAll()

	beforeTyped := tl.RenderSkillContentForPrompt()
	require.Contains(t, beforeTyped, "PROMOTED_BODY_AFTER_RESTORE")

	// A later body stays in Open across save/restore.
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "later-skill", "SKILL.md",
		"STILL_OPEN_BODY", SkillContentSourceAuto))
	openBefore := RenderTimelineFrozenOpen(tl)
	require.Contains(t, openBefore.Open, "STILL_OPEN_BODY")
	require.NotContains(t, openBefore.Frozen, "STILL_OPEN_BODY")

	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)

	after := RenderTimelineFrozenOpen(restored)
	require.Equal(t, openBefore.Frozen, after.Frozen)
	require.Equal(t, openBefore.Open, after.Open)
	require.Equal(t, beforeTyped, restored.RenderSkillContentForPrompt())
	require.Contains(t, restored.RenderSkillContentForPrompt(), "PROMOTED_BODY_AFTER_RESTORE")
	require.NotContains(t, restored.RenderSkillContentForPrompt(), "STILL_OPEN_BODY")
}

// A sub/branch timeline must not leak its private skill bodies back to the parent.
func TestSkillContentBranchIsolation(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	tl := cfg.GetTimeline()
	tl.SetTimelineBucketByteSize(-1)
	tl.PushText(cfg.AcquireId(), "parent observation")
	require.True(t, tl.PushSkillContent(cfg.AcquireId(), "parent-skill", "SKILL.md",
		"PARENT_BODY", SkillContentSourceAuto))

	child := tl.CreateSubTimeline(0)
	if child == nil || child == tl {
		t.Skip("sub timeline not supported in this configuration")
	}
	require.True(t, child.PushSkillContent(cfg.AcquireId(), "child-skill", "SKILL.md",
		"CHILD_PRIVATE_BODY", SkillContentSourceAuto))

	require.Contains(t, RenderTimelineFrozenOpen(child).Open, "CHILD_PRIVATE_BODY")
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Open, "CHILD_PRIVATE_BODY")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "PARENT_BODY")
}
