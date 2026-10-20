package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
)

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
	require.True(t, target.GetTimeline().PushSkillFileContent(target.AcquireId(), "code-review", "SKILL.md", "TARGET_FROZEN_BODY", SkillContentSourceAuto, 1, 1, 1))
	target.GetTimeline().FreezeAll()
	require.True(t, target.GetTimeline().PushSkillFileContent(target.AcquireId(), "target-only", "SKILL.md", "TARGET_OPEN_BODY", SkillContentSourceManual, 1, 1, 1))
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
