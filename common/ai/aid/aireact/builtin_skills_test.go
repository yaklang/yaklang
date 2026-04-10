package aireact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	{"how-to-use-browser", "skills/how-to-use-browser/SKILL.md", []string{"snapshot", "click", "fill", "screenshot", "CDP"}},
	{"authorization-bypass", "skills/authorization-bypass/SKILL.md", []string{"IDOR", "WSTG-ATHZ-02", "Horizontal", "Vertical", "do_http_request"}},
}

func useTempBuiltinSkillReleaseDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "profile.db")
	db, err := consts.CreateProfileDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create temp profile db: %v", err)
	}

	originalResolver := builtinSkillReleaseDB
	builtinSkillReleaseDB = func() *gorm.DB {
		return db
	}

	t.Cleanup(func() {
		builtinSkillReleaseDB = originalResolver
		_ = db.Close()
	})
}

func useTempYakitHome(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	originalYakitHome := os.Getenv("YAKIT_HOME")
	if err := os.Setenv("YAKIT_HOME", tempDir); err != nil {
		t.Fatalf("failed to set YAKIT_HOME: %v", err)
	}
	consts.ResetYakitHomeOnce()

	t.Cleanup(func() {
		if err := os.Setenv("YAKIT_HOME", originalYakitHome); err != nil {
			t.Fatalf("failed to restore YAKIT_HOME: %v", err)
		}
		consts.ResetYakitHomeOnce()
	})

	return tempDir
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
	useTempBuiltinSkillReleaseDB(t)
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
		if releasedAt, ok := getBuiltinSkillReleaseTime(relPath); !ok || releasedAt.IsZero() {
			t.Errorf("expected release time to be recorded for %s", relPath)
		}
		t.Logf("verified extracted: %s (%d bytes)", expectedPath, info.Size())
	}

	// Verify that files are NOT at the old (non-builtin) path
	oldPath := filepath.Join(tmpDir, "code-review", "SKILL.md")
	if _, err := os.Stat(oldPath); err == nil {
		t.Errorf("file should NOT exist at old path %s (should be under builtin/)", oldPath)
	}
}

func TestExtractBuiltinSkills_PreservesExistingFile(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)
	customContent := []byte("user customized skill content\n")

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := os.WriteFile(targetPath, customContent, 0o644); err != nil {
		t.Fatalf("failed to write custom skill file: %v", err)
	}

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("ExtractBuiltinSkillsToDir failed: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read preserved skill file: %v", err)
	}
	if string(got) != string(customContent) {
		t.Fatalf("existing skill file was overwritten, got %q", string(got))
	}
	if raw := yakit.GetKey(builtinSkillReleaseDB(), builtinSkillReleaseKey(relPath)); raw != "" {
		t.Fatalf("expected no release record for pre-existing local file, got %q", raw)
	}
}

func TestExtractBuiltinSkills_DoesNotReExtractAfterUserDeletesFile(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("first ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected extracted file: %v", err)
	}

	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("failed to remove skill file: %v", err)
	}

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("second ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to remain deleted, stat err=%v", err)
	}
	if !isBuiltinSkillSuppressed(relPath) {
		t.Fatal("expected suppression record after user deleted a previously extracted builtin skill")
	}

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("third ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatal("expected file to stay absent on subsequent extracts")
	}
}

func TestExtractBuiltinSkills_ReExtractsAfterSuppressionCleared(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("first ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("failed to remove skill file: %v", err)
	}
	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("second ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if !isBuiltinSkillSuppressed(relPath) {
		t.Fatal("expected suppression record")
	}

	yakit.DelKey(builtinSkillReleaseDB(), builtinSkillSuppressedKey(relPath))
	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("third ExtractBuiltinSkillsToDir failed: %v", err)
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected builtin skill file to be re-extracted after clearing suppression: %v", err)
	}
}

func TestExtractBuiltinSkills_PreservesModifiedFileAfterRelease(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("first ExtractBuiltinSkillsToDir failed: %v", err)
	}

	originalRelease := yakit.GetKey(builtinSkillReleaseDB(), builtinSkillReleaseKey(relPath))
	if originalRelease == "" {
		t.Fatal("expected release record after initial extraction")
	}

	customContent := []byte("user modified after release\n")
	if err := os.WriteFile(targetPath, customContent, 0o644); err != nil {
		t.Fatalf("failed to modify skill file: %v", err)
	}

	releasedAt, ok := getBuiltinSkillReleaseTime(relPath)
	if !ok {
		t.Fatal("expected parsed release time after initial extraction")
	}
	modifiedAt := releasedAt.Add(2 * time.Second)
	if err := os.Chtimes(targetPath, modifiedAt, modifiedAt); err != nil {
		t.Fatalf("failed to update skill file mtime: %v", err)
	}

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("second ExtractBuiltinSkillsToDir failed: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read modified skill file: %v", err)
	}
	if string(got) != string(customContent) {
		t.Fatalf("modified skill file was overwritten, got %q", string(got))
	}
	if currentRelease := yakit.GetKey(builtinSkillReleaseDB(), builtinSkillReleaseKey(relPath)); currentRelease != originalRelease {
		t.Fatalf("expected release record to remain unchanged, got %q want %q", currentRelease, originalRelease)
	}
	if info, err := os.Stat(targetPath); err != nil {
		t.Fatalf("failed to stat modified skill file: %v", err)
	} else if !info.ModTime().After(releasedAt) {
		t.Fatalf("expected modified skill file mtime %v to be after release time %v", info.ModTime(), releasedAt)
	}
}

func TestExtractBuiltinSkills_PreservesExistingFileEvenWithPriorReleaseRecord(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)
	customContent := []byte("keep legacy customized content\n")

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := os.WriteFile(targetPath, customContent, 0o644); err != nil {
		t.Fatalf("failed to seed custom skill file: %v", err)
	}

	legacyReleaseAt := time.Now().Add(2 * time.Hour)
	markBuiltinSkillReleased(relPath, legacyReleaseAt)
	olderMtime := legacyReleaseAt.Add(-1 * time.Hour)
	if err := os.Chtimes(targetPath, olderMtime, olderMtime); err != nil {
		t.Fatalf("failed to backdate skill file mtime: %v", err)
	}

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("ExtractBuiltinSkillsToDir failed: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read preserved legacy skill file: %v", err)
	}
	if string(got) != string(customContent) {
		t.Fatalf("legacy skill file was overwritten, got %q", string(got))
	}
	currentRelease, ok := getBuiltinSkillReleaseTime(relPath)
	if !ok {
		t.Fatal("expected existing release record to remain readable")
	}
	if currentRelease.UnixMilli() != legacyReleaseAt.UnixMilli() {
		t.Fatalf("expected release record to remain unchanged, got %v want %v", currentRelease, legacyReleaseAt)
	}
	if info, err := os.Stat(targetPath); err != nil {
		t.Fatalf("failed to stat legacy skill file: %v", err)
	} else if !info.ModTime().Before(currentRelease) {
		t.Fatalf("expected legacy skill file mtime %v to remain before release time %v", info.ModTime(), currentRelease)
	}
}

func TestExtractBuiltinSkills_ReleaseRecordMatchesWrittenFileTime(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tmpDir := t.TempDir()
	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	targetPath := filepath.Join(tmpDir, "builtin", relPath)

	if err := ExtractBuiltinSkillsToDir(tmpDir); err != nil {
		t.Fatalf("ExtractBuiltinSkillsToDir failed: %v", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat extracted skill file: %v", err)
	}
	releasedAt, ok := getBuiltinSkillReleaseTime(relPath)
	if !ok {
		t.Fatal("expected release record after extraction")
	}
	if releasedAt.UnixMilli() != info.ModTime().UnixMilli() {
		t.Fatalf("expected release record %v to match file mod time %v", releasedAt, info.ModTime())
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

func TestBuiltinSkills_DisabledDoesNotExtractBuiltinFiles(t *testing.T) {
	useTempBuiltinSkillReleaseDB(t)
	tempYakitHome := useTempYakitHome(t)

	_, err := NewReAct(
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
	)
	if err != nil {
		t.Fatalf("NewReAct failed: %v", err)
	}

	relPath := strings.TrimPrefix(allBuiltinSkills[0].fsPath, "skills/")
	extractedPath := filepath.Join(tempYakitHome, "ai-skills", "builtin", relPath)
	if _, err := os.Stat(extractedPath); err == nil {
		t.Fatalf("builtin skill should not be extracted when auto-skills are disabled: %s", extractedPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed to stat builtin skill path %s: %v", extractedPath, err)
	}
	if raw := yakit.GetKey(builtinSkillReleaseDB(), builtinSkillReleaseKey(relPath)); raw != "" {
		t.Fatalf("expected no release record when auto-skills are disabled, got %q", raw)
	}
}
