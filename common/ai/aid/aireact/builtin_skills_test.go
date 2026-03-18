package aireact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
)

// allBuiltinSkills lists every built-in skill that ships with the binary.
// Update this slice when adding or removing embedded skills.
var allBuiltinSkills = []struct {
	name     string
	fsPath   string // path inside the embedded FS
	keywords []string
}{
	{"code-review", "skills/code-review/SKILL.md", []string{"grep", "CWE-89", "CWE-77", "CWE-79"}},
	{"xss-testing", "skills/xss-testing/SKILL.md", []string{"XSS", "Payload", "CSP"}},
	{"sql-injection", "skills/sql-injection/SKILL.md", []string{"UNION", "SQL", "Payload"}},
	{"command-injection", "skills/command-injection/SKILL.md", []string{"CWE-77", "CWE-78", "Payload"}},
	{"template-injection", "skills/template-injection/SKILL.md", []string{"SSTI", "Jinja2", "Freemarker"}},
	{"recon-planning", "skills/recon-planning/SKILL.md", []string{"OWASP", "Recon", "Scoping"}},
	{"web-crawler", "skills/web-crawler/SKILL.md", []string{"URL", "API", "JavaScript"}},
	{"pentest-task-design", "skills/pentest-task-design/SKILL.md", []string{"scan_port", "do_http_request", "OWASP", "Phase"}},
	{"use-browser", "skills/use-browser/SKILL.md", []string{"snapshot", "click", "fill", "screenshot", "CDP"}},
}

// TestBuiltinSkillsFS_ContainsAllSkills verifies that the embedded filesystem
// contains all expected built-in SKILL.md files.
func TestBuiltinSkillsFS_ContainsAllSkills(t *testing.T) {
	fs := GetBuiltinSkillsFS()
	if fs == nil {
		t.Fatal("GetBuiltinSkillsFS() returned nil")
	}

	for _, skill := range allBuiltinSkills {
		content, err := fs.ReadFile(skill.fsPath)
		if err != nil {
			t.Errorf("failed to read %s from embedded FS: %v", skill.fsPath, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("%s is empty", skill.fsPath)
		}
		t.Logf("embedded %s size: %d bytes", skill.name, len(content))
	}
}

// TestBuiltinSkillsFS_AllMetaValid parses every built-in SKILL.md and validates
// the metadata fields and expected content keywords.
func TestBuiltinSkillsFS_AllMetaValid(t *testing.T) {
	fs := GetBuiltinSkillsFS()

	for _, skill := range allBuiltinSkills {
		t.Run(skill.name, func(t *testing.T) {
			content, err := fs.ReadFile(skill.fsPath)
			if err != nil {
				t.Fatalf("failed to read SKILL.md: %v", err)
			}

			meta, err := aiskillloader.ParseSkillMeta(string(content))
			if err != nil {
				t.Fatalf("ParseSkillMeta failed: %v", err)
			}

			if meta.Name != skill.name {
				t.Errorf("expected name %q, got %q", skill.name, meta.Name)
			}
			if meta.Description == "" {
				t.Error("description must not be empty")
			}
			if meta.Body == "" {
				t.Error("body must not be empty")
			}

			for _, kw := range skill.keywords {
				if !strings.Contains(meta.Body, kw) {
					t.Errorf("body missing expected keyword: %q", kw)
				}
			}

			t.Logf("parsed skill: name=%s, body_length=%d", meta.Name, len(meta.Body))
		})
	}
}

// TestExtractBuiltinSkills_WritesToBuiltinSubdir verifies that
// ExtractBuiltinSkillsToDir places files under a "builtin/" subdirectory.
func TestExtractBuiltinSkills_WritesToBuiltinSubdir(t *testing.T) {
	tmpDir := t.TempDir()

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("ExtractBuiltinSkillsToDir failed: %v", err)
	}

	for _, skill := range allBuiltinSkills {
		relPath := strings.TrimPrefix(skill.fsPath, "skills/")
		expectedPath := filepath.Join(tmpDir, "builtin", relPath)

		info, err := os.Stat(expectedPath)
		if err != nil {
			t.Errorf("expected file at %s but got error: %v", expectedPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("extracted file %s is empty", expectedPath)
		}
		t.Logf("verified extracted: %s (%d bytes)", expectedPath, info.Size())
	}

	// Verify that files are NOT at the old (non-builtin) path
	oldPath := filepath.Join(tmpDir, "code-review", "SKILL.md")
	if _, err := os.Stat(oldPath); err == nil {
		t.Errorf("file should NOT exist at old path %s (should be under builtin/)", oldPath)
	}
}

// TestBuiltinSkills_LoadedByReAct creates a ReAct instance WITHOUT disabling
// auto-skills and verifies that all built-in skills are loaded.
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
		t.Fatal("expected at least one skill to be loaded")
	}

	metas := loader.AllSkillMetas()
	loadedNames := make(map[string]bool, len(metas))
	for _, m := range metas {
		loadedNames[m.Name] = true
	}

	for _, skill := range allBuiltinSkills {
		if !loadedNames[skill.name] {
			names := make([]string, 0, len(loadedNames))
			for n := range loadedNames {
				names = append(names, n)
			}
			t.Errorf("built-in skill %q not found; available: %v", skill.name, names)
		} else {
			t.Logf("found built-in skill: %s", skill.name)
		}
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
