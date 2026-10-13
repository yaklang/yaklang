package aiskillloader

import (
	"strings"
	"testing"
)

// forced_skills_test.go 验证「加载来源」收敛为 item 上的一个标记。
//
// 改造后不再有 ForcedSkillRegistry / 独立渲染块 / 独立压缩保护:
//   - 三条加载路径 (manual / forced / auto) 写入同一 Timeline 类型;
//   - 差异只在 payload 的 source 字段;
//   - 冻结后统一进入同一个技能块。

func TestForcedSkill_MarksSourceOnly(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)

	added, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadForcedSkill failed: %v", err)
	}
	if !added {
		t.Fatal("first LoadForcedSkill should report newly loaded")
	}
	for _, item := range sink.items {
		if item.src != SkillContentSourceForced {
			t.Fatalf("each forced item must carry source=%q, got %q", SkillContentSourceForced, item.src)
		}
	}
	if !mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("forced skill must be visible as loaded")
	}

	// 全文进入同一个技能块, 而不是独立的 USER_FORCED_SKILL 段。
	rendered := mgr.RenderStable()
	if !strings.Contains(rendered, "deploy-app") {
		t.Fatalf("forced skill body must render in the shared skill block. Got:\n%s", rendered)
	}
	if strings.Contains(rendered, "USER_FORCED_SKILL") {
		t.Fatalf("dedicated USER_FORCED_SKILL block must be gone. Got:\n%s", rendered)
	}

	// 幂等: 已可见不再重复追加。
	appendsBefore := sink.appendCount()
	added2, err := mgr.LoadForcedSkill("deploy-app")
	if err != nil {
		t.Fatalf("second LoadForcedSkill err: %v", err)
	}
	if added2 {
		t.Fatal("second LoadForcedSkill should report already visible")
	}
	if sink.appendCount() != appendsBefore {
		t.Fatal("already visible skill must not be re-appended")
	}
}

// 重复加载自动技能幂等, 且与普通加载同构 (没有 AUTO_LOADED_SKILLS 段)。
func TestAutoSkill_SameAsManual(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	mgr.BindTimelineSink(newTestTimelineSink())

	added, err := mgr.LoadAutoSkill("code-review")
	if err != nil {
		t.Fatalf("LoadAutoSkill failed: %v", err)
	}
	if !added {
		t.Fatal("first LoadAutoSkill should report newly loaded")
	}
	if !mgr.HasAutoLoadedSkills() {
		t.Fatal("HasAutoLoadedSkills should be true after any load")
	}
	if !mgr.IsAutoSkillLoadedAndUnfolded("code-review") {
		t.Fatal("IsAutoSkillLoadedAndUnfolded should be true")
	}

	rendered := mgr.RenderStable()
	if strings.Contains(rendered, "AUTO_LOADED_SKILLS") {
		t.Fatalf("dedicated AUTO_LOADED_SKILLS block must be gone. Got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "code-review") {
		t.Fatalf("auto skill body must render in the shared skill block. Got:\n%s", rendered)
	}

	added2, _ := mgr.LoadAutoSkill("code-review")
	if added2 {
		t.Fatal("second LoadAutoSkill should report already visible")
	}
}

// 同一技能从任意来源加载后, 相同正文不会再次追加或改写已有 item 的来源。
func TestForcedThenAuto_NoDuplicateBody(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	sink := newTestTimelineSink()
	mgr.BindTimelineSink(sink)

	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("LoadForcedSkill: %v", err)
	}
	added, err := mgr.LoadAutoSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadAutoSkill on visible skill err: %v", err)
	}
	if added {
		t.Fatal("loading an already-visible skill must not report new content")
	}
	if !mgr.IsAutoSkillLoadedAndUnfolded("deploy-app") {
		t.Fatal("visible skill must satisfy the auto-loaded check")
	}
	for _, item := range sink.items {
		if item.src != SkillContentSourceForced {
			t.Fatal("the original item source marker must be preserved")
		}
	}

	bodyRenders := strings.Count(mgr.RenderStable(), "Run `scripts/deploy.sh`")
	if bodyRenders != 1 {
		t.Fatalf("skill body must appear exactly once, got %d\n%s", bodyRenders, mgr.RenderStable())
	}
}

// 显式移除只退休 Timeline item: 之后再次加载会重新追加一份新正文。
func TestUnloadRetiresTimelineItem(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	mgr.BindTimelineSink(newTestTimelineSink())

	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("LoadForcedSkill: %v", err)
	}
	if !mgr.UnloadSkill("deploy-app") {
		t.Fatal("UnloadSkill should retire the timeline items")
	}
	if mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("unloaded skill must not be visible")
	}
	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("reload after unload failed: %v", err)
	}
	if !mgr.IsSkillLoaded("deploy-app") {
		t.Fatal("reloaded skill must be visible again")
	}
}
