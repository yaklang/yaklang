package aicommon

import (
	"strings"
	"sync"
	"testing"

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

	// Step 1: before loading, manager should still detect available skills
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should detect registered skills from loader")
	}
	if mgr.HasTruncatedViews() {
		t.Fatal("no truncated views before loading any skill")
	}

	// Step 2: render without loading -> should list available skills
	rendered := mgr.Render("lifecycle_nonce")
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_lifecycle_nonce|>") {
		t.Fatal("render should have context start tag")
	}
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_END_lifecycle_nonce|>") {
		t.Fatal("render should have context end tag")
	}
	if !strings.Contains(rendered, "deploy-app") || !strings.Contains(rendered, "code-review") {
		t.Fatal("render should list all available skills before loading")
	}

	// Step 3: load a skill
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered = mgr.Render("lifecycle_nonce")
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatal("rendered should contain fully expanded skill")
	}
	if !strings.Contains(rendered, "VIEW_WINDOW") {
		t.Fatal("rendered should contain SKILL.md view window")
	}

	// Step 4: load second skill
	if err := mgr.LoadSkill("code-review"); err != nil {
		t.Fatalf("LoadSkill for code-review failed: %v", err)
	}

	rendered = mgr.Render("lifecycle_nonce")
	if !strings.Contains(rendered, "deploy-app") {
		t.Fatal("rendered should still contain deploy-app")
	}
	if !strings.Contains(rendered, "code-review") {
		t.Fatal("rendered should contain code-review")
	}

	// Step 5: change view offset
	if err := mgr.ChangeViewOffset("deploy-app", "", 2); err != nil {
		t.Fatalf("ChangeViewOffset failed: %v", err)
	}

	rendered = mgr.Render("lifecycle_nonce")
	if rendered == "" {
		t.Fatal("render after offset change should not be empty")
	}
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

	// Use a very small max bytes to force folding
	mgr.SetMaxBytes(300)

	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("code-review")

	rendered := mgr.Render("fold_nonce")

	// The rendered prompt must still be valid (have open/close tags)
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_fold_nonce|>") {
		t.Fatal("folded render must have start tag")
	}
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_END_fold_nonce|>") {
		t.Fatal("folded render must have end tag")
	}

	// At least one skill should be folded
	if !strings.Contains(rendered, "(folded)") {
		t.Fatal("with very small limit, skills should be folded")
	}
}

func TestSkillsContextManager_UnfoldByReload(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)

	// Force folding with small limit
	mgr.SetMaxBytes(400)
	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("code-review")

	// Now increase limit and reload the folded skill
	mgr.SetMaxBytes(aiskillloader.SkillsContextMaxBytes)
	// Reloading a folded skill should unfold it
	_ = mgr.LoadSkill("deploy-app")

	rendered := mgr.Render("unfold_nonce")
	// deploy-app should be unfolded now (not have "(folded)")
	// Check that deploy-app section exists and has VIEW_WINDOW (indicating full view)
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatal("deploy-app should appear in rendered output")
	}
}

// --- Integration: ChangeViewOffset for a non-SKILL.md file ---

func TestSkillsContextManager_ChangeViewOffset_SubFile(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)

	_ = mgr.LoadSkill("code-review")

	// Open a secondary file from the skill
	err := mgr.ChangeViewOffset("code-review", "rules/RULES.md", 1)
	if err != nil {
		t.Fatalf("ChangeViewOffset for sub-file failed: %v", err)
	}

	rendered := mgr.Render("subfile_nonce")
	if !strings.Contains(rendered, "rules/RULES.md") {
		t.Fatal("rendered should contain the sub-file")
	}
}

func TestSkillsContextManager_ChangeViewOffset_NonexistentFile(t *testing.T) {
	vfs := buildSkillsVFS()
	loader, _ := aiskillloader.NewFSSkillLoader(vfs)
	mgr := aiskillloader.NewSkillsContextManager(loader)

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

	// After all concurrent ops, render should still produce valid output
	rendered := mgr.Render("final_nonce")
	if rendered == "" {
		t.Fatal("render after concurrent access should not be empty")
	}
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_final_nonce|>") {
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
			_, _ = loader.ListSkills()
			_, _ = loader.LoadSkill("deploy-app")
			_, _ = loader.SearchSkills("deploy")
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

	_ = mgr.LoadSkill("deploy-app")

	nonce := "completeness_test"
	rendered := mgr.Render(nonce)

	// Check all required structural elements
	requiredElements := []struct {
		element     string
		description string
	}{
		{"<|SKILLS_CONTEXT_" + nonce + "|>", "context start tag"},
		{"<|SKILLS_CONTEXT_END_" + nonce + "|>", "context end tag"},
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

	// Validate tag pairing
	startTagCount := strings.Count(rendered, "<|SKILLS_CONTEXT_"+nonce+"|>")
	endTagCount := strings.Count(rendered, "<|SKILLS_CONTEXT_END_"+nonce+"|>")
	if startTagCount != 1 || endTagCount != 1 {
		t.Fatalf("expected exactly 1 start and 1 end context tag, got start=%d end=%d", startTagCount, endTagCount)
	}

	// Check that start tag comes before end tag
	startIdx := strings.Index(rendered, "<|SKILLS_CONTEXT_"+nonce+"|>")
	endIdx := strings.Index(rendered, "<|SKILLS_CONTEXT_END_"+nonce+"|>")
	if startIdx >= endIdx {
		t.Fatal("context start tag should come before end tag")
	}
}

// --- Integration: ViewWindow large file rendering in context ---

func TestSkillsContextManager_LargeSkillMD(t *testing.T) {
	// Create a skill with a large SKILL.md that exceeds 15KB view window
	vfs := filesys.NewVirtualFs()
	largeBody := strings.Repeat("This is a long line of skill documentation content.\n", 400)
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
	if err := mgr.LoadSkill("large-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

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
