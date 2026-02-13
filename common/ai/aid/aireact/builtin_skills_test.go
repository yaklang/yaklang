package aireact

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
)

// TestBuiltinSkillsFS_ContainsCodeReview verifies that the embedded filesystem
// contains the production code-review SKILL.md file.
func TestBuiltinSkillsFS_ContainsCodeReview(t *testing.T) {
	fs := GetBuiltinSkillsFS()
	if fs == nil {
		t.Fatal("GetBuiltinSkillsFS() returned nil")
	}

	// The embedded FS should contain skills/code-review/SKILL.md
	content, err := fs.ReadFile("skills/code-review/SKILL.md")
	if err != nil {
		t.Fatalf("failed to read skills/code-review/SKILL.md from embedded FS: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("skills/code-review/SKILL.md is empty")
	}

	t.Logf("embedded SKILL.md size: %d bytes", len(content))
}

// TestBuiltinSkillsFS_CodeReviewMetaValid parses the production SKILL.md
// using the real aiskillloader.ParseSkillMeta and validates the metadata fields.
func TestBuiltinSkillsFS_CodeReviewMetaValid(t *testing.T) {
	fs := GetBuiltinSkillsFS()

	content, err := fs.ReadFile("skills/code-review/SKILL.md")
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	meta, err := aiskillloader.ParseSkillMeta(string(content))
	if err != nil {
		t.Fatalf("ParseSkillMeta failed on production SKILL.md: %v", err)
	}

	// Validate required fields
	if meta.Name != "code-review" {
		t.Errorf("expected name 'code-review', got %q", meta.Name)
	}
	if meta.Description == "" {
		t.Error("description must not be empty for production skill")
	}
	if meta.Body == "" {
		t.Error("body must not be empty for production skill")
	}

	// Validate body contains expected production content sections
	expectedSections := []string{
		"grep",
		"CWE-89",
		"CWE-77",
		"CWE-79",
		"CWE-918",
		"CWE-502",
		"CWE-22",
	}
	for _, section := range expectedSections {
		if !strings.Contains(meta.Body, section) {
			t.Errorf("production SKILL.md body missing expected section: %q", section)
		}
	}

	t.Logf("parsed skill: name=%s, description=%s, body_length=%d",
		meta.Name, meta.Description, len(meta.Body))
}

// TestBuiltinSkills_LoadedByReAct creates a ReAct instance WITHOUT disabling
// auto-skills and verifies that the built-in code-review skill is loaded.
func TestBuiltinSkills_LoadedByReAct(t *testing.T) {
	react, err := NewReAct(
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		// NOTE: NOT setting WithDisableAutoSkills(true) — built-in skills should load
	)
	if err != nil {
		t.Fatalf("NewReAct failed: %v", err)
	}

	loader := react.config.GetSkillLoader()
	if loader == nil {
		t.Fatal("skill loader should not be nil when auto-skills are enabled")
	}

	if !loader.HasSkills() {
		t.Fatal("expected at least one skill to be loaded (built-in code-review)")
	}

	metas := loader.AllSkillMetas()
	found := false
	for _, m := range metas {
		if m.Name == "code-review" {
			found = true
			t.Logf("found built-in skill: name=%s, description=%s", m.Name, m.Description)
			break
		}
	}
	if !found {
		names := make([]string, len(metas))
		for i, m := range metas {
			names[i] = m.Name
		}
		t.Fatalf("built-in code-review skill not found; available skills: %v", names)
	}
}

// TestBuiltinSkills_DisabledWhenRequested creates a ReAct instance WITH
// DisableAutoSkills and verifies that no built-in skills are loaded.
func TestBuiltinSkills_DisabledWhenRequested(t *testing.T) {
	react, err := NewTestReAct() // NewTestReAct sets WithDisableAutoSkills(true)
	if err != nil {
		t.Fatalf("NewTestReAct failed: %v", err)
	}

	loader := react.config.GetSkillLoader()
	if loader != nil && loader.HasSkills() {
		metas := loader.AllSkillMetas()
		names := make([]string, len(metas))
		for i, m := range metas {
			names[i] = m.Name
		}
		t.Fatalf("expected no skills when auto-skills disabled, but found: %v", names)
	}

	t.Log("confirmed: no built-in skills loaded when DisableAutoSkills is set")
}
