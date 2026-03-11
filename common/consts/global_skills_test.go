package consts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetAllAISkillsDirs_AlwaysContainsDefault(t *testing.T) {
	dirs := GetAllAISkillsDirs()
	if len(dirs) == 0 {
		t.Fatal("GetAllAISkillsDirs must return at least the default dir")
	}
	defaultDir := GetDefaultAISkillsDir()
	if dirs[0] != defaultDir {
		t.Fatalf("first entry should be the yakit default dir %q, got %q", defaultDir, dirs[0])
	}
}

func TestGetAllAISkillsDirs_NoDuplicates(t *testing.T) {
	dirs := GetAllAISkillsDirs()
	seen := make(map[string]struct{})
	for _, d := range dirs {
		abs, _ := filepath.Abs(d)
		if _, dup := seen[abs]; dup {
			t.Fatalf("duplicate directory found: %s", d)
		}
		seen[abs] = struct{}{}
	}
}

func resolveReal(t *testing.T, p string) string {
	t.Helper()
	abs, _ := filepath.Abs(p)
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return real
}

func TestGetAllAISkillsDirs_PicksUpCursorProjectDir(t *testing.T) {
	root := t.TempDir()
	cursorSkills := filepath.Join(root, ".cursor", "skills")
	if err := os.MkdirAll(cursorSkills, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	want := resolveReal(t, cursorSkills)
	found := false
	for _, d := range dirs {
		if resolveReal(t, d) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .cursor/skills in CWD to be picked up, dirs = %v", dirs)
	}
}

func TestGetAllAISkillsDirs_PicksUpClaudeProjectDir(t *testing.T) {
	root := t.TempDir()
	claudeSkills := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	want := resolveReal(t, claudeSkills)
	found := false
	for _, d := range dirs {
		if resolveReal(t, d) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .claude/skills in CWD to be picked up, dirs = %v", dirs)
	}
}

func TestGetAllAISkillsDirs_PicksUpGitHubProjectDir(t *testing.T) {
	root := t.TempDir()
	ghSkills := filepath.Join(root, ".github", "skills")
	if err := os.MkdirAll(ghSkills, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	want := resolveReal(t, ghSkills)
	found := false
	for _, d := range dirs {
		if resolveReal(t, d) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .github/skills in CWD to be picked up, dirs = %v", dirs)
	}
}

func TestGetAllAISkillsDirs_PicksUpOpenCodeProjectDir(t *testing.T) {
	root := t.TempDir()
	ocSkills := filepath.Join(root, ".opencode", "skills")
	if err := os.MkdirAll(ocSkills, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	want := resolveReal(t, ocSkills)
	found := false
	for _, d := range dirs {
		if resolveReal(t, d) == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .opencode/skills in CWD to be picked up, dirs = %v", dirs)
	}
}

func TestGetAllAISkillsDirs_IgnoresNonExistentDirs(t *testing.T) {
	root := t.TempDir()

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	for _, d := range dirs[1:] {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Fatalf("non-existent dir should not be returned: %s", d)
		}
	}
}

func TestGetAllAISkillsDirs_MultipleToolDirsAtOnce(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{".cursor/skills", ".claude/skills", ".github/skills", ".opencode/skills"} {
		os.MkdirAll(filepath.Join(root, sub), 0o755)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(root)

	dirs := GetAllAISkillsDirs()
	if len(dirs) < 5 {
		t.Fatalf("expected at least 5 dirs (1 default + 4 project-level), got %d: %v", len(dirs), dirs)
	}
}
