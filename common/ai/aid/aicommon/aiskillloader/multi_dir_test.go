package aiskillloader

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func writeTestSkillMD(t *testing.T, dir, skillName string) {
	t.Helper()
	skillDir := filepath.Join(dir, skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	content := "---\nname: " + skillName + "\ndescription: test skill " + skillName + "\n---\n# " + skillName + "\nBody content.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

// --- RefreshFromDirs ---

func TestRefreshFromDirs_MultipleLocalDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	writeTestSkillMD(t, dir1, "skill-from-cursor")
	writeTestSkillMD(t, dir2, "skill-from-claude")
	writeTestSkillMD(t, dir2, "skill-from-claude-extra")
	writeTestSkillMD(t, dir3, "skill-from-opencode")

	loader, err := NewAutoSkillLoader()
	if err != nil {
		t.Fatalf("NewAutoSkillLoader: %v", err)
	}

	loader.RefreshFromDirs([]string{dir1, dir2, dir3})

	metas := loader.AllSkillMetas()
	if len(metas) != 4 {
		names := make([]string, 0, len(metas))
		for _, m := range metas {
			names = append(names, m.Name)
		}
		t.Fatalf("expected 4 skills, got %d: %v", len(metas), names)
	}

	nameSet := make(map[string]bool)
	for _, m := range metas {
		nameSet[m.Name] = true
	}
	for _, want := range []string{"skill-from-cursor", "skill-from-claude", "skill-from-claude-extra", "skill-from-opencode"} {
		if !nameSet[want] {
			t.Errorf("skill %q not found", want)
		}
	}
}

func TestRefreshFromDirs_SkipsNonExistentDirs(t *testing.T) {
	realDir := t.TempDir()
	writeTestSkillMD(t, realDir, "real-skill")

	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs([]string{
		"/tmp/absolutely-does-not-exist-xyz-123",
		realDir,
		"/tmp/another-nonexistent",
	})

	metas := loader.AllSkillMetas()
	if len(metas) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(metas))
	}
	if metas[0].Name != "real-skill" {
		t.Fatalf("expected real-skill, got %s", metas[0].Name)
	}
}

func TestRefreshFromDirs_DedupSameDir(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillMD(t, dir, "dedup-skill")

	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs([]string{dir, dir, dir})

	metas := loader.AllSkillMetas()
	if len(metas) != 1 {
		t.Fatalf("expected 1 skill (dedup), got %d", len(metas))
	}

	loader.mu.RLock()
	srcCount := len(loader.sources)
	loader.mu.RUnlock()
	if srcCount != 1 {
		t.Fatalf("expected 1 source (dedup), got %d", srcCount)
	}
}

func TestRefreshFromDirs_EmptyList(t *testing.T) {
	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs(nil)
	loader.RefreshFromDirs([]string{})
	if loader.HasSkills() {
		t.Fatal("empty dir list should discover no skills")
	}
}

// --- CoolDown ---

func TestRefreshFromDirs_CoolDown(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeTestSkillMD(t, dir1, "first-skill")
	writeTestSkillMD(t, dir2, "second-skill")

	loader, _ := NewAutoSkillLoader()
	loader.cooldown.Close()
	loader.cooldown = utils.NewCoolDown(time.Minute)

	loader.RefreshFromDirs([]string{dir1})
	if len(loader.AllSkillMetas()) != 1 {
		t.Fatal("first call should discover 1 skill")
	}

	loader.RefreshFromDirs([]string{dir2})
	if len(loader.AllSkillMetas()) != 1 {
		t.Fatal("second call within cooldown should be skipped, still 1 skill")
	}
}

func TestRefreshFromDirs_CoolDown_ExpiresAndAllowsRescan(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeTestSkillMD(t, dir1, "first-skill")
	writeTestSkillMD(t, dir2, "second-skill")

	loader, _ := NewAutoSkillLoader()
	loader.cooldown.Close()
	loader.cooldown = utils.NewCoolDown(50 * time.Millisecond)

	loader.RefreshFromDirs([]string{dir1})
	if len(loader.AllSkillMetas()) != 1 {
		t.Fatal("first call should discover 1 skill")
	}

	time.Sleep(100 * time.Millisecond)

	loader.RefreshFromDirs([]string{dir1, dir2})
	if len(loader.AllSkillMetas()) != 2 {
		t.Fatalf("after cooldown, should discover second-skill too, got %d", len(loader.AllSkillMetas()))
	}
}

func TestRefreshFromDirs_CoolDown_ConcurrentSafety(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillMD(t, dir, "concurrent-skill")

	loader, _ := NewAutoSkillLoader()
	loader.cooldown.Close()
	loader.cooldown = utils.NewCoolDown(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loader.RefreshFromDirs([]string{dir})
		}()
	}
	wg.Wait()

	metas := loader.AllSkillMetas()
	if len(metas) != 1 {
		t.Fatalf("expected 1 skill under concurrent refresh, got %d", len(metas))
	}
}

// --- AddLocalDir dedup ---

func TestAddLocalDir_Dedup(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillMD(t, dir, "add-dedup-skill")

	loader, _ := NewAutoSkillLoader()

	count1, err := loader.AddLocalDir(dir)
	if err != nil {
		t.Fatalf("first AddLocalDir: %v", err)
	}
	if count1 != 1 {
		t.Fatalf("first call should discover 1, got %d", count1)
	}

	count2, err := loader.AddLocalDir(dir)
	if err != nil {
		t.Fatalf("second AddLocalDir: %v", err)
	}
	if count2 != 0 {
		t.Fatalf("second call should discover 0 (dedup), got %d", count2)
	}

	if len(loader.AllSkillMetas()) != 1 {
		t.Fatal("should still have exactly 1 skill")
	}
}

func TestAddLocalDir_NonExistent(t *testing.T) {
	loader, _ := NewAutoSkillLoader()
	_, err := loader.AddLocalDir("/tmp/this-path-should-not-exist-xyz")
	if err == nil {
		t.Fatal("should fail for non-existent dir")
	}
}

// --- RescanLocalDir ---

func TestRescanLocalDir_PicksUpNewSkills(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillMD(t, dir, "initial-skill")

	loader, _ := NewAutoSkillLoader()
	count, err := loader.AddLocalDir(dir)
	if err != nil {
		t.Fatalf("AddLocalDir: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	writeTestSkillMD(t, dir, "new-skill-added-later")

	count2, err := loader.RescanLocalDir(dir)
	if err != nil {
		t.Fatalf("RescanLocalDir: %v", err)
	}
	if count2 != 1 {
		t.Fatalf("rescan should discover 1 new skill, got %d", count2)
	}

	if len(loader.AllSkillMetas()) != 2 {
		t.Fatalf("expected 2 total skills, got %d", len(loader.AllSkillMetas()))
	}

	loaded, err := loader.LoadSkill("new-skill-added-later")
	if err != nil {
		t.Fatalf("LoadSkill: %v", err)
	}
	if loaded.Meta.Name != "new-skill-added-later" {
		t.Fatalf("unexpected name: %s", loaded.Meta.Name)
	}
}

func TestRescanLocalDir_DoesNotDuplicateSources(t *testing.T) {
	dir := t.TempDir()
	writeTestSkillMD(t, dir, "src-dedup-skill")

	loader, _ := NewAutoSkillLoader()
	loader.AddLocalDir(dir)

	loader.mu.RLock()
	srcBefore := len(loader.sources)
	loader.mu.RUnlock()

	loader.RescanLocalDir(dir)
	loader.RescanLocalDir(dir)

	loader.mu.RLock()
	srcAfter := len(loader.sources)
	loader.mu.RUnlock()

	if srcAfter != srcBefore {
		t.Fatalf("RescanLocalDir should not duplicate sources: before=%d after=%d", srcBefore, srcAfter)
	}
}

func TestRescanLocalDir_NonExistent(t *testing.T) {
	loader, _ := NewAutoSkillLoader()
	_, err := loader.RescanLocalDir("/tmp/nonexistent-rescan-xyz")
	if err == nil {
		t.Fatal("should fail for non-existent dir")
	}
}

// --- Integration: simulates the NewReAct flow ---

func TestIntegration_ConfigThenExtractThenRescan(t *testing.T) {
	baseDir := t.TempDir()
	writeTestSkillMD(t, baseDir, "pre-existing-skill")

	loader, _ := NewAutoSkillLoader()

	loader.RefreshFromDirs([]string{baseDir})
	if len(loader.AllSkillMetas()) != 1 {
		t.Fatal("should find pre-existing skill")
	}

	writeTestSkillMD(t, baseDir, "builtin-extracted-skill")

	count, err := loader.RescanLocalDir(baseDir)
	if err != nil {
		t.Fatalf("RescanLocalDir: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 new skill from rescan, got %d", count)
	}

	metas := loader.AllSkillMetas()
	if len(metas) != 2 {
		t.Fatalf("expected 2 total, got %d", len(metas))
	}

	nameSet := make(map[string]bool)
	for _, m := range metas {
		nameSet[m.Name] = true
	}
	if !nameSet["pre-existing-skill"] || !nameSet["builtin-extracted-skill"] {
		t.Fatalf("missing expected skills: %v", nameSet)
	}
}

func TestIntegration_MultiToolDirs(t *testing.T) {
	root := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor", "skills")
	claudeDir := filepath.Join(root, ".claude", "skills")
	githubDir := filepath.Join(root, ".github", "skills")
	opencodeDir := filepath.Join(root, ".opencode", "skills")
	copilotDir := filepath.Join(root, ".copilot", "skills")

	writeTestSkillMD(t, cursorDir, "cursor-deployment")
	writeTestSkillMD(t, claudeDir, "claude-code-review")
	writeTestSkillMD(t, githubDir, "github-ci-cd")
	writeTestSkillMD(t, opencodeDir, "opencode-refactor")
	writeTestSkillMD(t, copilotDir, "copilot-testing")

	loader, _ := NewAutoSkillLoader()

	dirs := []string{cursorDir, claudeDir, githubDir, opencodeDir, copilotDir}
	loader.RefreshFromDirs(dirs)

	metas := loader.AllSkillMetas()
	if len(metas) != 5 {
		names := make([]string, 0, len(metas))
		for _, m := range metas {
			names = append(names, m.Name)
		}
		t.Fatalf("expected 5 skills from 5 tool dirs, got %d: %v", len(metas), names)
	}

	for _, want := range []string{"cursor-deployment", "claude-code-review", "github-ci-cd", "opencode-refactor", "copilot-testing"} {
		if _, err := loader.LoadSkill(want); err != nil {
			t.Errorf("LoadSkill(%q) failed: %v", want, err)
		}
	}
}

// --- Edge cases ---

func TestRefreshFromDirs_EmptyDirNoSkills(t *testing.T) {
	dir := t.TempDir()

	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs([]string{dir})

	if loader.HasSkills() {
		t.Fatal("empty dir should produce no skills")
	}
}

func TestRefreshFromDirs_MalformedSkillMD(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("not yaml at all {{{"), 0o644)

	writeTestSkillMD(t, dir, "good-skill")

	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs([]string{dir})

	metas := loader.AllSkillMetas()
	if len(metas) != 1 {
		t.Fatalf("expected 1 good skill (bad one ignored), got %d", len(metas))
	}
	if metas[0].Name != "good-skill" {
		t.Fatalf("expected good-skill, got %s", metas[0].Name)
	}
}

func TestRefreshFromDirs_SkillOverrideByName(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writeTestSkillMD(t, dir1, "shared-name")
	writeTestSkillMD(t, dir2, "shared-name")

	loader, _ := NewAutoSkillLoader()
	loader.RefreshFromDirs([]string{dir1, dir2})

	metas := loader.AllSkillMetas()
	if len(metas) != 1 {
		t.Fatalf("same-name skills should be deduplicated by name, got %d", len(metas))
	}
}

// --- CoolDown: verifies only 1 execution within window ---

func TestCoolDown_OnlyOneExecutionPerWindow(t *testing.T) {
	var execCount int64

	loader, _ := NewAutoSkillLoader()
	loader.cooldown.Close()
	loader.cooldown = utils.NewCoolDown(500 * time.Millisecond)

	dir := t.TempDir()
	writeTestSkillMD(t, dir, "cd-test-skill")

	origRefresh := func() {
		atomic.AddInt64(&execCount, 1)
		loader.addLocalDirInternal(dir)
	}

	loader.cooldown.Do(origRefresh)
	loader.cooldown.Do(origRefresh)
	loader.cooldown.Do(origRefresh)

	if atomic.LoadInt64(&execCount) != 1 {
		t.Fatalf("expected exactly 1 execution within cooldown, got %d", atomic.LoadInt64(&execCount))
	}
}

func (l *AutoSkillLoader) addLocalDirInternal(dir string) {
	if !utils.IsDir(dir) {
		return
	}
	absDir, _ := filepath.Abs(dir)
	l.mu.RLock()
	_, exists := l.scannedDirs[absDir]
	l.mu.RUnlock()
	if exists {
		return
	}
	localFS := filesys.NewRelLocalFs(dir)
	l.discoverSkills(localFS)
	l.mu.Lock()
	l.sources = append(l.sources, localFS)
	l.scannedDirs[absDir] = struct{}{}
	l.mu.Unlock()
}
