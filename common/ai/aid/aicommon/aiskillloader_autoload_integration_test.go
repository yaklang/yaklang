package aicommon

import (
	"strings"
	"sync"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// --- helpers ---

func buildAutoloadSkillMD(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body
}

func buildAutoloadVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("deploy-app/SKILL.md", buildAutoloadSkillMD(
		"deploy-app",
		"Deploy the application to staging or production.",
		"# Deploy App\n\nStep 1: build.\nStep 2: deploy.\n",
	))
	vfs.AddFile("deploy-app/scripts/deploy.sh", "#!/bin/bash\necho deploy")
	vfs.AddFile("security/scanning/vuln-scan/SKILL.md", buildAutoloadSkillMD(
		"vuln-scan",
		"Scan for security vulnerabilities in the codebase.",
		"# Vulnerability Scanner\n\nUse static analysis.\n",
	))
	vfs.AddFile("tools/code-review/SKILL.md", buildAutoloadSkillMD(
		"code-review",
		"Perform an automated code review with linters.",
		"# Code Review\n\nRun linters on the codebase.\n",
	))
	return vfs
}

func newAutoloadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	db.AutoMigrate(&schema.AISkill{})
	return db
}

// --- Integration: AutoSkillLoader + SkillsContextManager ---

func TestAutoSkillLoader_Integration_WithContextManager(t *testing.T) {
	vfs := buildAutoloadVFS()
	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	mgr := aiskillloader.NewSkillsContextManager(loader)

	// Manager should detect available skills
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should detect skills from AutoSkillLoader")
	}

	// Render available skills list
	rendered := mgr.Render("autoload_nonce")
	if !strings.Contains(rendered, "deploy-app") {
		t.Fatal("render should list deploy-app")
	}
	if !strings.Contains(rendered, "vuln-scan") {
		t.Fatal("render should list vuln-scan")
	}

	// Load a skill
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered = mgr.Render("autoload_nonce")
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatal("rendered should contain fully expanded deploy-app")
	}
	if !strings.Contains(rendered, "VIEW_WINDOW") {
		t.Fatal("rendered should contain VIEW_WINDOW")
	}
}

func TestAutoSkillLoader_Integration_PromptCompleteness(t *testing.T) {
	vfs := buildAutoloadVFS()
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader)

	_ = mgr.LoadSkill("vuln-scan")

	nonce := "completeness_autoload"
	rendered := mgr.Render(nonce)

	requiredElements := []struct {
		element     string
		description string
	}{
		{"<|SKILLS_CONTEXT_" + nonce + "|>", "context start tag"},
		{"<|SKILLS_CONTEXT_END_" + nonce + "|>", "context end tag"},
		{"=== Skill: vuln-scan ===", "skill header"},
		{"Description:", "skill description"},
		{"<|VIEW_WINDOW_", "view window start tag"},
		{"<|VIEW_WINDOW_END_", "view window end tag"},
	}

	for _, req := range requiredElements {
		if !strings.Contains(rendered, req.element) {
			t.Fatalf("prompt context missing %q (%s)\nFull rendered:\n%s",
				req.element, req.description, rendered)
		}
	}
}

func TestAutoSkillLoader_Integration_FoldingWithManySkills(t *testing.T) {
	vfs := buildAutoloadVFS()
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader)

	// Use small limit to force folding
	mgr.SetMaxBytes(500)

	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("vuln-scan")
	_ = mgr.LoadSkill("code-review")

	rendered := mgr.Render("fold_nonce")

	// Tags must still be valid
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_fold_nonce|>") {
		t.Fatal("folded render must have start tag")
	}
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_END_fold_nonce|>") {
		t.Fatal("folded render must have end tag")
	}
	// At least one should be folded
	if !strings.Contains(rendered, "(folded)") {
		t.Fatal("with small limit, some skills should be folded")
	}
}

// --- Integration: AutoSkillLoader + DB + SkillsContextManager ---

func TestAutoSkillLoader_Integration_DBAndContextManager(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()

	vfs := buildAutoloadVFS()
	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with DB failed: %v", err)
	}

	mgr := aiskillloader.NewSkillsContextManager(loader, aiskillloader.WithManagerDB(db))

	// Verify skills are in DB
	skill, err := yakit.GetAISkillByName(db, "deploy-app")
	if err != nil {
		t.Fatalf("skill should be persisted in DB: %v", err)
	}
	if skill.Description != "Deploy the application to staging or production." {
		t.Fatalf("unexpected description: %q", skill.Description)
	}

	// Verify manager still works with AutoSkillLoader
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should have skills")
	}

	_ = mgr.LoadSkill("deploy-app")
	rendered := mgr.Render("db_nonce")
	if !strings.Contains(rendered, "deploy-app") {
		t.Fatal("rendered should contain deploy-app")
	}
}

// --- Integration: BM25 search results loaded into context ---

func TestAutoSkillLoader_Integration_BM25ResultsInContext(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()

	vfs := buildAutoloadVFS()
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader, aiskillloader.WithManagerDB(db))

	// Search for skills via BM25
	results, err := mgr.SearchKeywordBM25("vulnerability security", 5)
	if err != nil {
		t.Fatalf("BM25 search failed: %v", err)
	}

	// Load found skills into context
	for _, meta := range results {
		_ = mgr.LoadSkill(meta.Name)
	}

	rendered := mgr.Render("bm25_context_nonce")
	if rendered == "" {
		t.Fatal("rendered should not be empty after loading BM25 results")
	}
	// The vulnerability-related skill should be in context
	if !strings.Contains(rendered, "vuln-scan") {
		t.Fatal("BM25 result 'vuln-scan' should be loadable into context")
	}
}

// --- Integration: Concurrent AutoSkillLoader + SkillsContextManager ---

func TestAutoSkillLoader_Integration_ConcurrentAccess(t *testing.T) {
	vfs := buildAutoloadVFS()
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
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
	// Concurrent BM25 searches
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.SearchKeywordBM25("deploy", 5)
		}()
	}
	// Concurrent state queries
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.HasRegisteredSkills()
			_ = mgr.HasTruncatedViews()
			_, _ = mgr.ListSkills()
		}()
	}
	wg.Wait()

	// After all concurrent ops, system should be stable
	rendered := mgr.Render("final_nonce")
	if !strings.Contains(rendered, "<|SKILLS_CONTEXT_final_nonce|>") {
		t.Fatal("render after concurrent access should have proper tags")
	}
}

// --- Integration: SearchByAI mock callback prompt verification ---

func TestAutoSkillLoader_Integration_SearchByAIMock(t *testing.T) {
	vfs := buildAutoloadVFS()

	var promptReceived string
	mockCb := func(prompt string, schema string) ([]aiskillloader.SkillSelection, error) {
		promptReceived = prompt
		return []aiskillloader.SkillSelection{
			{SkillName: "vuln-scan", Reason: "security scanning is relevant"},
		}, nil
	}

	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader, aiskillloader.WithManagerSearchAICallback(mockCb))

	results, err := mgr.SearchByAI("scan my code for security issues")
	if err != nil {
		t.Fatalf("SearchByAI failed: %v", err)
	}

	if promptReceived == "" {
		t.Fatal("LiteForge callback should have been invoked")
	}
	// Verify all skills appear in the prompt
	for _, name := range []string{"deploy-app", "vuln-scan", "code-review"} {
		if !strings.Contains(promptReceived, name) {
			t.Fatalf("prompt should contain skill %q", name)
		}
	}
	// Verify user need is in the prompt
	if !strings.Contains(promptReceived, "scan my code for security issues") {
		t.Fatal("prompt should contain user need")
	}

	// Verify results
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "vuln-scan" {
		t.Fatalf("expected vuln-scan, got %q", results[0].Name)
	}
}

// --- Integration: Nil-safe operations ---

func TestAutoSkillLoader_Integration_NilSafe(t *testing.T) {
	// AutoSkillLoader with no sources should not panic when used with ContextManager
	loader, _ := aiskillloader.NewAutoSkillLoader()
	mgr := aiskillloader.NewSkillsContextManager(loader)

	if mgr.HasRegisteredSkills() {
		t.Fatal("no skills should be registered")
	}

	rendered := mgr.Render("nil_nonce")
	if rendered != "" {
		t.Fatalf("render should be empty, got %q", rendered)
	}

	err := mgr.LoadSkill("nonexistent")
	if err == nil {
		t.Fatal("loading nonexistent skill should fail")
	}
}
