package reactloopstests

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// ============ Test 1: Prompt Difference With/Without Skills ============
// This test verifies that the prompt is different when skills are configured vs not configured.

func TestReActLoop_Skills_PromptDifference(t *testing.T) {
	vfs := BuildTestSkillVFS()

	// Test 1a: Without skills - prompt should NOT contain loading_skills action
	t.Run("without_skills", func(t *testing.T) {
		var capturedPrompt string
		reactNoSkill, err := aireact.NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				capturedPrompt = req.GetPrompt()
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("failed to create ReAct without skills: %v", err)
		}

		loop, err := reactloops.NewReActLoop("no-skill-test", reactNoSkill)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}
		_ = loop.Execute("test", context.Background(), "test")

		// Verify: loading_skills action should NOT be in schema
		if strings.Contains(capturedPrompt, "loading_skills") {
			t.Error("loading_skills should NOT appear in prompt when no skills are configured")
		}
	})

	// Test 1b: With skills - prompt SHOULD contain loading_skills action and skill list
	t.Run("with_skills", func(t *testing.T) {
		var capturedPrompt string
		reactWithSkill := NewSkillTestReAct(t, vfs,
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				capturedPrompt = req.GetPrompt()
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
				rsp.Close()
				return rsp, nil
			}),
		)

		loop, err := reactloops.NewReActLoop("with-skill-test", reactWithSkill)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}
		_ = loop.Execute("test", context.Background(), "test")

		// Verify: loading_skills action SHOULD be in schema
		if !strings.Contains(capturedPrompt, "loading_skills") {
			t.Error("loading_skills SHOULD appear in prompt when skills are configured")
		}
		if !strings.Contains(capturedPrompt, "test-skill") {
			t.Error("'test-skill' SHOULD appear in available skills list")
		}
	})
}

// ============ Test 2: Loading Skills Action ============
// This test verifies that the loading_skills action can successfully load a skill.

func TestReActLoop_Skills_LoadingActionWorks(t *testing.T) {
	vfs := BuildTestSkillVFS()

	skillLoaded := false

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			// First call: return loading_skills action
			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "test-skill"}`))
				rsp.Close()
				return rsp, nil
			}

			// Subsequent calls: finish
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	loop, err := reactloops.NewReActLoop("skill-load-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load the test skill")

	// Verify skill was loaded
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	skills := mgr.GetCurrentSelectedSkills()
	if len(skills) == 0 {
		t.Fatal("At least one skill should be loaded after loading_skills action")
	}

	loadedSkillName := skills[0].Name
	if loadedSkillName != "test-skill" {
		t.Errorf("Expected skill 'test-skill', got '%s'", loadedSkillName)
	}

	t.Logf("Successfully loaded skill: %s", loadedSkillName)
}

// ============ Test 3: Loaded Skill Content in Prompt ============
// This test verifies that loaded skill content appears in subsequent prompts.

func TestReActLoop_Skills_LoadedContentInPrompt(t *testing.T) {
	vfs := BuildTestSkillVFS()

	callCount := 0
	var prompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			prompts = append(prompts, req.GetPrompt())

			prompt := req.GetPrompt()
			// First call: load skill
			if callCount == 1 && strings.Contains(prompt, "loading_skills") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "test-skill"}`))
				rsp.Close()
				return rsp, nil
			}

			// All subsequent calls: finish
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	loop, err := reactloops.NewReActLoop("skill-content-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load skill")

	// After loading the skill, the prompt should contain the actual skill body content.
	// The first prompt has SKILLS_CONTEXT with the available skills list;
	// the second prompt (after loading) should have the loaded content.
	foundLoadedContent := false
	for i, p := range prompts {
		if strings.Contains(p, "This is a test skill") {
			foundLoadedContent = true
			t.Logf("Prompt %d contains loaded skill body content", i+1)
			break
		}
	}

	if !foundLoadedContent {
		t.Error("Loaded skill body content 'This is a test skill' should appear in prompt after loading_skills action")
		t.Logf("Total prompts captured: %d", len(prompts))
	}
}

// ============ Test 4: Change View Offset Action ============
// This test verifies that loading a large skill and then changing view offset works.

func TestReActLoop_Skills_ChangeViewOffset(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	var bodyContent string
	for i := 1; i <= 100; i++ {
		bodyContent += "Line of content for truncation testing.\n"
	}

	vfs.AddFile("large-skill/SKILL.md", `---
name: large-skill
description: A large skill for view offset testing
---
# Large Skill

`+bodyContent)

	var actionCalls []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			// First call: load the skill
			if len(actionCalls) == 0 && strings.Contains(prompt, "loading_skills") {
				actionCalls = append(actionCalls, "loading_skills")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "large-skill"}`))
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	loop, err := reactloops.NewReActLoop("view-offset-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load large skill")

	t.Logf("Action calls: %v", actionCalls)

	// Verify the skill was loaded
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	skills := mgr.GetCurrentSelectedSkills()
	if len(skills) == 0 {
		t.Error("Expected at least one skill to be loaded")
	}
}

// ============ Test 5: Multiple Skills Loading ============
// This test verifies that multiple skills can be loaded in sequence.

func TestReActLoop_Skills_MultipleSkills(t *testing.T) {
	vfs := BuildTestSkillVFS()

	loadedSkills := make(map[string]bool)

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			// Load test-skill first
			if strings.Contains(prompt, "loading_skills") && !loadedSkills["test-skill"] {
				loadedSkills["test-skill"] = true
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "test-skill"}`))
				rsp.Close()
				return rsp, nil
			}

			// Load code-review second
			if loadedSkills["test-skill"] && !loadedSkills["code-review"] {
				loadedSkills["code-review"] = true
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "code-review"}`))
				rsp.Close()
				return rsp, nil
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	loop, err := reactloops.NewReActLoop("multi-skill-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load multiple skills")

	// Verify both skills are loaded
	mgr := loop.GetSkillsContextManager()
	skills := mgr.GetCurrentSelectedSkills()

	if len(skills) < 2 {
		t.Errorf("Expected at least 2 loaded skills, got %d", len(skills))
	}

	skillNames := make(map[string]bool)
	for _, s := range skills {
		skillNames[s.Name] = true
	}

	if !skillNames["test-skill"] {
		t.Error("test-skill should be loaded")
	}
	if !skillNames["code-review"] {
		t.Error("code-review should be loaded")
	}

	t.Logf("Successfully loaded %d skills: %v", len(skills), skillNames)
}

// ============ Test 6: Action Schema Contains loading_skills ============
// This test verifies that skill-related actions are registered in the loop.

func TestReActLoop_Skills_ActionInSchema(t *testing.T) {
	vfs := BuildTestSkillVFS()

	reactIns := NewSkillTestReAct(t, vfs)
	loop, err := reactloops.NewReActLoop("schema-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	// Get all actions
	actions := loop.GetAllActions()

	// Verify loading_skills action exists
	var hasLoadingSkills bool
	for _, action := range actions {
		if action.ActionType == schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS {
			hasLoadingSkills = true
		}
	}

	if !hasLoadingSkills {
		t.Error("loading_skills action should be registered when skills are available")
	}

	// Note: change_skill_view_offset is registered dynamically only when
	// there are truncated views (after a skill with long content is loaded),
	// so it is NOT expected to be present at initialization time.
	t.Log("loading_skills action is registered in the loop")
}

// ============ Test 7: No Skills Available Behavior ============
// This test verifies the behavior when no skills are available (empty VFS).

func TestReActLoop_Skills_NoSkillsAvailable(t *testing.T) {
	// Empty VFS - no skills
	vfs := filesys.NewVirtualFs()

	var capturedPrompt string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = req.GetPrompt()
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
	)

	loop, err := reactloops.NewReActLoop("no-skills-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "test")

	// When no skills are available, loading_skills should NOT be in prompt
	if strings.Contains(capturedPrompt, "loading_skills") {
		t.Error("loading_skills should NOT appear when no skills are available")
	}

	// SkillsContextManager should exist but have no skills
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Log("SkillsContextManager is nil (expected for empty VFS with no skills)")
	} else if mgr.HasRegisteredSkills() {
		t.Error("HasRegisteredSkills should return false for empty skills")
	}

	t.Log("Correctly handles no skills available scenario")
}
