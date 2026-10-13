package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// buildSkillMD creates a valid SKILL.md content for testing.
func buildSkillMD(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body
}

// buildSkillsVFS builds a VirtualFS with multiple skills for integration testing.
func buildSkillsVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("deploy-app/SKILL.md", buildSkillMD(
		"deploy-app",
		"Deploy the application to staging or production.",
		"# Deploy App\n\nStep 1: build.\nStep 2: deploy.\n",
	))
	vfs.AddFile("deploy-app/scripts/deploy.sh", "#!/bin/bash\necho deploy")
	vfs.AddFile("code-review/SKILL.md", buildSkillMD(
		"code-review",
		"Perform an automated code review.",
		"# Code Review\n\nUse linters and static analysis.\n",
	))
	vfs.AddFile("code-review/rules/RULES.md", "# Rules\nAlways lint.")
	return vfs
}

// --- Integration: SkillsContextManager end-to-end ---

func TestSkillsContextManager_Integration_FullLifecycle(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	// Step 1: before loading, manager should still detect available skills
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should detect registered skills from loader")
	}
	if mgr.HasTruncatedViews() {
		t.Fatal("no truncated views before loading any skill")
	}

	// Step 2: render without loading -> catalog hidden by default → empty (save tokens).
	rendered := mgr.Render("lifecycle_nonce")
	if rendered != "" {
		t.Fatalf("default render with hidden catalog should be empty. Got:\n%s", rendered)
	}
	// Surface catalog (simulating intent recognition): 目录先进入 Timeline Open。
	mgr.SetCatalogSkills(loader.AllSkillMetas())
	require.Contains(t, RenderTimelineFrozenOpen(cfg.GetTimeline()).Open, "deploy-app")
	require.Empty(t, mgr.Render("lifecycle_nonce"), "未冻结内容不得提前进入可缓存技能块")

	// Step 3: load a skill。正文同样先进入 Open 尾段。
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	require.Contains(t, RenderTimelineFrozenOpen(cfg.GetTimeline()).Open, "VIEW_WINDOW")

	// Step 4: 由既有冻结事务提交后, 正文才提升进单一技能块。
	cfg.GetTimeline().FreezeAll()
	rendered = mgr.Render("lifecycle_nonce")
	require.Contains(t, rendered, "<|SKILLS_CONTEXT_")
	require.Contains(t, rendered, "=== Skill: deploy-app ===")
	require.Contains(t, rendered, "VIEW_WINDOW")
	require.Contains(t, rendered, "deploy-app")
	require.Contains(t, rendered, "code-review")

	// Step 5: 加载第二个技能, 再次冻结后两个技能共处同一个块。
	if err := mgr.LoadSkill("code-review"); err != nil {
		t.Fatalf("LoadSkill for code-review failed: %v", err)
	}
	cfg.GetTimeline().FreezeAll()
	rendered = mgr.Render("lifecycle_nonce")
	require.Contains(t, rendered, "=== Skill: code-review ===")

	// Step 6: 翻页产生一条新的阅读窗口 item。
	if err := mgr.ChangeViewOffset("deploy-app", "", 2); err != nil {
		t.Fatalf("ChangeViewOffset failed: %v", err)
	}
	cfg.GetTimeline().FreezeAll()
	rendered = mgr.Render("lifecycle_nonce")
	require.Contains(t, rendered, "File: SKILL.md (Skill: deploy-app)")
}

// --- Integration: AISkillContextProvider safe usage ---

func TestAISkillContextProvider_SafeUsage(t *testing.T) {
	// AISkillContextProvider should never panic regardless of inputs
	provider := AISkillContextProvider("test-skill", "help me deploy")
	result, err := provider(nil, nil, "test_key")
	if err != nil {
		t.Fatalf("AISkillContextProvider should not return error: %v", err)
	}
	if !strings.Contains(result, "test-skill") {
		t.Fatal("provider result should contain skill name")
	}
	if !strings.Contains(result, "loading_skills") {
		t.Fatal("provider result should instruct to use loading_skills action")
	}
}

func TestAISkillContextProvider_EmptySkillName(t *testing.T) {
	provider := AISkillContextProvider("")
	result, err := provider(nil, nil, "key")
	if err != nil {
		t.Fatalf("empty skill name should not cause error: %v", err)
	}
	if result == "" {
		t.Fatal("result should not be empty even with empty skill name")
	}
}

func TestAISkillContextProvider_NilConfig(t *testing.T) {
	// Should not panic with nil config and nil emitter
	provider := AISkillContextProvider("my-skill")
	result, err := provider(nil, nil, "")
	if err != nil {
		t.Fatalf("should not error with nil config: %v", err)
	}
	if result == "" {
		t.Fatal("result should not be empty")
	}
}

// --- Integration: NewContextProvider for aiskill type ---

func TestNewContextProvider_AISkill_ByName(t *testing.T) {
	provider := NewContextProvider(
		CONTEXT_PROVIDER_TYPE_AISKILL,
		CONTEXT_PROVIDER_KEY_NAME,
		"deploy-app",
		"deploy the thing",
	)
	result, err := provider(nil, nil, "provider_key")
	if err != nil {
		t.Fatalf("NewContextProvider for aiskill should not error: %v", err)
	}
	if !strings.Contains(result, "deploy-app") {
		t.Fatal("result should contain skill name")
	}
}

func TestNewContextProvider_AISkill_UnknownKey(t *testing.T) {
	provider := NewContextProvider(
		CONTEXT_PROVIDER_TYPE_AISKILL,
		"unknown_key",
		"value",
	)
	result, err := provider(nil, nil, "provider_key")
	if err == nil {
		t.Fatal("unknown key should return error")
	}
	if !strings.Contains(result, "Error") {
		t.Fatal("error result should contain error indicator")
	}
}

// --- Integration: SkillsContextManager nil-safe operations ---

func TestSkillsContextManager_NilLoader_Safe(t *testing.T) {
	mgr := aiskillloader.NewSkillsContextManager(nil)

	// None of these should panic
	if mgr.HasRegisteredSkills() {
		t.Fatal("nil loader should have no skills")
	}
	if mgr.HasTruncatedViews() {
		t.Fatal("nil loader should have no truncated views")
	}

	err := mgr.LoadSkill("anything")
	if err == nil {
		t.Fatal("LoadSkill with nil loader should error")
	}

	rendered := mgr.Render("nonce")
	if rendered != "" {
		t.Fatalf("render with nil loader should be empty, got %q", rendered)
	}
}

// --- Integration: SkillsContextManager folding behavior ---

func TestSkillsContextManager_FoldingPreservesPromptIntegrity(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	// manager 侧不再有独立折叠预算: 正文体积由 Timeline 统一压缩控制。
	mgr.SetMaxTokens(50)

	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("code-review")
	cfg.GetTimeline().FreezeAll()

	rendered := mgr.Render("fold_nonce")

	if !strings.Contains(rendered, "SKILLS_CONTEXT_") {
		t.Fatal("render must have start tag")
	}
	if !strings.Contains(rendered, "SKILLS_CONTEXT_END_") {
		t.Fatal("render must have end tag")
	}
	if strings.Contains(rendered, "(folded)") {
		t.Fatal("manager must not fold skills behind the unified compression")
	}
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") || !strings.Contains(rendered, "=== Skill: code-review ===") {
		t.Fatal("both skills should render in full")
	}
}

func TestSkillsContextManager_UnfoldByReload(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("code-review")
	cfg.GetTimeline().FreezeAll()

	// Reload is idempotent while the body is still visible: no duplicate append.
	before := RenderTimelineFrozenOpen(cfg.GetTimeline()).Open
	_ = mgr.LoadSkill("deploy-app")
	after := RenderTimelineFrozenOpen(cfg.GetTimeline()).Open
	if before != after {
		t.Fatalf("reloading a visible skill must not change the Open tail:\nbefore=%q\nafter=%q", before, after)
	}

	rendered := mgr.Render("unfold_nonce")
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatal("deploy-app should appear in rendered output")
	}
}

// --- Integration: ChangeViewOffset for a non-SKILL.md file ---

func TestSkillsContextManager_ChangeViewOffset_SubFile(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	_ = mgr.LoadSkill("code-review")

	// Open a secondary file from the skill
	err := mgr.ChangeViewOffset("code-review", "rules/RULES.md", 1)
	if err != nil {
		t.Fatalf("ChangeViewOffset for sub-file failed: %v", err)
	}
	cfg.GetTimeline().FreezeAll()

	rendered := mgr.Render("subfile_nonce")
	if !strings.Contains(rendered, "rules/RULES.md") {
		t.Fatal("rendered should contain the sub-file")
	}
}

func TestSkillsContextManager_ChangeViewOffset_NonexistentFile(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	_ = mgr.LoadSkill("deploy-app")

	err := mgr.ChangeViewOffset("deploy-app", "nonexistent.txt", 1)
	if err == nil {
		t.Fatal("ChangeViewOffset for nonexistent file should fail")
	}
}

// --- Integration: concurrent safety ---

func TestSkillsContextManager_ConcurrentAccess(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	var wg sync.WaitGroup
	// Concurrent loads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.LoadSkill("deploy-app")
		}()
	}
	// Concurrent renders
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.Render("concurrent_nonce")
		}()
	}
	// Concurrent offset changes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.ChangeViewOffset("deploy-app", "", 2)
		}()
	}
	// Concurrent state queries
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.HasRegisteredSkills()
			_ = mgr.HasTruncatedViews()
		}()
	}
	wg.Wait()
	cfg.GetTimeline().FreezeAll()

	// After all concurrent ops, render should still produce valid output
	rendered := mgr.Render("final_nonce")
	if rendered == "" {
		t.Fatal("render after concurrent access should not be empty")
	}
	if !strings.Contains(rendered, "SKILLS_CONTEXT_") {
		t.Fatal("render after concurrent access should have proper tags")
	}
}

func TestFSSkillLoader_ConcurrentAccess(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = loader.AllSkillMetas()
			_, _ = loader.LoadSkill("deploy-app")
			_ = loader.HasSkills()
			_, _ = loader.GetFileSystem("deploy-app")
		}()
	}
	wg.Wait()
}

// --- Integration: prompt context completeness check ---

func TestSkillsContextManager_PromptContextCompleteness(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	_ = mgr.LoadSkill("deploy-app")
	// 冻结提交后正文才进入可缓存的单一技能块。
	cfg.GetTimeline().FreezeAll()

	rendered := mgr.Render("completeness_test")

	// Check all required structural elements
	requiredElements := []struct {
		element     string
		description string
	}{
		{"SKILLS_CONTEXT_", "context block tag"},
		{"=== Skill: deploy-app ===", "skill header"},
		{"Description:", "skill description"},
		{"File Tree:", "file tree section"},
		{"<|VIEW_WINDOW_", "view window start tag"},
		{"<|VIEW_WINDOW_END_", "view window end tag"},
	}

	for _, req := range requiredElements {
		if !strings.Contains(rendered, req.element) {
			t.Fatalf("prompt context missing required element %q (%s)\nFull rendered:\n%s",
				req.element, req.description, rendered)
		}
	}

	// Validate tag pairing: 单一技能块只有一对边界标签。
	// 注: END 标签本身也带 "SKILLS_CONTEXT_" 前缀。
	endTagCount := strings.Count(rendered, "SKILLS_CONTEXT_END_")
	allTagCount := strings.Count(rendered, "SKILLS_CONTEXT_")
	if endTagCount != 1 || allTagCount != 2 {
		t.Fatalf("expected exactly one start/end pair, got all=%d end=%d", allTagCount, endTagCount)
	}

	startIdx := strings.Index(rendered, "SKILLS_CONTEXT_")
	endIdx := strings.Index(rendered, "SKILLS_CONTEXT_END_")
	if startIdx >= endIdx {
		t.Fatal("context start tag should come before end tag")
	}
}

// --- Integration: ViewWindow large file rendering in context ---

func TestSkillsContextManager_LargeSkillMD(t *testing.T) {
	// Create a skill with a large SKILL.md that exceeds 32KB view window
	vfs := filesys.NewVirtualFs()
	largeBody := strings.Repeat("This is a long line of skill documentation content.\n", 700)
	vfs.AddFile("large-skill/SKILL.md", buildSkillMD(
		"large-skill",
		"A skill with very large SKILL.md content.",
		largeBody,
	))

	loader, err := aiskillloader.NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)
	if err := mgr.LoadSkill("large-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	cfg.GetTimeline().FreezeAll()

	rendered := mgr.Render("large_nonce")
	if rendered == "" {
		t.Fatal("render should not be empty for large skill")
	}
	// VIEW_WINDOW should exist and contain truncation indicator
	if !strings.Contains(rendered, "VIEW_WINDOW") {
		t.Fatal("rendered should contain VIEW_WINDOW")
	}
	// HasTruncatedViews should detect the truncation
	if !mgr.HasTruncatedViews() {
		t.Fatal("large SKILL.md should result in truncated view")
	}
}
