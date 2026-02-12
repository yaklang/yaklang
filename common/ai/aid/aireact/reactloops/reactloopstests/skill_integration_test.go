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
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// =====================================================================
// Prompt classification helpers
// =====================================================================
//
// ReActLoop issues multiple types of AI calls, each with a distinct prompt.
// Mock callbacks MUST correctly classify the prompt to return the right response.
//
// Priority order (first match wins):
//   1. verify-satisfaction  — "verify-satisfaction" + "user_satisfied" + "reasoning"
//   2. self-reflection      — "SELF_REFLECTION_TASK"
//   3. call-tool params     — "You need to generate parameters for the tool" + "call-tool"
//   4. main ReAct prompt    — "directly_answer" + "SCHEMA_" + "USER_QUERY"
//   5. unknown / fallback
//
// The main ReAct prompt is the only one that contains the action schema
// (including loading_skills when skills are configured).
//
// Markers were chosen by observing actual prompt content:
//   - "directly_answer": always in the main prompt action schema
//   - "SCHEMA_":         nonce-tagged <|SCHEMA_{nonce}|> block, main prompt only
//   - "USER_QUERY":      nonce-tagged <|USER_QUERY_{nonce}|> block, main prompt only
//   (Note: verify-satisfaction uses "USER_ORIGINAL_QUERY_" instead of "USER_QUERY")

// promptType enumerates the known prompt types issued by ReActLoop.
type promptType int

const (
	promptUnknown promptType = iota
	promptMainReAct
	promptVerifySatisfaction
	promptSelfReflection
	promptCallToolParams
)

// classifyPrompt determines the type of an AI prompt from its content.
// This uses multiple fixed markers per type for stability.
func classifyPrompt(prompt string) promptType {
	// 1. verify-satisfaction: requires all three markers
	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		return promptVerifySatisfaction
	}

	// 2. self-reflection: unique tag never present elsewhere
	if strings.Contains(prompt, "SELF_REFLECTION_TASK") {
		return promptSelfReflection
	}

	// 3. call-tool params: two markers that uniquely identify it
	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		return promptCallToolParams
	}

	// 4. main ReAct prompt: three markers that uniquely and stably identify it
	//    - "directly_answer": always present as an action type in the schema
	//    - "SCHEMA_": nonce-tagged schema block unique to main prompt
	//    - "USER_QUERY": nonce-tagged user query block unique to main prompt
	if utils.MatchAllOfSubString(prompt, "directly_answer", "SCHEMA_", "USER_QUERY") {
		return promptMainReAct
	}

	return promptUnknown
}

// =====================================================================
// Standard response builders
// =====================================================================

func makeVerifySatisfactionResponse(i aicommon.AICallerConfigIf) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(
		`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "OK", "human_readable_result": "done"}`,
	))
	rsp.Close()
	return rsp, nil
}

func makeFinishResponse(i aicommon.AICallerConfigIf) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
	rsp.Close()
	return rsp, nil
}

func makeLoadSkillResponse(i aicommon.AICallerConfigIf, skillName string) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "loading_skills", "skill_name": "` + skillName + `"}`))
	rsp.Close()
	return rsp, nil
}

// handleNonMainPrompt handles all non-main prompts with appropriate default responses.
// Returns (response, true) if the prompt was handled, or (nil, false) if it's the main prompt.
func handleNonMainPrompt(prompt string, i aicommon.AICallerConfigIf) (*aicommon.AIResponse, error, bool) {
	switch classifyPrompt(prompt) {
	case promptVerifySatisfaction:
		rsp, err := makeVerifySatisfactionResponse(i)
		return rsp, err, true
	case promptSelfReflection:
		// self-reflection: return a simple positive reflection
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"action_is_effective": true, "should_continue": true, "suggestion": ""}`))
		rsp.Close()
		return rsp, nil, true
	case promptCallToolParams:
		// call-tool params: should not appear in skill tests (no tools), finish gracefully
		rsp, err := makeFinishResponse(i)
		return rsp, err, true
	case promptMainReAct:
		return nil, nil, false // caller handles main prompt
	default:
		// Unknown prompt: return finish to avoid hangs
		rsp, err := makeFinishResponse(i)
		return rsp, err, true
	}
}

// ============ Test 1: Prompt Difference With/Without Skills ============
// Verifies that the main ReAct prompt differs when skills are configured vs not configured.

func TestReActLoop_Skills_PromptDifference(t *testing.T) {
	vfs := BuildTestSkillVFS()

	// Test 1a: Without skills - prompt should NOT contain loading_skills action
	t.Run("without_skills", func(t *testing.T) {
		var capturedMainPrompt string
		reactNoSkill, err := aireact.NewTestReAct(
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := req.GetPrompt()

				if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
					return rsp, err
				}

				// Main ReAct prompt
				capturedMainPrompt = prompt
				return makeFinishResponse(i)
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

		if capturedMainPrompt == "" {
			t.Fatal("main ReAct prompt was not captured (no prompt matched directly_answer+human_readable_thought+cumulative_summary)")
		}

		// Verify: loading_skills action should NOT be in action schema
		if strings.Contains(capturedMainPrompt, "loading_skills") {
			t.Error("loading_skills should NOT appear in prompt when no skills are configured")
		}
		// Verify: SKILLS_CONTEXT tag should NOT be present
		if strings.Contains(capturedMainPrompt, "SKILLS_CONTEXT") {
			t.Error("SKILLS_CONTEXT tag should NOT appear when no skills are configured")
		}
	})

	// Test 1b: With skills - prompt SHOULD contain loading_skills action and skill list
	t.Run("with_skills", func(t *testing.T) {
		var capturedMainPrompt string
		reactWithSkill := NewSkillTestReAct(t, vfs,
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := req.GetPrompt()

				if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
					return rsp, err
				}

				// Main ReAct prompt
				capturedMainPrompt = prompt
				return makeFinishResponse(i)
			}),
		)

		loop, err := reactloops.NewReActLoop("with-skill-test", reactWithSkill)
		if err != nil {
			t.Fatalf("failed to create loop: %v", err)
		}
		_ = loop.Execute("test", context.Background(), "test")

		if capturedMainPrompt == "" {
			t.Fatal("main ReAct prompt was not captured")
		}

		// Verify: loading_skills action SHOULD be in action schema
		if !strings.Contains(capturedMainPrompt, "loading_skills") {
			t.Error("loading_skills SHOULD appear in prompt when skills are configured")
		}
		// Verify: SKILLS_CONTEXT tag SHOULD be present
		if !strings.Contains(capturedMainPrompt, "SKILLS_CONTEXT") {
			t.Error("SKILLS_CONTEXT tag SHOULD be present when skills are configured")
		}
		// Verify: skill names appear in available skills list
		if !strings.Contains(capturedMainPrompt, "test-skill") {
			t.Error("'test-skill' SHOULD appear in available skills list")
		}
		if !strings.Contains(capturedMainPrompt, "test-lint-check") {
			t.Error("'test-lint-check' SHOULD appear in available skills list")
		}
	})
}

// ============ Test 2: Loading Skills Action ============
// Verifies that the loading_skills action can successfully load a skill.

func TestReActLoop_Skills_LoadingActionWorks(t *testing.T) {
	vfs := BuildTestSkillVFS()

	skillLoaded := false

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			// Main ReAct prompt: on first occurrence with loading_skills available, load the skill
			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "test-skill")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("skill-load-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load the test skill")

	// Verify skill was loaded via SkillsContextManager
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
// Verifies that loaded skill content appears in subsequent main prompts.

func TestReActLoop_Skills_LoadedContentInPrompt(t *testing.T) {
	vfs := BuildTestSkillVFS()

	skillLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			// Main ReAct prompt: track all main prompts
			mainPrompts = append(mainPrompts, prompt)

			// On the first main prompt: load skill
			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "test-skill")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("skill-content-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load skill")

	// mainPrompts[0] = before loading (available skills list only)
	// mainPrompts[1] = after loading (loaded skill content rendered)
	if len(mainPrompts) < 2 {
		t.Fatalf("Expected at least 2 main prompts, got %d", len(mainPrompts))
	}

	firstPrompt := mainPrompts[0]
	secondPrompt := mainPrompts[1]

	// First prompt: should have available skills list but NOT loaded content
	if !strings.Contains(firstPrompt, "test-skill") {
		t.Error("First main prompt should list 'test-skill' in available skills")
	}

	// Second prompt: should have the actual loaded skill body content
	if !strings.Contains(secondPrompt, "This is a test skill") {
		t.Error("Second main prompt should contain loaded skill body 'This is a test skill'")
	}
	if !strings.Contains(secondPrompt, "=== Skill: test-skill ===") {
		t.Error("Second main prompt should contain skill header '=== Skill: test-skill ==='")
	}

	t.Logf("Captured %d main prompts; verified skill content transition", len(mainPrompts))
}

// ============ Test 4: Change View Offset Action ============
// Verifies that loading a large skill works and truncation metadata appears in prompt.

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

	skillLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			// Main ReAct prompt
			mainPrompts = append(mainPrompts, prompt)

			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "large-skill")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("view-offset-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load large skill")

	// Verify the skill was loaded
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	skills := mgr.GetCurrentSelectedSkills()
	if len(skills) == 0 {
		t.Fatal("Expected at least one skill to be loaded")
	}
	if skills[0].Name != "large-skill" {
		t.Errorf("Expected 'large-skill', got '%s'", skills[0].Name)
	}

	// Verify the loaded prompt contains truncation metadata (Total Lines, Current Offset)
	if len(mainPrompts) >= 2 {
		loadedPrompt := mainPrompts[1]
		if !strings.Contains(loadedPrompt, "Total Lines") {
			t.Error("Loaded large skill prompt should contain 'Total Lines' metadata")
		}
		if !strings.Contains(loadedPrompt, "Current Offset") {
			t.Error("Loaded large skill prompt should contain 'Current Offset' metadata")
		}
	}

	t.Logf("Large skill loaded with %d main prompts captured", len(mainPrompts))
}

// ============ Test 5: Multiple Skills Loading ============
// Verifies that multiple skills can be loaded in sequence via main prompts.

func TestReActLoop_Skills_MultipleSkills(t *testing.T) {
	vfs := BuildTestSkillVFS()

	loadedSkills := make(map[string]bool)

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			// Main ReAct prompt: load skills in sequence
			if !loadedSkills["test-skill"] {
				loadedSkills["test-skill"] = true
				return makeLoadSkillResponse(i, "test-skill")
			}
			if !loadedSkills["test-lint-check"] {
				loadedSkills["test-lint-check"] = true
				return makeLoadSkillResponse(i, "test-lint-check")
			}

			return makeFinishResponse(i)
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
	if !skillNames["test-lint-check"] {
		t.Error("test-lint-check should be loaded")
	}

	t.Logf("Successfully loaded %d skills: %v", len(skills), skillNames)
}

// ============ Test 6: Action Schema Contains loading_skills ============
// Verifies that skill-related actions are registered in the loop at initialization.

func TestReActLoop_Skills_ActionInSchema(t *testing.T) {
	vfs := BuildTestSkillVFS()

	reactIns := NewSkillTestReAct(t, vfs)
	loop, err := reactloops.NewReActLoop("schema-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	actions := loop.GetAllActions()

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
// Verifies the behavior when no skills are available (empty VFS).

func TestReActLoop_Skills_NoSkillsAvailable(t *testing.T) {
	// Empty VFS - no skills
	vfs := filesys.NewVirtualFs()

	var capturedMainPrompt string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			// Main ReAct prompt
			capturedMainPrompt = prompt
			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("no-skills-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "test")

	if capturedMainPrompt == "" {
		t.Fatal("main ReAct prompt was not captured")
	}

	// When no skills discovered, loading_skills should NOT be in prompt
	if strings.Contains(capturedMainPrompt, "loading_skills") {
		t.Error("loading_skills should NOT appear when no skills are discovered")
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
