package aiskillloader

import (
	"strings"
	"testing"
)

// forced_dup_render_test.go 验证「单一技能块」不重复渲染。
//
// 改造前的旧问题: forced skill 同时出现在 frozen_block 与 SKILLS_CONTEXT。
// 改造后 forced/auto 与普通加载共用一条 Timeline 路径, 不存在第二个渲染块,
// 因此重复渲染在结构上不可能发生。

func TestSkillBodyRenderedOnceInSingleBlock(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)
	mgr.BindTimelineSink(newTestTimelineSink())

	if _, err := mgr.LoadForcedSkill("deploy-app"); err != nil {
		t.Fatalf("LoadForcedSkill failed: %v", err)
	}

	rendered := mgr.RenderStable()
	if rendered == "" {
		t.Fatal("RenderStable should render the shared skill block")
	}
	skillMDBody := "Run `scripts/deploy.sh`"
	if count := strings.Count(rendered, skillMDBody); count != 1 {
		t.Fatalf("skill body must appear exactly once, got %d\n%s", count, rendered)
	}
}

// forced 与 auto 同时存在时, 两个技能正文各出现一次, 且都在同一个块里。
func TestForcedAndAutoShareSingleBlock(t *testing.T) {
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
	if _, err := mgr.LoadAutoSkill("code-review"); err != nil {
		t.Fatalf("LoadAutoSkill: %v", err)
	}

	rendered := mgr.RenderStable()

	forcedBody := "Run `scripts/deploy.sh`"
	autoBody := "Use linters"
	if count := strings.Count(rendered, forcedBody); count != 1 {
		t.Fatalf("forced skill body must appear exactly once, got %d\n%s", count, rendered)
	}
	if count := strings.Count(rendered, autoBody); count != 1 {
		t.Fatalf("auto skill body must appear exactly once, got %d\n%s", count, rendered)
	}
}
