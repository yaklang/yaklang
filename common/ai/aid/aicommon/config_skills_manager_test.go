package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestConfigSkillManagerAvailableWithoutLoop(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithSkillsFS(buildSkillsVFS()))
	before, err := MarshalTimeline(cfg.GetTimeline())
	require.NoError(t, err)
	mgr := cfg.GetSkillsContextManager()
	metas, err := mgr.ListSkills()
	require.NoError(t, err)
	require.Len(t, metas, 2)
	after, err := MarshalTimeline(cfg.GetTimeline())
	require.NoError(t, err)
	require.Equal(t, before, after, "getting a session manager must not load skill bodies")

	// 并发 session 查询拿到同一实例，不互相重建或重新展开正文。
	managers := make([]*aiskillloader.SkillsContextManager, 12)
	var wg sync.WaitGroup
	for i := range managers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			managers[index] = cfg.GetSkillsContextManager()
		}(i)
	}
	wg.Wait()
	for _, manager := range managers {
		require.Same(t, mgr, manager)
	}

	require.NoError(t, mgr.LoadSkill("deploy-app"))
	inventory := BuildBaseCapabilityInventoryPayload(cfg)
	require.Len(t, inventory.Fixed.Skills, 2)
	require.Equal(t, "code-review", inventory.Fixed.Skills[0].Name)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, inventory.Fixed.Skills[0].SkillLoadState)
	require.Equal(t, "deploy-app", inventory.Fixed.Skills[1].Name)
	require.Equal(t, CapabilityInventorySkillLoadLoaded, inventory.Fixed.Skills[1].SkillLoadState)
	require.Equal(t, mgr.GetCurrentSelectedSkills()[0].TokenCount, inventory.Fixed.Skills[1].TokenCount)
	require.True(t, mgr.UnloadSkill("deploy-app"))
	inventory = BuildBaseCapabilityInventoryPayload(cfg)
	require.Equal(t, CapabilityInventorySkillLoadMetadata, inventory.Fixed.Skills[1].SkillLoadState)
	require.Zero(t, inventory.Fixed.Skills[1].TokenCount)
}

func TestConfigSkillManagerChildTimelineIsolation(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAutoSkills(true), WithSkillsFS(buildSkillsVFS()))
	parentMgr := parent.GetSkillsContextManager()
	require.NoError(t, parentMgr.LoadSkill("deploy-app"))

	shared := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	sharedMgr := shared.GetSkillsContextManager()
	require.NotSame(t, parentMgr, sharedMgr)
	require.Same(t, parent.GetTimeline(), shared.GetTimeline())
	require.True(t, sharedMgr.IsSkillLoaded("deploy-app"))
	require.NoError(t, sharedMgr.LoadSkill("code-review"))
	require.True(t, parentMgr.IsSkillLoaded("code-review"))

	fork, err := parent.GetTimeline().ForkForTask("child", "isolated task", parent, parent)
	require.NoError(t, err)
	opts := append(ConvertConfigToOptions(parent), WithTimeline(fork.Branch))
	child := NewConfig(context.Background(), opts...)
	childMgr := child.GetSkillsContextManager()
	require.NotSame(t, parentMgr, childMgr)
	before, err := MarshalTimeline(parent.GetTimeline())
	require.NoError(t, err)
	require.NoError(t, childMgr.ChangeViewOffset("deploy-app", "", 3))
	require.True(t, childMgr.UnloadSkill("code-review"))
	require.False(t, childMgr.IsSkillLoaded("code-review"))
	require.True(t, parentMgr.IsSkillLoaded("code-review"))
	after, err := MarshalTimeline(parent.GetTimeline())
	require.NoError(t, err)
	require.Equal(t, before, after, "child operations must not rebind or modify the parent's manager/timeline")
}

func TestConfigSkillManagerRestoresWindowsFromSerializedTimeline(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("guide/SKILL.md", buildSkillMD("guide", "Guide", "Use references.md"))
	vfs.AddFile("guide/references.md", strings.Repeat("A long guide line that requires another page of context.\n", 1000))
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithSkillsFS(vfs))
	mgr := cfg.GetSkillsContextManager()
	require.NoError(t, mgr.LoadSkill("guide"))
	require.NoError(t, mgr.ChangeViewOffset("guide", "references.md", 100))
	cfg.GetTimeline().FreezeAll()
	require.NoError(t, mgr.ChangeViewOffset("guide", "references.md", 300))
	raw, err := MarshalTimeline(cfg.GetTimeline())
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	restoredCfg := NewConfig(context.Background(), WithDisableAutoSkills(true),
		WithSkillLoader(cfg.skillLoader), WithTimeline(restored), WithSequence(restored.GetMaxID()))
	before, err := MarshalTimeline(restored)
	require.NoError(t, err)
	restoredMgr := restoredCfg.GetSkillsContextManager()
	require.True(t, restoredMgr.HasTruncatedViews())
	_, err = restoredMgr.LoadSkillResource("guide", "references.md")
	require.NoError(t, err)
	after, err := MarshalTimeline(restored)
	require.NoError(t, err)
	require.Equal(t, before, after, "restoring windows must preserve both frozen and Open items byte-for-byte")
	require.NoError(t, restoredMgr.ChangeViewOffset("guide", "references.md", 600))
	summary := restoredMgr.GetSkillViewSummary("guide")
	for _, start := range []string{"100", "300", "600"} {
		require.Contains(t, summary, "references.md:"+start+"-")
	}
	require.NotContains(t, mgr.GetSkillViewSummary("guide"), "references.md:600-")
}
