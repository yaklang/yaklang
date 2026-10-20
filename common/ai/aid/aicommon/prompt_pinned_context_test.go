package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedContextProjectionKeepsResourcesAndJournal(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true))
		tl := cfg.Timeline
		tl.SetTimelineBucketByteSize(-1)
		require.True(t, tl.PushSkillMetadata(cfg.AcquireId(), "guide", "DUPLICATE_METADATA", SkillContentSourceAuto))
		require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "guide", "SKILL.md", "DUPLICATE_ROOT", SkillContentSourceAuto, 1, 1, 1))
		require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "guide", "reference.md", "KEEP_RESOURCE", SkillContentSourceManual, 1, 1, 1))
		require.True(t, tl.PushPromotable(cfg.AcquireId(), TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "query", TimelinePromotedOperationUpsert, "DUPLICATE_SCHEMA"))
		tl.PushText(cfg.AcquireId(), "KEEP_TOOL_RESULT")
		if frozen {
			tl.FreezeAll()
		}
		before := RenderTimelineFrozenOpen(tl).PromptText()
		view := RenderTimelineFrozenOpenWithOptions(tl, TimelinePromptOptions{PinnedSkillNames: []string{"guide"}, PinnedToolNames: []string{"query"}}).PromptText()
		for _, duplicate := range []string{"DUPLICATE_METADATA", "DUPLICATE_ROOT", "DUPLICATE_SCHEMA"} {
			require.Contains(t, before, duplicate)
			require.NotContains(t, view, duplicate)
		}
		require.Contains(t, view, "KEEP_RESOURCE")
		require.Contains(t, view, "KEEP_TOOL_RESULT")
		require.Equal(t, before, RenderTimelineFrozenOpen(tl).PromptText(), "focus projection must be read-only")
	}
}
