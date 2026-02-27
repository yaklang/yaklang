package aicommon

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func buildResolverTestVFS(names ...string) *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	for _, name := range names {
		vfs.AddFile(name+"/SKILL.md", "---\nname: "+name+"\ndescription: Test skill "+name+"\n---\n# "+name+"\n")
	}
	return vfs
}

func makeTestConfigWithSkills(names ...string) *Config {
	vfs := buildResolverTestVFS(names...)
	loader, err := aiskillloader.NewAutoSkillLoader()
	if err != nil {
		return &Config{}
	}
	loader.AddSource(vfs)
	return &Config{
		skillLoader: loader,
	}
}

func TestResolveIdentifier_SkillOnly(t *testing.T) {
	config := makeTestConfigWithSkills("recon")

	resolved := config.ResolveIdentifier("recon")
	if resolved.IdentityType != ResolvedAs_Skill {
		t.Errorf("expected Skill, got %s", resolved.IdentityType)
	}
	if len(resolved.Alternatives) != 0 {
		t.Errorf("expected no alternatives, got %d", len(resolved.Alternatives))
	}
	if !strings.Contains(resolved.Suggestion, "SKILL") {
		t.Errorf("suggestion should mention SKILL, got: %s", resolved.Suggestion)
	}
}

func TestResolveIdentifier_Unknown(t *testing.T) {
	config := &Config{}
	resolved := config.ResolveIdentifier("nonexistent")
	if !resolved.IsUnknown() {
		t.Errorf("expected Unknown, got %s", resolved.IdentityType)
	}
}

func TestResolveIdentifier_EmptyName(t *testing.T) {
	config := &Config{}
	resolved := config.ResolveIdentifier("")
	if !resolved.IsUnknown() {
		t.Errorf("expected Unknown for empty name, got %s", resolved.IdentityType)
	}
}

func TestResolvedIdentifier_HasAlternative(t *testing.T) {
	r := &ResolvedIdentifier{
		Name:         "test",
		IdentityType: ResolvedAs_Forge,
		Alternatives: []*ResolvedIdentifier{
			{Name: "test", IdentityType: ResolvedAs_Skill},
		},
	}

	if !r.HasAlternative(ResolvedAs_Skill) {
		t.Error("should have skill alternative")
	}
	if r.HasAlternative(ResolvedAs_Tool) {
		t.Error("should not have tool alternative")
	}
}

func TestResolvedIdentifier_GetAlternative(t *testing.T) {
	r := &ResolvedIdentifier{
		Name:         "test",
		IdentityType: ResolvedAs_Forge,
		Alternatives: []*ResolvedIdentifier{
			{Name: "test", IdentityType: ResolvedAs_Skill, ActionType: "loading_skills"},
		},
	}

	alt := r.GetAlternative(ResolvedAs_Skill)
	if alt == nil {
		t.Fatal("should return skill alternative")
	}
	if alt.ActionType != "loading_skills" {
		t.Errorf("expected loading_skills, got %s", alt.ActionType)
	}

	nilAlt := r.GetAlternative(ResolvedAs_Tool)
	if nilAlt != nil {
		t.Error("should return nil for non-existing alternative")
	}
}

func TestResolvedIdentifier_NoAlternatives(t *testing.T) {
	r := &ResolvedIdentifier{
		Name:         "test",
		IdentityType: ResolvedAs_Tool,
	}

	if r.HasAlternative(ResolvedAs_Skill) {
		t.Error("should not have alternatives when none set")
	}
	if r.GetAlternative(ResolvedAs_Skill) != nil {
		t.Error("GetAlternative should return nil when none set")
	}
}

func TestResolveIdentifier_MultipleMatches_Suggestion(t *testing.T) {
	r := &ResolvedIdentifier{
		Name:         "recon",
		IdentityType: ResolvedAs_Forge,
		Alternatives: []*ResolvedIdentifier{
			{Name: "recon", IdentityType: ResolvedAs_Skill},
		},
	}
	r.Suggestion = buildResolveSuggestion("recon", r, []*ResolvedIdentifier{r, r.Alternatives[0]})

	if !strings.Contains(r.Suggestion, "MULTIPLE") {
		t.Errorf("suggestion should mention MULTIPLE types, got: %s", r.Suggestion)
	}
	if !strings.Contains(r.Suggestion, "Skill") {
		t.Errorf("suggestion should mention Skill, got: %s", r.Suggestion)
	}
	if !strings.Contains(r.Suggestion, "AI Blueprint") {
		t.Errorf("suggestion should mention AI Blueprint, got: %s", r.Suggestion)
	}
}

func TestResolveIdentifier_SingleMatch_Suggestion(t *testing.T) {
	tests := []struct {
		identityType  ResolvedIdentifierType
		expectedInSug string
	}{
		{ResolvedAs_Tool, "TOOL"},
		{ResolvedAs_Forge, "AI Blueprint"},
		{ResolvedAs_Skill, "SKILL"},
	}

	for _, tt := range tests {
		sug := buildSingleSuggestion("test", &ResolvedIdentifier{
			Name:         "test",
			IdentityType: tt.identityType,
		})
		if !strings.Contains(sug, tt.expectedInSug) {
			t.Errorf("for type %s, suggestion should contain %q, got: %s", tt.identityType, tt.expectedInSug, sug)
		}
	}
}

func TestResolveIdentifier_MultipleSkills(t *testing.T) {
	config := makeTestConfigWithSkills("alpha", "beta", "gamma")

	for _, name := range []string{"alpha", "beta", "gamma"} {
		resolved := config.ResolveIdentifier(name)
		if resolved.IdentityType != ResolvedAs_Skill {
			t.Errorf("%s: expected Skill, got %s", name, resolved.IdentityType)
		}
	}
}

func TestDescribeType(t *testing.T) {
	tests := []struct {
		t    ResolvedIdentifierType
		want string
	}{
		{ResolvedAs_Tool, "Tool"},
		{ResolvedAs_Forge, "AI Blueprint (forge)"},
		{ResolvedAs_Skill, "Skill"},
		{ResolvedAs_FocusedMode, "Focus Mode"},
		{ResolvedAs_Unknown, "Unknown"},
	}
	for _, tt := range tests {
		got := describeType(tt.t)
		if got != tt.want {
			t.Errorf("describeType(%s) = %q, want %q", tt.t, got, tt.want)
		}
	}
}
