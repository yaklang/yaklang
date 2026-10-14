package aiskillloader

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type bindingTestLoader struct {
	SkillLoader
	loads int
}

func (l *bindingTestLoader) LoadSkill(name string) (*LoadedSkill, error) {
	l.loads++
	return l.SkillLoader.LoadSkill(name)
}

func TestDefaultSkillLoadingIsExplicitAndDoesNotReplayHistory(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("default/SKILL.md", "---\nname: default\ndescription: Default guide\nmetadata:\n  auto_load: true\n---\nDEFAULT_BODY")
	base, err := NewFSSkillLoader(vfs)
	require.NoError(t, err)
	loader := &bindingTestLoader{SkillLoader: base}
	mgr := NewSkillsContextManager(loader)
	require.Zero(t, loader.loads, "construction must not load skill content")
	require.Empty(t, mgr.GetCurrentSelectedSkills())

	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)
	require.Zero(t, loader.loads, "binding must not load skill content")
	require.Zero(t, sink.appendCount())
	mgr.LoadDefaultSkills()
	require.Equal(t, 1, loader.loads)
	require.True(t, mgr.IsSkillLoaded("default"))
	appends := sink.appendCount()
	require.Positive(t, appends)

	require.True(t, sink.DeleteSkillContent("default"))
	mgr.BindTimelineSink(sink)
	mgr.LoadDefaultSkills()
	require.Equal(t, 1, loader.loads)
	require.Equal(t, appends, sink.appendCount(), "binding and initialization must respect explicit removal")
	require.False(t, mgr.IsSkillLoaded("default"))
	require.Empty(t, mgr.GetCurrentSelectedSkills())

	added, err := mgr.LoadAutoSkill("default")
	require.NoError(t, err)
	require.True(t, added, "an explicit load may reread retired content")
	require.Greater(t, sink.appendCount(), appends)
}

func TestUnboundSkillStateDoesNotBackfillTimeline(t *testing.T) {
	base, err := NewFSSkillLoader(buildTestVFS())
	require.NoError(t, err)
	loader := &bindingTestLoader{SkillLoader: base}
	mgr := NewSkillsContextManager(loader)
	added, err := mgr.LoadForcedSkill("deploy-app")
	require.NoError(t, err)
	require.False(t, added, "unbound loading has no timeline item to append")
	require.Equal(t, 1, loader.loads)
	require.Equal(t, 1, mgr.states.Len())

	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)
	require.Zero(t, sink.appendCount(), "binding must not backfill unbound local state")
	mgr.LoadDefaultSkills()
	require.Zero(t, sink.appendCount(), "initialization must not copy unbound state")
	added, err = mgr.LoadForcedSkill("deploy-app")
	require.NoError(t, err)
	require.True(t, added)
	require.Greater(t, sink.appendCount(), 0, "an explicit load after binding writes the target timeline")
}

func TestSkillBindingQueriesOnlyTheTargetTimeline(t *testing.T) {
	base, err := NewFSSkillLoader(buildTestVFS())
	require.NoError(t, err)
	loader := &bindingTestLoader{SkillLoader: base}
	mgr := NewSkillsContextManager(loader)
	first := newTestTimelineSink()
	mgr.BindTimelineSink(first)
	require.NoError(t, mgr.LoadSkill("deploy-app"))
	firstAppends, loads := first.appendCount(), loader.loads

	target := newTestTimelineSink()
	require.True(t, target.AppendSkillFileContent("code-review", "SKILL.md", "TARGET_BODY", SkillContentSourceAuto, 1, 1, 1))
	mgr.BindTimelineSink(target)
	require.Equal(t, 1, target.appendCount(), "binding must not copy cached skills into the target")
	require.Equal(t, firstAppends, first.appendCount())
	require.Equal(t, loads, loader.loads)
	require.False(t, mgr.IsSkillLoaded("deploy-app"))
	require.True(t, mgr.IsSkillLoaded("code-review"))
	selected := mgr.GetCurrentSelectedSkills()
	require.Len(t, selected, 1)
	require.Equal(t, "code-review", selected[0].Name)
	require.Equal(t, MeasureStringTokens("TARGET_BODY", nil), selected[0].TokenCount)
	require.Equal(t, target.RenderSkillContentForPrompt(), mgr.RenderStable())
	require.Equal(t, loads, loader.loads, "viewing the target must not load content")
}

func TestSkillManagerViewsTimelineTokenCounts(t *testing.T) {
	base, err := NewFSSkillLoader(buildTestVFS())
	require.NoError(t, err)
	loader := &bindingTestLoader{SkillLoader: base}
	mgr := NewSkillsContextManager(loader)
	require.Empty(t, mgr.GetCurrentSelectedSkills())
	require.Empty(t, mgr.GetSkillViewSummary("deploy-app"))
	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)
	inventory := mgr.SkillInventory()

	metadata, first, second := "Skill metadata", "First page body", "Second page body"
	require.True(t, sink.AppendSkillMetadata("deploy-app", metadata, SkillContentSourceManual))
	require.True(t, sink.AppendSkillFileContent("deploy-app", "SKILL.md", first, SkillContentSourceManual, 1, 4, 8))
	require.True(t, sink.AppendSkillCatalog("this catalog is not loaded skill content"))
	expected := MeasureStringTokens(metadata, nil) + MeasureStringTokens(first, nil)
	selected := mgr.GetCurrentSelectedSkills()
	require.Len(t, selected, 1)
	require.Equal(t, expected, selected[0].TokenCount)
	require.Same(t, base.skills["deploy-app"].meta, selected[0].SkillMeta, "runtime size is separate from loader metadata")

	require.True(t, sink.AppendSkillFileContent("deploy-app", "SKILL.md", second, SkillContentSourceAuto, 5, 8, 8))
	expected += MeasureStringTokens(second, nil)
	require.Equal(t, expected, mgr.GetCurrentSelectedSkills()[0].TokenCount)
	require.Equal(t, expected-MeasureStringTokens(second, nil), selected[0].TokenCount, "an earlier query result must not acquire later sizes")
	summary := mgr.GetSkillViewSummary("deploy-app")
	require.Contains(t, summary, fmt.Sprintf("Timeline content: %d tokens", expected))
	require.Contains(t, summary, fmt.Sprintf("Metadata: loaded (%d tokens)", MeasureStringTokens(metadata, nil)))
	require.Contains(t, summary, fmt.Sprintf("SKILL.md:5-8/8 END (%d tokens)", MeasureStringTokens(second, nil)))
	require.Zero(t, loader.loads, "observing skill sizes must not read skill files")
	require.Equal(t, inventory, mgr.SkillInventory(), "runtime sizes must not rewrite the fixed inventory")

	require.True(t, mgr.UnloadSkill("deploy-app"))
	require.Empty(t, mgr.GetCurrentSelectedSkills())
	require.Empty(t, mgr.GetSkillViewSummary("deploy-app"))
}
