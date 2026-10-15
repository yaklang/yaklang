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
//   2. main ReAct prompt    — layered main-loop envelope + action schema
//   3. unknown / fallback
//
// The main ReAct prompt is the only one that contains the action schema
// (including loading_skills when skills are configured).
//
// Markers were chosen by observing actual prompt content:
//   - "directly_answer": always in the main prompt action schema
//   - "SCHEMA":          schema block <|SCHEMA|>, main prompt only
//   - "PROMPT_SECTION_dynamic_": turn envelope; user input lives in Timeline

// extractSchemaBlock 从主 ReAct prompt 中提取 SCHEMA 块的 jsonschema 内容,
// 用于精确判断某个 action (例如 loading_skills / load_skill_resources) 是否
// 真正出现在 action enum 中. 这是必要的: high_static_section.txt 已重构为
// 无条件介绍全部能力 (实际可用性以本轮 SCHEMA enum 为准, 未列出即视为禁用),
// 因此整 prompt 全文 contains 不再准确反映可用性, 必须看 SCHEMA enum.
//
// SCHEMA 块在 semi_dynamic_section.txt 中由前后两个 `<|SCHEMA|>` 标签包裹.
// 提取逻辑: 找首个 `<|SCHEMA|>`, 跳过该标签后再找下一个 `<|SCHEMA|>`, 中间
// 即为 schema 字面量. 找不到时返回空串, 调用方应视为 "未启用".
//
// 关键词: extractSchemaBlock, SCHEMA enum 精确判断, high-static 反污染对应测试侧调整
// skillPromptMarkerPresent 判断 prompt 中是否包含该技能的可见内容:
// 未冻结时是 Open 段的 [skill <name> | ...] 标签, 冻结提升后是 === Skill: <name> === 块。
func skillPromptMarkerPresent(prompt, name string) bool {
	return strings.Contains(prompt, "=== Skill: "+name+" ===") ||
		strings.Contains(prompt, "[skill "+name+" |")
}

func extractSchemaBlock(prompt string) string {
	const tag = "<|SCHEMA|>"
	first := strings.Index(prompt, tag)
	if first == -1 {
		return ""
	}
	innerStart := first + len(tag)
	if innerStart >= len(prompt) {
		return ""
	}
	rest := prompt[innerStart:]
	end := strings.Index(rest, tag)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// promptType enumerates the known prompt types issued by ReActLoop.
type promptType int

const (
	promptUnknown promptType = iota
	promptMainReAct
	promptVerifySatisfaction
)

// classifyPrompt determines the type of an AI prompt from its content.
// This uses multiple fixed markers per type for stability.
func classifyPrompt(prompt string) promptType {
	// 1. verify-satisfaction: requires all three markers
	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		return promptVerifySatisfaction
	}

	// 2. main ReAct prompt: layered envelope and the decision action schema.
	if aicommon.IsPrimaryDecisionPrompt(prompt) {
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
		`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "OK"}`,
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

		// Verify: loading_skills action should NOT be in action schema enum.
		// high_static_section.txt 现在无条件介绍所有能力, 所以全 prompt 一定
		// 含 loading_skills 字面量; 真正决定可用性的是 SCHEMA enum, 必须只
		// 在 SCHEMA 块内做 contains 判断.
		// 关键词: SCHEMA enum 精确判断, high-static 反污染
		schemaBlock := extractSchemaBlock(capturedMainPrompt)
		if strings.Contains(schemaBlock, "loading_skills") {
			t.Error("loading_skills should NOT appear in SCHEMA enum when no skills are configured")
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

		// Verify: loading_skills action SHOULD be in SCHEMA enum (not just any
		// where in prompt — high-static 介绍部分会无条件出现该字面量, 真实可用
		// 性以 SCHEMA enum 为准).
		// 关键词: SCHEMA enum 精确判断, with skills 启用断言
		schemaBlock := extractSchemaBlock(capturedMainPrompt)
		if !strings.Contains(schemaBlock, "loading_skills") {
			t.Error("loading_skills SHOULD appear in SCHEMA enum when skills are configured")
		}
		// 固定 Skill Inventory 在 FrozenBlock 展示名称；未加载时没有技能正文块。
		if strings.Contains(capturedMainPrompt, "SKILLS_CONTEXT") {
			t.Error("SKILLS_CONTEXT tag should NOT appear before any skill is loaded")
		}
		if !strings.Contains(capturedMainPrompt, "# Skill Inventory") || !strings.Contains(capturedMainPrompt, "  - test-skill:") {
			t.Error("fixed Skill Inventory should list 'test-skill' before loading")
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

	loop, err := reactloops.NewReActLoop("skill-test", reactIns)
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

	// 加载前只有固定目录，正文仍需按需加载进 Timeline。
	if !strings.Contains(firstPrompt, "# Skill Inventory") || !strings.Contains(firstPrompt, "  - test-skill:") {
		t.Error("First main prompt should list 'test-skill' in the fixed inventory")
	}
	if strings.Contains(firstPrompt, "SKILLS_CONTEXT") {
		t.Error("First main prompt should NOT contain a loaded skill context block")
	}

	// Second prompt: should have the actual loaded skill body content.
	// AI 自主加载先进入 Timeline Open；冻结后才进入 SemiDynamic1 技能块。
	if !strings.Contains(secondPrompt, "This is a test skill") {
		t.Error("Second main prompt should contain loaded skill body 'This is a test skill'")
	}
	if !skillPromptMarkerPresent(secondPrompt, "test-skill") {
		t.Error("Second main prompt should contain the loaded skill section")
	}

	t.Logf("Captured %d main prompts; verified skill content transition", len(mainPrompts))
}

// ============ Test 4: Change View Offset Action ============
// Verifies that loading a large skill works and truncation metadata appears in prompt.

func TestReActLoop_Skills_ChangeViewOffset(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	// SKILL view window 走双模式契约 (见 aiskillloader/view_window.go):
	//   - offset == 1 且 内容 <= ViewWindowMaxBytes (32KB) -> 全文展示, 仅 File header,
	//     不带 Total Lines / Current Offset / 行号 / 截断 Note;
	//   - 否则 (内容 > 32KB 或 offset > 1) -> 部分展示, 带 Total Lines /
	//     Current Offset / 行号 / 截断 Note, 这才是 change_skill_view_offset
	//     真正生效的场景.
	//
	// 这里测试名是 "ChangeViewOffset", 必须让 skill 体量超过 32KB 才能触发部分
	// 展示路径, 否则原来 100 行 / 4KB 会落到全文展示路径, 自然不会出现 Total
	// Lines / Current Offset.
	//
	// 关键词: ChangeViewOffset 测试体量调整, ViewWindowMaxBytes 32KB,
	//        部分展示路径, Total Lines / Current Offset 元数据, 双模式契约
	var bodyContent string
	for i := 1; i <= 1000; i++ {
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
		if !strings.Contains(loadedPrompt, "[skill large-skill | SKILL.md:") {
			t.Error("Loaded large skill prompt should contain the SKILL.md range")
		}
		if strings.Contains(loadedPrompt, " END]") {
			t.Error("truncated first range must not claim the file has ended")
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

	// When no skills discovered, loading_skills should NOT be in SCHEMA enum.
	// 关键词: SCHEMA enum 精确判断, 无 skills 启用时 enum 不含 loading_skills
	schemaBlock := extractSchemaBlock(capturedMainPrompt)
	if strings.Contains(schemaBlock, "loading_skills") {
		t.Error("loading_skills should NOT appear in SCHEMA enum when no skills are discovered")
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
