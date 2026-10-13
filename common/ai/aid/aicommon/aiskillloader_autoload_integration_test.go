package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/gorm"
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
	db.AutoMigrate(&schema.AIForge{})
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
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	// Manager should detect available skills
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should detect skills from AutoSkillLoader")
	}

	// Surface catalog (default hidden; intent recognition does this) and load a skill:
	// 两者都先进 Timeline Open, 冻结后才进可缓存的单一技能块。
	mgr.SetCatalogSkills(loader.AllSkillMetas())
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	open := RenderTimelineFrozenOpen(cfg.GetTimeline()).Open
	require.Contains(t, open, "deploy-app")
	require.Contains(t, open, "vuln-scan")
	require.Contains(t, open, "VIEW_WINDOW")

	cfg.GetTimeline().FreezeAll()
	rendered := mgr.Render("autoload_nonce")
	require.Contains(t, rendered, "deploy-app")
	require.Contains(t, rendered, "vuln-scan")
	require.Contains(t, rendered, "=== Skill: deploy-app ===")
	require.Contains(t, rendered, "VIEW_WINDOW")
}

func TestAutoSkillLoader_Integration_PromptCompleteness(t *testing.T) {
	vfs := buildAutoloadVFS()
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader)
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	_ = mgr.LoadSkill("vuln-scan")
	cfg.GetTimeline().FreezeAll()

	rendered := mgr.Render("completeness_autoload")

	requiredElements := []struct {
		element     string
		description string
	}{
		{"SKILLS_CONTEXT_", "context start tag"},
		{"SKILLS_CONTEXT_END_", "context end tag"},
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
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	// manager 侧折叠预算已删除: 正文体积由 Timeline 统一压缩控制。
	mgr.SetMaxTokens(100)

	_ = mgr.LoadSkill("deploy-app")
	_ = mgr.LoadSkill("vuln-scan")
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
}

// --- Integration: AutoSkillLoader + DB-backed skillmd forges + SkillsContextManager ---

func TestAutoSkillLoader_Integration_DBAndContextManager(t *testing.T) {
	db := newAutoloadTestDB(t)
	defer db.Close()

	vfs := buildAutoloadVFS()
	sourceLoader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	if _, err := aiskillloader.ImportAISkillsToDB(db, sourceLoader); err != nil {
		t.Fatalf("ImportAISkillsToDB failed: %v", err)
	}
	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_Database(db),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with database source failed: %v", err)
	}
	if metas := loader.AllSkillMetas(); len(metas) != 0 {
		t.Fatalf("database source should stay lazy at init, got %d eagerly loaded metas", len(metas))
	}

	mgr := aiskillloader.NewSkillsContextManager(loader, aiskillloader.WithManagerDB(db))
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	forge, err := yakit.GetAIForgeByNameAndTypes(db, "deploy-app", schema.FORGE_TYPE_SkillMD)
	if err != nil {
		t.Fatalf("skillmd forge should be persisted in DB: %v", err)
	}
	if forge.Description != "Deploy the application to staging or production." {
		t.Fatalf("unexpected description: %q", forge.Description)
	}

	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should have skills")
	}
	// 改造后: 目录默认隐藏, 数据库来源 skill 是 lazy 的 (不在 AllSkillMetas).
	// 数据库 skill 通过搜索 (BM25) 访问, 验证可检索即可.
	results, err := mgr.SearchKeywordBM25("deploy", 5)
	if err != nil {
		t.Fatalf("SearchKeywordBM25 failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected to find deploy-app via BM25 search on database-backed skills")
	}
	found := false
	for _, m := range results {
		if m != nil && m.Name == "deploy-app" {
			found = true
		}
	}
	if !found {
		t.Fatalf("BM25 search should find deploy-app, got: %v", results)
	}

	_ = mgr.LoadSkill("deploy-app")
	cfg.GetTimeline().FreezeAll()
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
	sourceLoader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(vfs),
	)
	if _, err := aiskillloader.ImportAISkillsToDB(db, sourceLoader); err != nil {
		t.Fatalf("ImportAISkillsToDB failed: %v", err)
	}
	loader, _ := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_Database(db),
	)
	mgr := aiskillloader.NewSkillsContextManager(loader, aiskillloader.WithManagerDB(db))
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

	// Search for skills via BM25
	results, err := mgr.SearchKeywordBM25("vulnerability security", 5)
	if err != nil {
		t.Fatalf("BM25 search failed: %v", err)
	}

	// Load found skills into context
	for _, meta := range results {
		_ = mgr.LoadSkill(meta.Name)
	}

	cfg.GetTimeline().FreezeAll()
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
	cfg.GetTimeline().FreezeAll()

	// After all concurrent ops, system should be stable
	rendered := mgr.Render("final_nonce")
	if !strings.Contains(rendered, "SKILLS_CONTEXT_") {
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
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

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
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	bindTestTimeline(mgr, cfg)

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
