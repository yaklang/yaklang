package aiskillloader

import (
	"strings"
	"testing"
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

	listed, omitted := SelectSkillMetasForPromptRegistry(loader.AllSkillMetas(), mgr.TokenEstimator())
	if len(listed) != 2 || omitted != 0 {
		t.Fatalf("expected full registry list, got listed=%d omitted=%d", len(listed), omitted)
	}

	rendered := mgr.RenderStable()
	if !strings.Contains(rendered, "  - code-review:") || !strings.Contains(rendered, "  - deploy-app:") {
		t.Fatalf("rendered prompt should list both skills: %s", rendered)
	}
}
