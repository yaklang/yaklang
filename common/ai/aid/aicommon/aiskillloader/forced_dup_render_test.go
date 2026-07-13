package aiskillloader

import (
	"strings"
	"testing"
)

// TestForcedSkill_DuplicateRenderInSkillsContext 验证 forced skill 是否同时出现在
// frozen_block (USER_FORCED_SKILL) 和 SKILLS_CONTEXT (Currently Loaded Skills) 两处。
//
// 预期行为: forced skill 满内容应只在 frozen_block 出现一次, 不应在 SKILLS_CONTEXT 中重复。
// 若测试失败, 说明存在 prompt 内容重复问题。
func TestForcedSkill_DuplicateRenderInSkillsContext(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	// 加载一个 forced skill
	added, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadForcedSkill failed: %v", err)
	}
	if !added {
		t.Fatal("LoadForcedSkill should return added=true")
	}

	// 渲染 forced 段 (frozen_block)
	forcedRender := mgr.RenderForcedSkills()
	if forcedRender == "" {
		t.Fatal("RenderForcedSkills should not be empty")
	}

	// 渲染 SKILLS_CONTEXT 段 (SemiDynamic 1)
	skillsContext := mgr.RenderStable()

	// 检查 SKILL.md body 是否在 SKILLS_CONTEXT 中出现
	// body 是 "# Deploy App\n\nRun `scripts/deploy.sh`.\n"
	skillMDBody := "Run `scripts/deploy.sh`"

	forcedHasBody := strings.Contains(forcedRender, skillMDBody)
	contextHasBody := strings.Contains(skillsContext, skillMDBody)

	t.Logf("forcedRender:\n%s", forcedRender)
	t.Logf("skillsContext:\n%s", skillsContext)
	t.Logf("forcedHasBody=%v, contextHasBody=%v", forcedHasBody, contextHasBody)

	if !forcedHasBody {
		t.Fatal("forced render should contain the SKILL.md body")
	}

	if contextHasBody {
		t.Fatal("BUG: SKILLS_CONTEXT also contains the SKILL.md body — forced skill content is duplicated in prompt (frozen_block + SKILLS_CONTEXT)")
	}
}

// TestForcedAndAuto_NoCrossDuplicate 验证 forced skill 和 auto skill 同时存在时,
// 不会互相重复渲染。
func TestForcedAndAuto_NoCrossDuplicate(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	// deploy-app → forced (frozen_block)
	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("LoadForcedSkill: %v", err)
	}
	// code-review → auto (SemiDynamic 2)
	if _, err := mgr.LoadAutoSkill("code-review"); err != nil {
		t.Fatalf("LoadAutoSkill: %v", err)
	}

	forced := mgr.RenderForcedSkills()
	auto := mgr.RenderAutoLoadedSkills()
	skillsContext := mgr.RenderStable()

	// forced skill body 只应出现在 forced 段
	forcedBody := "Run `scripts/deploy.sh`"
	// auto skill body 只应出现在 auto 段
	autoBody := "Use linters"

	t.Logf("=== forced ===\n%s", forced)
	t.Logf("=== auto ===\n%s", auto)
	t.Logf("=== skillsContext ===\n%s", skillsContext)

	// forced body 不应出现在 auto 段
	if strings.Contains(auto, forcedBody) {
		t.Fatal("BUG: forced skill body leaked into AUTO_LOADED_SKILLS section")
	}
	// auto body 不应出现在 forced 段
	if strings.Contains(forced, autoBody) {
		t.Fatal("BUG: auto skill body leaked into USER_FORCED_SKILL section")
	}
	// forced body 不应出现在 SKILLS_CONTEXT
	if strings.Contains(skillsContext, forcedBody) {
		t.Fatal("BUG: forced skill body duplicated in SKILLS_CONTEXT")
	}
	// auto body 不应出现在 SKILLS_CONTEXT (auto 进 SemiDynamic 2, 不进 SemiDynamic 1)
	if strings.Contains(skillsContext, autoBody) {
		t.Fatal("BUG: auto skill body duplicated in SKILLS_CONTEXT")
	}
}
