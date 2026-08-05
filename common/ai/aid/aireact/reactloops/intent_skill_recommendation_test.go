package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func newIntentSkillRecommendationTestLoop(t *testing.T) (*ReActLoop, *mockcfg.MockInvoker, *aiskillloader.SkillsContextManager) {
	t.Helper()

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("active-skill/SKILL.md", "---\nname: active-skill\ndescription: already active\n---\n# Active")
	vfs.AddFile("candidate-skill/SKILL.md", "---\nname: candidate-skill\ndescription: should be recommended\n---\n# Candidate")
	loader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(vfs))
	require.NoError(t, err)

	mgr := aiskillloader.NewSkillsContextManager(loader)
	added, err := mgr.LoadForcedSkill("active-skill")
	require.NoError(t, err)
	require.True(t, added)

	cfg := aicommon.NewConfig(context.Background(), aicommon.WithSkillLoader(loader))
	invoker := mockcfg.NewMockInvoker(context.Background())
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	loop.extraCapabilities = NewExtraCapabilitiesManager()
	loop.skillsContextManager = mgr
	return loop, invoker, mgr
}

// 目的：验证深度意图识别不会再次推荐显式加载的 Skill；预期仅保留未加载 Skill。
func TestDeepIntentSkillRecommendationSkipsLoadedSkill(t *testing.T) {
	loop, invoker, mgr := newIntentSkillRecommendationTestLoop(t)

	PopulateExtraCapabilitiesFromDeepIntent(invoker, loop, &DeepIntentResult{
		MatchedSkillNames: "active-skill,candidate-skill",
	})

	recommended := loop.GetExtraCapabilities().ListSkills()
	require.Len(t, recommended, 1)
	require.Equal(t, "candidate-skill", recommended[0].Name)
	catalog := mgr.GetCatalogSkills()
	require.Len(t, catalog, 1)
	require.Equal(t, "candidate-skill", catalog[0].Name)
}

// 目的：验证能力搜索同样过滤已加载 Skill；预期未加载 Skill 仍可正常推荐。
func TestCapabilitySearchSkillRecommendationSkipsLoadedSkill(t *testing.T) {
	loop, invoker, _ := newIntentSkillRecommendationTestLoop(t)

	PopulateExtraCapabilitiesFromCapabilitySearchResult(invoker, loop, &CapabilitySearchResult{
		MatchedSkillNames: []string{"active-skill", "candidate-skill"},
	})

	recommended := loop.GetExtraCapabilities().ListSkills()
	require.Len(t, recommended, 1)
	require.Equal(t, "candidate-skill", recommended[0].Name)
}
