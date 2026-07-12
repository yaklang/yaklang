package aiskillloader

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

// forced_skills_test.go 验证 ForcedSkillRegistry + 三态分离 (forced/catalog/auto).

func TestForcedSkillRegistry_AddRemoveRender(t *testing.T) {
	reg := NewForcedSkillRegistry()
	if !reg.IsEmpty() {
		t.Fatal("new registry should be empty")
	}
	if reg.Render() != "" {
		t.Fatal("empty registry should render empty string")
	}

	// 构造一个 LoadedSkill.
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SKILL.md", "---\nname: test-skill\ndescription: a test\n---\n# Body\nHello")
	loaded := &LoadedSkill{
		Meta:           &SkillMeta{Name: "test-skill", Description: "a test"},
		FileSystem:     vfs,
		SkillMDContent: "# Body\nHello",
	}

	added := reg.Add("test-skill", loaded)
	if !added {
		t.Fatal("first Add should return true (new)")
	}
	if reg.IsEmpty() {
		t.Fatal("registry should not be empty after add")
	}
	if !reg.Has("test-skill") {
		t.Fatal("Has should return true")
	}

	// 重复 Add → false.
	if reg.Add("test-skill", loaded) {
		t.Fatal("second Add of same name should return false")
	}

	// Render 应包含满内容 + USER_FORCED_SKILL 边界.
	rendered := reg.Render()
	if !strings.Contains(rendered, "USER_FORCED_SKILL") {
		t.Fatalf("render should contain USER_FORCED_SKILL boundary. Got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "=== Skill: test-skill ===") {
		t.Fatalf("render should contain skill header. Got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "# Body") {
		t.Fatalf("render should contain full SKILL.md body. Got:\n%s", rendered)
	}

	// Names.
	names := reg.Names()
	if len(names) != 1 || names[0] != "test-skill" {
		t.Fatalf("Names should be [test-skill], got %v", names)
	}

	// Remove.
	if !reg.Remove("test-skill") {
		t.Fatal("Remove should return true")
	}
	if !reg.IsEmpty() {
		t.Fatal("registry should be empty after remove")
	}
}

// TestSkillsContextManager_ForcedSkill_LoadAndRender 验证 LoadForcedSkill 把满内容
// 登记进 forced 容器, RenderForcedSkills 输出, 同时 loadedSkills 也登记.
func TestSkillsContextManager_ForcedSkill_LoadAndRender(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	added, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadForcedSkill failed: %v", err)
	}
	if !added {
		t.Fatal("first LoadForcedSkill should return added=true")
	}
	if !mgr.HasForcedSkills() {
		t.Fatal("HasForcedSkills should be true after load")
	}
	if !mgr.IsForcedSkill("deploy-app") {
		t.Fatal("IsForcedSkill should be true")
	}
	// loadedSkills 也应登记 (IsSkillLoaded 一致性).
	if !mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("forced skill should also register in loadedSkills for IsSkillLoaded consistency")
	}

	forced := mgr.RenderForcedSkills()
	if !strings.Contains(forced, "USER_FORCED_SKILL") {
		t.Fatalf("RenderForcedSkills should contain boundary. Got:\n%s", forced)
	}
	if !strings.Contains(forced, "deploy-app") {
		t.Fatalf("RenderForcedSkills should contain skill name. Got:\n%s", forced)
	}

	// 重复 LoadForcedSkill → added=false (幂等).
	added2, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("second LoadForcedSkill err: %v", err)
	}
	if added2 {
		t.Fatal("second LoadForcedSkill should return added=false (idempotent)")
	}

	// UnloadForcedSkill.
	if !mgr.UnloadForcedSkill("deploy-app") {
		t.Fatal("UnloadForcedSkill should return true")
	}
	if mgr.HasForcedSkills() {
		t.Fatal("HasForcedSkills should be false after unload")
	}
}

// TestSkillsContextManager_AutoSkill_LoadAndRender 验证 LoadAutoSkill 进入 autoLoadedSkills,
// RenderAutoLoadedSkills 输出 AUTO_LOADED_SKILLS 段; 且 forced 优先短路.
func TestSkillsContextManager_AutoSkill_LoadAndRender(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	added, err := mgr.LoadAutoSkill("code-review")
	if err != nil {
		t.Fatalf("LoadAutoSkill failed: %v", err)
	}
	if !added {
		t.Fatal("first LoadAutoSkill should return added=true")
	}
	if !mgr.HasAutoLoadedSkills() {
		t.Fatal("HasAutoLoadedSkills should be true")
	}
	if !mgr.IsAutoSkillLoadedAndUnfolded("code-review") {
		t.Fatal("IsAutoSkillLoadedAndUnfolded should be true")
	}

	auto := mgr.RenderAutoLoadedSkills()
	if !strings.Contains(auto, "AUTO_LOADED_SKILLS") {
		t.Fatalf("RenderAutoLoadedSkills should contain boundary. Got:\n%s", auto)
	}
	if !strings.Contains(auto, "code-review") {
		t.Fatalf("RenderAutoLoadedSkills should contain skill name. Got:\n%s", auto)
	}

	// 重复 LoadAutoSkill → added=false.
	added2, _ := mgr.LoadAutoSkill("code-review")
	if added2 {
		t.Fatal("second LoadAutoSkill should return added=false (idempotent)")
	}
}

// TestSkillsContextManager_ForcePreemptsAuto 验证 forced skill 优先: 若已 forced,
// LoadAutoSkill 短路 (added=false), IsAutoSkillLoadedAndUnfolded 也返回 true.
func TestSkillsContextManager_ForcePreemptsAuto(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("LoadForcedSkill: %v", err)
	}
	// 对同名 skill 调 LoadAutoSkill 应短路.
	added, err := mgr.LoadAutoSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadAutoSkill on forced skill err: %v", err)
	}
	if added {
		t.Fatal("LoadAutoSkill on already-forced skill should short-circuit (added=false)")
	}
	if !mgr.IsAutoSkillLoadedAndUnfolded("deploy-app") {
		t.Fatal("IsAutoSkillLoadedAndUnfolded should be true for forced skill (forced preempts)")
	}
}
