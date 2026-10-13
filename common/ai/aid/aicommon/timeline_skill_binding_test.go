package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestSkillBindingPreservesRetiredTimelineAndRestoredHistory(t *testing.T) {
	for _, restore := range []bool{false, true} {
		name := "rebind"
		if restore {
			name = "restore"
		}
		t.Run(name, func(t *testing.T) {
			vfs := filesys.NewVirtualFs()
			vfs.AddFile("default/SKILL.md", "---\nname: default\ndescription: Default guide\nmetadata:\n  auto_load: true\n---\nRETIRED_DEFAULT_BODY")
			loader, err := aiskillloader.NewFSSkillLoader(vfs)
			require.NoError(t, err)
			mgr := aiskillloader.NewSkillsContextManager(loader)
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
			tl := cfg.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			bindTestTimeline(mgr, cfg)
			mgr.LoadDefaultSkills()
			require.True(t, mgr.IsSkillLoaded("default"))

			bindCompressionMock(t, tl, func(_ *AIRequest) (string, error) {
				return compressionMockSummary("Default guidance summarized."), nil
			})
			options := compressionTestOptions()
			options.FinalizeMemory = true
			_, err = tl.CompressOnce(options)
			require.NoError(t, err)
			require.False(t, mgr.IsSkillLoaded("default"))
			require.Empty(t, mgr.GetCurrentSelectedSkills())

			if restore {
				raw, err := MarshalTimeline(tl)
				require.NoError(t, err)
				tl, err = UnmarshalTimeline(raw)
				require.NoError(t, err)
				mgr = aiskillloader.NewSkillsContextManager(loader)
			}
			before, err := MarshalTimeline(tl)
			require.NoError(t, err)
			beforeView := RenderTimelineFrozenOpen(tl)
			nextID := tl.GetMaxID()
			mgr.BindTimelineSink(NewSkillContentTimelineAdapter(tl, func() int64 {
				nextID++
				return nextID
			}))
			if !restore {
				mgr.LoadDefaultSkills()
			}
			after, err := MarshalTimeline(tl)
			require.NoError(t, err)
			require.Equal(t, before, after, "binding and default initialization must not change existing history")
			require.Equal(t, beforeView, RenderTimelineFrozenOpen(tl))
			if !restore {
				require.True(t, mgr.HasSkillContentHistory("default"))
			}
			require.False(t, mgr.IsSkillLoaded("default"))
			require.Empty(t, mgr.GetCurrentSelectedSkills())

			added, err := mgr.LoadAutoSkill("default")
			require.NoError(t, err)
			require.True(t, added)
			require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "RETIRED_DEFAULT_BODY")
			require.Empty(t, mgr.RenderStable(), "explicit reload must still wait for freezing")
		})
	}
}

func TestSkillBindingViewsTargetWithoutChangingEitherTimeline(t *testing.T) {
	loader, err := aiskillloader.NewFSSkillLoader(buildSkillsVFS())
	require.NoError(t, err)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	original := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, original)
	require.NoError(t, mgr.LoadSkill("deploy-app"))
	original.GetTimeline().FreezeAll()

	target := NewConfig(context.Background(), WithDisableAutoSkills(true))
	target.GetTimeline().SetTimelineBucketByteSize(-1)
	require.True(t, target.GetTimeline().PushSkillContent(target.AcquireId(), "code-review",
		"SKILL.md", "TARGET_FROZEN_BODY", SkillContentSourceAuto))
	target.GetTimeline().FreezeAll()
	require.True(t, target.GetTimeline().PushSkillContent(target.AcquireId(), "target-only",
		"SKILL.md", "TARGET_OPEN_BODY", SkillContentSourceManual))
	require.True(t, target.GetTimeline().PushSkillCatalog(target.AcquireId(), "TARGET_CATALOG"))
	beforeOriginal, err := MarshalTimeline(original.GetTimeline())
	require.NoError(t, err)
	beforeTarget, err := MarshalTimeline(target.GetTimeline())
	require.NoError(t, err)

	bindTestTimeline(mgr, target)
	require.False(t, mgr.IsSkillLoaded("deploy-app"))
	require.True(t, mgr.IsSkillLoaded("code-review"))
	require.True(t, mgr.IsSkillLoaded("target-only"))
	selected := mgr.GetCurrentSelectedSkills()
	require.Len(t, selected, 2, "loaded names must include target items, not cached skills or catalogs")
	require.Equal(t, "code-review", selected[0].Name)
	require.Equal(t, "target-only", selected[1].Name)
	require.Equal(t, target.GetTimeline().RenderSkillContentForPrompt(), mgr.RenderStable())
	require.Contains(t, mgr.RenderStable(), "TARGET_FROZEN_BODY")
	require.NotContains(t, mgr.RenderStable(), "TARGET_OPEN_BODY")
	afterOriginal, err := MarshalTimeline(original.GetTimeline())
	require.NoError(t, err)
	afterTarget, err := MarshalTimeline(target.GetTimeline())
	require.NoError(t, err)
	require.Equal(t, beforeOriginal, afterOriginal)
	require.Equal(t, beforeTarget, afterTarget)
}
