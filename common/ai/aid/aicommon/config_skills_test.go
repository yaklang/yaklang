package aicommon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// TestConfig_AutoLoadSkillsFromDefaultDir tests that skills are automatically loaded
// from the default directory (~/.yakit-projects/ai-skills) when NewConfig is called
// without WithDisableAutoSkills(true).
func TestConfig_AutoLoadSkillsFromDefaultDir(t *testing.T) {
	// Create a temporary directory to simulate yakit-projects
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")

	// Create the ai-skills directory with a test skill
	err := os.MkdirAll(filepath.Join(aiSkillsDir, "auto-test-skill"), 0755)
	if err != nil {
		t.Fatalf("failed to create ai-skills directory: %v", err)
	}

	// Write a test SKILL.md file
	skillContent := `---
name: auto-test-skill
description: A skill for testing auto-loading
---
# Auto Test Skill

This skill is automatically loaded from the default directory.
`
	err = os.WriteFile(filepath.Join(aiSkillsDir, "auto-test-skill", "SKILL.md"), []byte(skillContent), 0644)
	if err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Set YAKIT_HOME environment variable to use our temp directory
	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)

	// Reset the yakit home cache so it picks up our temp directory
	consts.ResetYakitHomeOnce()

	// Create Config WITHOUT WithDisableAutoSkills(true)
	// This should auto-load skills from our temp ai-skills directory
	config := NewConfig(context.Background())

	// Verify that the skill was auto-loaded
	loader := config.GetSkillLoader()
	if loader == nil {
		t.Fatal("GetSkillLoader() should not return nil when auto-loading is enabled and skills directory exists")
	}

	// Check that the skill is available
	skills := loader.AllSkillMetas()
	if len(skills) == 0 {
		t.Fatal("Expected at least one skill to be auto-loaded")
	}

	// Find our test skill
	var found bool
	for _, skill := range skills {
		if skill.Name == "auto-test-skill" {
			found = true
			if skill.Description != "A skill for testing auto-loading" {
				t.Errorf("Unexpected skill description: %s", skill.Description)
			}
			break
		}
	}

	if !found {
		t.Error("auto-test-skill should be in the loaded skills")
	}

	t.Logf("Successfully auto-loaded %d skill(s)", len(skills))
}

// TestConfig_DisableAutoSkillsOption tests that WithDisableAutoSkills(true) prevents
// automatic loading of skills from the default directory.
func TestConfig_DisableAutoSkillsOption(t *testing.T) {
	// Create a temporary directory to simulate yakit-projects
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")

	// Create the ai-skills directory with a test skill
	err := os.MkdirAll(filepath.Join(aiSkillsDir, "disable-test-skill"), 0755)
	if err != nil {
		t.Fatalf("failed to create ai-skills directory: %v", err)
	}

	// Write a test SKILL.md file
	skillContent := `---
name: disable-test-skill
description: This skill should NOT be loaded
---
# Disable Test Skill
`
	err = os.WriteFile(filepath.Join(aiSkillsDir, "disable-test-skill", "SKILL.md"), []byte(skillContent), 0644)
	if err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Set YAKIT_HOME environment variable
	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)

	// Reset the yakit home cache
	consts.ResetYakitHomeOnce()

	// Create Config WITH WithDisableAutoSkills(true)
	config := NewConfig(context.Background(), WithDisableAutoSkills(true))

	// Verify that NO skills were auto-loaded
	loader := config.GetSkillLoader()
	if loader != nil {
		t.Fatal("GetSkillLoader() should return nil when WithDisableAutoSkills(true) is used")
	}

	t.Log("WithDisableAutoSkills(true) correctly prevented auto-loading")
}

// TestConfig_ManualSkillsWithAutoDisabled tests that even when auto-loading is disabled,
// skills can still be manually added via WithSkillsFS, WithSkillsLocalDir, etc.
func TestConfig_ManualSkillsWithAutoDisabled(t *testing.T) {
	// Create a temporary directory to simulate yakit-projects (with skills we don't want)
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")

	// Create skills directory with a skill that should NOT be loaded
	err := os.MkdirAll(filepath.Join(aiSkillsDir, "unwanted-skill"), 0755)
	if err != nil {
		t.Fatalf("failed to create ai-skills directory: %v", err)
	}

	unwantedSkillContent := `---
name: unwanted-skill
description: This skill should NOT appear
---
# Unwanted Skill
`
	err = os.WriteFile(filepath.Join(aiSkillsDir, "unwanted-skill", "SKILL.md"), []byte(unwantedSkillContent), 0644)
	if err != nil {
		t.Fatalf("failed to write unwanted SKILL.md: %v", err)
	}

	// Set YAKIT_HOME environment variable
	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)

	// Reset the yakit home cache
	consts.ResetYakitHomeOnce()

	// Create a VFS with a manual skill we DO want
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("manual-skill/SKILL.md", `---
name: manual-skill
description: This skill was manually added
---
# Manual Skill

This skill was added via WithSkillsFS, not auto-loaded.
`)

	// Create Config with auto-loading disabled, but manually add skills
	config := NewConfig(
		context.Background(),
		WithDisableAutoSkills(true),
		WithSkillsFS(vfs),
	)

	// Verify that only the manual skill is loaded
	loader := config.GetSkillLoader()
	if loader == nil {
		t.Fatal("GetSkillLoader() should not return nil when WithSkillsFS is used")
	}

	skills := loader.AllSkillMetas()
	if len(skills) == 0 {
		t.Fatal("Expected at least one skill to be loaded")
	}

	// Check that manual-skill is present
	var foundManual, foundUnwanted bool
	for _, skill := range skills {
		if skill.Name == "manual-skill" {
			foundManual = true
		}
		if skill.Name == "unwanted-skill" {
			foundUnwanted = true
		}
	}

	if !foundManual {
		t.Error("manual-skill should be loaded (added via WithSkillsFS)")
	}
	if foundUnwanted {
		t.Error("unwanted-skill should NOT be loaded (auto-loading was disabled)")
	}

	t.Logf("Correctly loaded %d skill(s) (manual only, auto disabled)", len(skills))
}

// isolateAISkillsLookup 把当前测试中所有 well-known AI skills 扫描目录都隔离到 tempDir
//
// 背景：consts.GetAllAISkillsDirs 不仅返回 $YAKIT_HOME/ai-skills，还会扫描：
//   - $HOME/.cursor/skills、$HOME/.claude/skills、$HOME/.copilot/skills、$HOME/.opencode/skills
//   - $CWD/.cursor/skills、$CWD/.claude/skills、$CWD/.github/skills、$CWD/.opencode/skills
//
// 任意一个目录在开发者机器上存在（典型场景：Cursor / Claude 用户）就会让"empty/nonexistent
// ai-skills"测试发现非零 skill，造成 CI 通过、本地失败的不稳定行为。
//
// 这里用 t.Setenv 覆盖 YAKIT_HOME 与 HOME 指向 tempDir，并 chdir 到 tempDir，
// 让所有 well-known 子路径都解析到 tempDir 下不存在的位置。
//
// 关键词: 测试隔离, GetAllAISkillsDirs, $HOME / $CWD / YAKIT_HOME 隔离
func isolateAISkillsLookup(t *testing.T, tempDir string) {
	t.Helper()
	t.Setenv("YAKIT_HOME", tempDir)
	t.Setenv("HOME", tempDir)

	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to chdir to tempDir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origCwd)
	})

	consts.ResetYakitHomeOnce()
}

// TestConfig_AutoLoadEmptyDirectory tests that auto-loading works gracefully
// when the ai-skills directory exists but is empty.
//
// 关键词: AutoSkillLoader 空目录, 测试稳定性, ai-skills 隔离
func TestConfig_AutoLoadEmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")

	if err := os.MkdirAll(aiSkillsDir, 0755); err != nil {
		t.Fatalf("failed to create empty ai-skills directory: %v", err)
	}

	isolateAISkillsLookup(t, tempDir)

	config := NewConfig(context.Background())

	// Verify that skillLoader is nil when no skills are found
	loader := config.GetSkillLoader()
	if loader != nil {
		skills := loader.AllSkillMetas()
		if len(skills) > 0 {
			t.Errorf("Expected no skills in empty directory, got %d", len(skills))
		}
	}

	t.Log("Correctly handled empty ai-skills directory")
}

// TestConfig_AutoLoadNonexistentDirectory tests that auto-loading works gracefully
// when the ai-skills directory does not exist.
//
// 关键词: AutoSkillLoader 不存在目录, 测试稳定性, ai-skills 隔离
func TestConfig_AutoLoadNonexistentDirectory(t *testing.T) {
	tempDir := t.TempDir()

	isolateAISkillsLookup(t, tempDir)

	config := NewConfig(context.Background())

	loader := config.GetSkillLoader()
	if loader != nil {
		skills := loader.AllSkillMetas()
		if len(skills) > 0 {
			t.Errorf("Expected no skills when directory doesn't exist, got %d", len(skills))
		}
	}

	t.Log("Correctly handled nonexistent ai-skills directory")
}
