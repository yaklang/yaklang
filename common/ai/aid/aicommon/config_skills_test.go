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
// without WithDisableAutoSkills().
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

	// Create Config WITHOUT WithDisableAutoSkills()
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

// TestConfig_DisableAutoSkillsOption tests that WithDisableAutoSkills() prevents
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

	// Create Config WITH WithDisableAutoSkills()
	config := NewConfig(context.Background(), WithDisableAutoSkills())

	// Verify that NO skills were auto-loaded
	loader := config.GetSkillLoader()
	if loader != nil {
		t.Fatal("GetSkillLoader() should return nil when WithDisableAutoSkills() is used")
	}

	t.Log("WithDisableAutoSkills() correctly prevented auto-loading")
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
		WithDisableAutoSkills(),
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

// TestConfig_AutoLoadEmptyDirectory tests that auto-loading works gracefully
// when the ai-skills directory exists but is empty.
func TestConfig_AutoLoadEmptyDirectory(t *testing.T) {
	// Create a temporary directory with an empty ai-skills directory
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")

	err := os.MkdirAll(aiSkillsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create empty ai-skills directory: %v", err)
	}

	// Set YAKIT_HOME environment variable
	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)

	// Reset the yakit home cache
	consts.ResetYakitHomeOnce()

	// Create Config without WithDisableAutoSkills()
	config := NewConfig(context.Background())

	// Verify that skillLoader is nil when no skills are found
	loader := config.GetSkillLoader()
	if loader != nil {
		// If loader is not nil, it should have no skills
		skills := loader.AllSkillMetas()
		if len(skills) > 0 {
			t.Errorf("Expected no skills in empty directory, got %d", len(skills))
		}
	}

	t.Log("Correctly handled empty ai-skills directory")
}

// TestConfig_AutoLoadNonexistentDirectory tests that auto-loading works gracefully
// when the ai-skills directory does not exist.
func TestConfig_AutoLoadNonexistentDirectory(t *testing.T) {
	// Create a temporary directory WITHOUT ai-skills
	tempDir := t.TempDir()

	// Set YAKIT_HOME environment variable
	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)

	// Reset the yakit home cache
	consts.ResetYakitHomeOnce()

	// Create Config without WithDisableAutoSkills()
	config := NewConfig(context.Background())

	// Verify that skillLoader is nil when directory doesn't exist
	loader := config.GetSkillLoader()
	if loader != nil {
		// If loader is not nil, it should have no skills
		skills := loader.AllSkillMetas()
		if len(skills) > 0 {
			t.Errorf("Expected no skills when directory doesn't exist, got %d", len(skills))
		}
	}

	t.Log("Correctly handled nonexistent ai-skills directory")
}
