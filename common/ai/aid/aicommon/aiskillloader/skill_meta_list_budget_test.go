package aiskillloader

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectSkillMetasByTokenBudget_TruncatesRegistryList(t *testing.T) {
	metas := []*SkillMeta{
		{Name: "alpha", Description: strings.Repeat("x", 200)},
		{Name: "beta", Description: strings.Repeat("y", 200)},
		{Name: "gamma", Description: strings.Repeat("z", 200)},
	}

	estimator := func(s string) int { return len(s) }
	listed, omitted := SelectSkillMetasByTokenBudget(
		metas,
		MeasureStringTokens(AvailableSkillsRegistryHeader, estimator)+MeasureStringTokens(FormatAvailableSkillRegistryLine(metas[0]), estimator),
		estimator,
		AvailableSkillsRegistryHeader,
	)

	if len(listed) != 1 {
		t.Fatalf("expected 1 listed skill under tight budget, got %d", len(listed))
	}
	if omitted != 2 {
		t.Fatalf("expected 2 omitted skills, got %d", omitted)
	}
	if listed[0].Name != "alpha" {
		t.Fatalf("expected first sorted skill alpha, got %q", listed[0].Name)
	}
}

func TestSelectSkillMetasForPromptRegistry_MatchesAppendAvailableSkillsSection(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	mgr.BindTimelineSink(newTestTimelineSink())

	listed, omitted := SelectSkillMetasForPromptRegistry(loader.AllSkillMetas(), mgr.TokenEstimator())
	if len(listed) != 2 || omitted != 0 {
		t.Fatalf("expected full registry list, got listed=%d omitted=%d", len(listed), omitted)
	}

	// 运行时推荐目录由 SetCatalogSkills 写入，与固定 Skill Inventory 无关。
	mgr.SetCatalogSkills(loader.AllSkillMetas())
	rendered := mgr.RenderStable()
	if !strings.Contains(rendered, "  - code-review:") || !strings.Contains(rendered, "  - deploy-app:") {
		t.Fatalf("rendered prompt should list both skills in catalog: %s", rendered)
	}
}

func TestSkillsContextManagerSkillInventoryIsFixedMetadataOnly(t *testing.T) {
	base, err := NewFSSkillLoader(buildTestVFS())
	require.NoError(t, err)
	base.skills["disabled"] = &skillEntry{meta: &SkillMeta{
		Name: "disabled", Description: "do not suggest", DisableModelInvocation: true,
	}}
	base.skills["empty"] = &skillEntry{meta: &SkillMeta{Description: "missing name"}}
	base.skills["code-review"].meta.Description = "first\n  second " + strings.Repeat("技能", 100)

	loader := &bindingTestLoader{SkillLoader: base}
	mgr := NewSkillsContextManager(loader)
	inventory := mgr.SkillInventory()
	require.Zero(t, loader.loads, "building the directory must not load skill files")
	require.Contains(t, inventory, "# Skill Inventory")
	require.Contains(t, inventory, "first second ")
	require.Contains(t, inventory, "…")
	require.NotContains(t, inventory, strings.Repeat("技能", 100))
	require.NotContains(t, inventory, "disabled")
	require.NotContains(t, inventory, "missing name")
	require.NotContains(t, inventory, "Use linters")
	require.NotContains(t, inventory, "scripts/deploy.sh")
	require.Less(t, strings.Index(inventory, "  - code-review:"), strings.Index(inventory, "  - deploy-app:"))
	for i := 0; i < 10; i++ {
		require.Equal(t, inventory, NewSkillsContextManager(base).SkillInventory(), "map iteration order must not affect the directory")
	}

	mgr.BindTimelineSink(newTestTimelineSink())
	require.NoError(t, mgr.LoadSkill("deploy-app"))
	require.NoError(t, mgr.ChangeViewOffset("deploy-app", "SKILL.md", 2))
	mgr.SetCatalogSkills(base.AllSkillMetas())
	require.True(t, mgr.UnloadSkill("deploy-app"))
	base.skills["code-review"].meta.Description = "changed after initialization"
	base.skills["later"] = &skillEntry{meta: &SkillMeta{Name: "later", Description: "new source"}}
	delete(base.skills, "deploy-app")
	require.Equal(t, inventory, mgr.SkillInventory(), "loading, paging, unloading, catalog and loader changes must not rewrite the snapshot")
}

func TestSkillInventoryBudgetIncludesOverflowHint(t *testing.T) {
	metas := make([]*SkillMeta, 0, 200)
	for i := 199; i >= 0; i-- {
		metas = append(metas, &SkillMeta{
			Name:        fmt.Sprintf("skill-%03d", i),
			Description: strings.Repeat("用于检查应用配置与运行状态。", 20),
		})
	}
	// Duplicate, disabled and empty entries must not inflate the overflow count.
	metas = append(metas, metas[0], nil, &SkillMeta{}, &SkillMeta{Name: "disabled", DisableModelInvocation: true})
	for _, test := range []struct {
		name      string
		estimator func(string) int
	}{
		{name: "default"},
		{name: "custom", estimator: func(s string) int { return len(s) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := renderSkillInventory(metas, test.estimator)
			listed := strings.Count(inventory, "  - ")
			require.Positive(t, listed)
			require.Less(t, listed, 200)
			require.Contains(t, inventory, "  - skill-000:")
			require.Contains(t, inventory, strings.TrimSpace(AvailableSkillsOverflowHint(200-listed)))
			require.LessOrEqual(t, MeasureStringTokens(inventory, test.estimator), SkillInventoryMaxTokens)
		})
	}
}

func TestSkillInventoryEmptyAndNilManager(t *testing.T) {
	require.Empty(t, (*SkillsContextManager)(nil).SkillInventory())
	require.Empty(t, NewSkillsContextManager(nil).SkillInventory())
	require.Empty(t, renderSkillInventory([]*SkillMeta{
		nil, {}, {Name: "disabled", DisableModelInvocation: true},
	}, nil))
}
