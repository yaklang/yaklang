package aireact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/consts"
)

func TestReAct_RefreshSkillsForTaskStart_PicksUpNewSkillInInstallDir(t *testing.T) {
	tempDir := t.TempDir()
	aiSkillsDir := filepath.Join(tempDir, "ai-skills")
	if err := os.MkdirAll(aiSkillsDir, 0o755); err != nil {
		t.Fatalf("failed to create ai-skills dir: %v", err)
	}

	originalYakitHome := os.Getenv("YAKIT_HOME")
	os.Setenv("YAKIT_HOME", tempDir)
	defer os.Setenv("YAKIT_HOME", originalYakitHome)
	consts.ResetYakitHomeOnce()
	defer consts.ResetYakitHomeOnce()

	react, err := NewReAct(
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
	)
	if err != nil {
		t.Fatalf("NewReAct failed: %v", err)
	}

	loader := react.config.GetSkillLoader()
	if loader == nil {
		t.Fatal("expected skill loader to be initialized")
	}

	lateSkillName := "late-added-skill"
	for _, meta := range loader.AllSkillMetas() {
		if meta.Name == lateSkillName {
			t.Fatalf("skill %q should not exist before test setup", lateSkillName)
		}
	}

	lateSkillDir := filepath.Join(aiSkillsDir, lateSkillName)
	if err := os.MkdirAll(lateSkillDir, 0o755); err != nil {
		t.Fatalf("failed to create late skill dir: %v", err)
	}
	skillContent := `---
name: late-added-skill
description: Skill added after ReAct startup
---
# Late Added Skill

This skill should be discovered on task start refresh.
`
	if err := os.WriteFile(filepath.Join(lateSkillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatalf("failed to write late skill SKILL.md: %v", err)
	}

	react.refreshSkillsForTaskStart()

	found := false
	for _, meta := range loader.AllSkillMetas() {
		if meta.Name == lateSkillName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected late-added skill %q to be discovered after task-start refresh", lateSkillName)
	}

	if _, err := loader.LoadSkill(lateSkillName); err != nil {
		t.Fatalf("LoadSkill(%q) failed after refresh: %v", lateSkillName, err)
	}
}

func TestReAct_RefreshSkillsForTaskStart_NoLoaderIsSafe(t *testing.T) {
	react := &ReAct{}
	react.refreshSkillsForTaskStart()

	react = &ReAct{config: aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))}
	react.refreshSkillsForTaskStart()
}
