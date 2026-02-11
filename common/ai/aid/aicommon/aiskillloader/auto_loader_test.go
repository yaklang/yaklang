package aiskillloader

import (
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

func buildAutoLoaderTestSkillMD(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body
}

func buildNestedTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	// Top-level skill
	vfs.AddFile("top-skill/SKILL.md", buildAutoLoaderTestSkillMD(
		"top-skill",
		"A top-level skill for deployment.",
		"# Top Skill\n\nDeploy things.\n",
	))
	vfs.AddFile("top-skill/scripts/run.sh", "#!/bin/bash\necho run")

	// Deeply nested skill
	vfs.AddFile("deep/nested/hidden-skill/SKILL.md", buildAutoLoaderTestSkillMD(
		"hidden-skill",
		"A deeply nested skill for security scanning.",
		"# Hidden Skill\n\nScan for vulnerabilities.\n",
	))

	// Another skill at second level
	vfs.AddFile("tools/code-review/SKILL.md", buildAutoLoaderTestSkillMD(
		"code-review",
		"Automated code review with linters.",
		"# Code Review\n\nUse linters and static analysis.\n",
	))
	vfs.AddFile("tools/code-review/rules.yaml", "rules:\n  - no-eval\n")

	// Directory without SKILL.md (should be ignored)
	vfs.AddFile("no-skill/readme.txt", "nothing here")

	return vfs
}

func TestAutoSkillLoader_RecursiveDiscovery(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	if !loader.HasSkills() {
		t.Fatal("loader should have discovered skills")
	}

	metas := loader.AllSkillMetas()
	if len(metas) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(metas))
	}
	names := make(map[string]bool)
	for _, s := range metas {
		names[s.Name] = true
	}
	for _, expected := range []string{"top-skill", "hidden-skill", "code-review"} {
		if !names[expected] {
			t.Fatalf("expected skill %q not found", expected)
		}
	}
}

func TestAutoSkillLoader_LoadSkill(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	loaded, err := loader.LoadSkill("hidden-skill")
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if loaded.Meta.Name != "hidden-skill" {
		t.Fatalf("expected 'hidden-skill', got %q", loaded.Meta.Name)
	}
	if !strings.Contains(loaded.SkillMDContent, "Hidden Skill") {
		t.Fatal("SKILL.md content should contain body")
	}
}

func TestAutoSkillLoader_LoadSkill_NotFound(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if _, err := loader.LoadSkill("nonexistent"); err == nil {
		t.Fatal("LoadSkill should fail for nonexistent skill")
	}
}

func TestAutoSkillLoader_GetFileSystem(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	fsys, err := loader.GetFileSystem("code-review")
	if err != nil {
		t.Fatalf("GetFileSystem failed: %v", err)
	}
	content, err := fsys.ReadFile("rules.yaml")
	if err != nil {
		t.Fatalf("ReadFile via sub-fs failed: %v", err)
	}
	if !strings.Contains(string(content), "no-eval") {
		t.Fatalf("expected rules content, got %q", string(content))
	}
}

func TestAutoSkillLoader_EmptyVFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	if loader.HasSkills() {
		t.Fatal("empty VFS should have no skills")
	}
}

func TestAutoSkillLoader_NilFileSystem(t *testing.T) {
	if _, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(nil)); err == nil {
		t.Fatal("should fail with nil filesystem")
	}
}

func TestAutoSkillLoader_NoSources(t *testing.T) {
	loader, err := NewAutoSkillLoader()
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with no sources should succeed: %v", err)
	}
	if loader.HasSkills() {
		t.Fatal("no sources should have no skills")
	}
}

func TestAutoSkillLoader_MultipleSources(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("skill-a/SKILL.md", buildAutoLoaderTestSkillMD(
		"skill-a", "Skill A description.", "Body A",
	))

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("skill-b/SKILL.md", buildAutoLoaderTestSkillMD(
		"skill-b", "Skill B description.", "Body B",
	))

	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs1),
		WithAutoLoad_FileSystem(vfs2),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	if len(loader.AllSkillMetas()) != 2 {
		t.Fatalf("expected 2 skills from 2 sources, got %d", len(loader.AllSkillMetas()))
	}
}

func TestComputeSkillHash_Deterministic(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")
	vfs.AddFile("script.sh", "echo hello")

	h1 := ComputeSkillHash(vfs)
	h2 := ComputeSkillHash(vfs)
	if h1 != h2 {
		t.Fatal("hash should be deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("SHA256 hex should be 64 chars, got %d", len(h1))
	}
}

func TestComputeSkillHash_ChangesOnFileModification(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("SKILL.md", "---\nname: test\ndescription: v1\n---\nbody")

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("SKILL.md", "---\nname: test\ndescription: v2\n---\nbody")

	h1 := ComputeSkillHash(vfs1)
	h2 := ComputeSkillHash(vfs2)
	if h1 == h2 {
		t.Fatal("hash should differ when file content changes")
	}
}

func TestComputeSkillHash_IgnoresLargeFiles(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")
	vfs2.AddFile("large.bin", strings.Repeat("x", 11*1024))

	h1 := ComputeSkillHash(vfs1)
	h2 := ComputeSkillHash(vfs2)
	if h1 != h2 {
		t.Fatal("hash should ignore files > 10KB")
	}
}

func TestAutoSkillLoader_ConcurrentAccess(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = loader.AllSkillMetas()
			_, _ = loader.LoadSkill("top-skill")
			_ = loader.HasSkills()
			_, _ = loader.GetFileSystem("top-skill")
		}()
	}
	wg.Wait()
}

/*
package aiskillloader

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// --- helpers ---

func buildNestedTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	// Top-level skill
	vfs.AddFile("top-skill/SKILL.md", buildTestSkillMD(
		"top-skill",
		"A top-level skill for deployment.",
		"# Top Skill\n\nDeploy things.\n",
	))
	vfs.AddFile("top-skill/scripts/run.sh", "#!/bin/bash\necho run")

	// Deeply nested skill
	vfs.AddFile("deep/nested/hidden-skill/SKILL.md", buildTestSkillMD(
		"hidden-skill",
		"A deeply nested skill for security scanning.",
		"# Hidden Skill\n\nScan for vulnerabilities.\n",
	))

	// Another skill at second level
	vfs.AddFile("tools/code-review/SKILL.md", buildTestSkillMD(
		"code-review",
		"Automated code review with linters.",
		"# Code Review\n\nUse linters and static analysis.\n",
	))
	vfs.AddFile("tools/code-review/rules.yaml", "rules:\n  - no-eval\n")

	// Directory without SKILL.md (should be ignored)
	vfs.AddFile("no-skill/readme.txt", "nothing here")

	return vfs
}

func newTestMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory DB: %v", err)
	}
	db.AutoMigrate(&schema.AISkill{})
	return db
}

// --- AutoSkillLoader: Recursive Discovery ---

func TestAutoSkillLoader_RecursiveDiscovery(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	if !loader.HasSkills() {
		t.Fatal("loader should have discovered skills")
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}

	// Verify each skill was discovered
	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, expected := range []string{"top-skill", "hidden-skill", "code-review"} {
		if !names[expected] {
			t.Fatalf("expected skill %q not found", expected)
		}
	}
}

func TestAutoSkillLoader_LoadSkill(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	loaded, err := loader.LoadSkill("hidden-skill")
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if loaded.Meta.Name != "hidden-skill" {
		t.Fatalf("expected 'hidden-skill', got %q", loaded.Meta.Name)
	}
	if !strings.Contains(loaded.SkillMDContent, "Hidden Skill") {
		t.Fatal("SKILL.md content should contain body")
	}
}

func TestAutoSkillLoader_LoadSkill_NotFound(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	_, err := loader.LoadSkill("nonexistent")
	if err == nil {
		t.Fatal("LoadSkill should fail for nonexistent skill")
	}
}

func TestAutoSkillLoader_SearchSkills(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	results, err := loader.SearchSkills("deploy")
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'deploy', got %d", len(results))
	}
	if results[0].Name != "top-skill" {
		t.Fatalf("expected 'top-skill', got %q", results[0].Name)
	}
}

func TestAutoSkillLoader_GetFileSystem(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	fs, err := loader.GetFileSystem("code-review")
	if err != nil {
		t.Fatalf("GetFileSystem failed: %v", err)
	}

	content, err := fs.ReadFile("rules.yaml")
	if err != nil {
		t.Fatalf("ReadFile via sub-fs failed: %v", err)
	}
	if !strings.Contains(string(content), "no-eval") {
		t.Fatalf("expected rules content, got %q", string(content))
	}
}

// --- AutoSkillLoader: Empty/Nil sources ---

func TestAutoSkillLoader_EmptyVFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}
	if loader.HasSkills() {
		t.Fatal("empty VFS should have no skills")
	}
}

func TestAutoSkillLoader_NilFileSystem(t *testing.T) {
	_, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(nil))
	if err == nil {
		t.Fatal("should fail with nil filesystem")
	}
}

func TestAutoSkillLoader_NoSources(t *testing.T) {
	loader, err := NewAutoSkillLoader()
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with no sources should succeed: %v", err)
	}
	if loader.HasSkills() {
		t.Fatal("no sources should have no skills")
	}
}

// --- AutoSkillLoader: Multiple sources ---

func TestAutoSkillLoader_MultipleSources(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("skill-a/SKILL.md", buildTestSkillMD(
		"skill-a", "Skill A description.", "Body A",
	))

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("skill-b/SKILL.md", buildTestSkillMD(
		"skill-b", "Skill B description.", "Body B",
	))

	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs1),
		WithAutoLoad_FileSystem(vfs2),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills from 2 sources, got %d", len(skills))
	}
}

// --- AutoSkillLoader: Hash computation ---

func TestComputeSkillHash_Deterministic(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")
	vfs.AddFile("script.sh", "echo hello")

	h1 := computeSkillHash(vfs)
	h2 := computeSkillHash(vfs)
	if h1 != h2 {
		t.Fatal("hash should be deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("SHA256 hex should be 64 chars, got %d", len(h1))
	}
}

func TestComputeSkillHash_ChangesOnFileModification(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("SKILL.md", "---\nname: test\ndescription: v1\n---\nbody")

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("SKILL.md", "---\nname: test\ndescription: v2\n---\nbody")

	h1 := computeSkillHash(vfs1)
	h2 := computeSkillHash(vfs2)
	if h1 == h2 {
		t.Fatal("hash should differ when file content changes")
	}
}

func TestComputeSkillHash_IgnoresLargeFiles(t *testing.T) {
	vfs1 := filesys.NewVirtualFs()
	vfs1.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")

	vfs2 := filesys.NewVirtualFs()
	vfs2.AddFile("SKILL.md", "---\nname: test\ndescription: test\n---\nbody")
	// Add a file > 10KB that should be ignored
	vfs2.AddFile("large.bin", strings.Repeat("x", 11*1024))

	h1 := computeSkillHash(vfs1)
	h2 := computeSkillHash(vfs2)
	if h1 != h2 {
		t.Fatal("hash should ignore files > 10KB")
	}
}

// --- AutoSkillLoader: DB persistence ---

func TestAutoSkillLoader_WithDatabase(t *testing.T) {
	db := newTestMemDB(t)
	defer db.Close()

	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_Database(db),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with DB failed: %v", err)
	}
	if !loader.HasSkills() {
		t.Fatal("should have skills")
	}

	// Verify skills were persisted to DB
	skill, err := yakit.GetAISkillByName(db, "top-skill")
	if err != nil {
		t.Fatalf("GetAISkillByName failed: %v", err)
	}
	if skill.Name != "top-skill" {
		t.Fatalf("expected 'top-skill', got %q", skill.Name)
	}
	if skill.Hash == "" {
		t.Fatal("hash should be set")
	}
}

func TestAutoSkillLoader_DBHashDedup(t *testing.T) {
	db := newTestMemDB(t)
	defer db.Close()

	vfs := buildNestedTestVFS()

	// First load
	_, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_Database(db),
	)
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	skill1, _ := yakit.GetAISkillByName(db, "top-skill")
	updatedAt1 := skill1.UpdatedAt

	// Second load with same content should not update
	_, err = NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_Database(db),
	)
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}

	skill2, _ := yakit.GetAISkillByName(db, "top-skill")
	if !skill2.UpdatedAt.Equal(updatedAt1) {
		t.Fatal("skill should not be updated when hash matches")
	}
}

// --- AutoSkillLoader: BM25 search (in-memory mode) ---

func TestAutoSkillLoader_SearchKeywordBM25_InMemory(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	results, err := loader.SearchKeywordBM25("security scanning", 5)
	if err != nil {
		t.Fatalf("SearchKeywordBM25 failed: %v", err)
	}
	// Should find the "hidden-skill" which has "security scanning" in description
	found := false
	for _, r := range results {
		if r.Name == "hidden-skill" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(results))
		for _, r := range results {
			names = append(names, r.Name)
		}
		t.Fatalf("expected 'hidden-skill' in results, got %v", names)
	}
}

func TestAutoSkillLoader_SearchKeywordBM25_EmptyQuery(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	results, err := loader.SearchKeywordBM25("", 5)
	if err != nil {
		t.Fatalf("empty query should not error: %v", err)
	}
	if results != nil {
		t.Fatal("empty query should return nil")
	}
}

func TestAutoSkillLoader_SearchKeywordBM25_NoSkills(t *testing.T) {
	loader, _ := NewAutoSkillLoader()

	results, err := loader.SearchKeywordBM25("anything", 5)
	if err != nil {
		t.Fatalf("no skills should not error: %v", err)
	}
	if results != nil {
		t.Fatal("no skills should return nil")
	}
}

// --- AutoSkillLoader: BM25 search (DB mode) ---

func TestAutoSkillLoader_SearchKeywordBM25_WithDB(t *testing.T) {
	db := newTestMemDB(t)
	defer db.Close()

	vfs := buildNestedTestVFS()
	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_Database(db),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader with DB failed: %v", err)
	}

	results, err := loader.SearchKeywordBM25("code review linters", 5)
	if err != nil {
		t.Fatalf("SearchKeywordBM25 with DB failed: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Name == "code-review" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(results))
		for _, r := range results {
			names = append(names, r.Name)
		}
		t.Fatalf("expected 'code-review' in DB BM25 results, got %v", names)
	}
}

// --- AutoSkillLoader: SearchByAI with mock ---

func TestAutoSkillLoader_SearchByAI_Mock(t *testing.T) {
	vfs := buildNestedTestVFS()

	var capturedPrompt string
	var capturedSchema string
	mockCallback := func(prompt string, schema string) ([]SkillSelection, error) {
		capturedPrompt = prompt
		capturedSchema = schema
		// Return mock selections
		return []SkillSelection{
			{SkillName: "top-skill", Reason: "matches deployment requirement"},
			{SkillName: "hidden-skill", Reason: "security scanning useful"},
		}, nil
	}

	loader, err := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_SearchAICallback(mockCallback),
	)
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	results, err := loader.SearchByAI("deploy my application to production")
	if err != nil {
		t.Fatalf("SearchByAI failed: %v", err)
	}

	// Verify prompt contains expected elements
	if capturedPrompt == "" {
		t.Fatal("mock callback was not called")
	}
	if !strings.Contains(capturedPrompt, "deploy my application to production") {
		t.Fatal("prompt should contain user need")
	}
	if !strings.Contains(capturedPrompt, "top-skill") {
		t.Fatal("prompt should contain skill name 'top-skill'")
	}
	if !strings.Contains(capturedPrompt, "hidden-skill") {
		t.Fatal("prompt should contain skill name 'hidden-skill'")
	}
	if !strings.Contains(capturedPrompt, "code-review") {
		t.Fatal("prompt should contain skill name 'code-review'")
	}

	// Verify schema was passed
	if capturedSchema == "" {
		t.Fatal("schema should be passed to callback")
	}

	// Verify results were matched back to SkillMeta
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
	}
	if !names["top-skill"] || !names["hidden-skill"] {
		t.Fatalf("expected top-skill and hidden-skill, got %v", names)
	}
}

func TestAutoSkillLoader_SearchByAI_EmptyNeed(t *testing.T) {
	vfs := buildNestedTestVFS()
	mockCb := func(prompt string, schema string) ([]SkillSelection, error) {
		return nil, nil
	}
	loader, _ := NewAutoSkillLoader(
		WithAutoLoad_FileSystem(vfs),
		WithAutoLoad_SearchAICallback(mockCb),
	)

	_, err := loader.SearchByAI("")
	if err == nil {
		t.Fatal("empty user need should error")
	}
}

func TestAutoSkillLoader_SearchByAI_NoCallback(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	_, err := loader.SearchByAI("deploy things")
	if err == nil {
		t.Fatal("should error when no callback is set")
	}
}

func TestAutoSkillLoader_SearchByAI_NoSkills(t *testing.T) {
	mockCb := func(prompt string, schema string) ([]SkillSelection, error) {
		return nil, nil
	}
	loader, _ := NewAutoSkillLoader(WithAutoLoad_SearchAICallback(mockCb))

	results, err := loader.SearchByAI("deploy things")
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
	if results != nil {
		t.Fatal("no skills should return nil")
	}
}

// --- AutoSkillLoader: Concurrent safety ---

func TestAutoSkillLoader_ConcurrentAccess(t *testing.T) {
	vfs := buildNestedTestVFS()
	loader, _ := NewAutoSkillLoader(WithAutoLoad_FileSystem(vfs))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = loader.ListSkills()
			_, _ = loader.LoadSkill("top-skill")
			_, _ = loader.SearchSkills("deploy")
			_ = loader.HasSkills()
			_, _ = loader.GetFileSystem("top-skill")
		}()
	}
	wg.Wait()
}

// --- SearchByAI unit tests ---

func TestSearchByAI_NilCallback(t *testing.T) {
	skills := []*SkillMeta{
		{Name: "test-skill", Description: "A test skill"},
	}

	_, err := SearchByAI(skills, "test need", nil)
	if err == nil {
		t.Fatal("expected error when callback is nil")
	}
}

func TestSearchByAI_EmptySkills(t *testing.T) {
	cb := func(prompt string, schema string) ([]SkillSelection, error) {
		return nil, nil
	}
	results, err := SearchByAI(nil, "test need", cb)
	if err != nil {
		t.Fatalf("should not error with empty skills: %v", err)
	}
	if results != nil {
		t.Fatal("empty skills should return nil")
	}
}

func TestSearchByAI_EmptyNeed(t *testing.T) {
	skills := []*SkillMeta{
		{Name: "test-skill", Description: "A test skill"},
	}
	cb := func(prompt string, schema string) ([]SkillSelection, error) {
		return nil, nil
	}
	_, err := SearchByAI(skills, "", cb)
	if err == nil {
		t.Fatal("should error with empty need")
	}
}

func TestSearchByAI_MatchesResults(t *testing.T) {
	skills := []*SkillMeta{
		{Name: "skill-a", Description: "Skill A for building"},
		{Name: "skill-b", Description: "Skill B for testing"},
		{Name: "skill-c", Description: "Skill C for deploying"},
	}
	cb := func(prompt string, schema string) ([]SkillSelection, error) {
		return []SkillSelection{
			{SkillName: "skill-a", Reason: "good for building"},
			{SkillName: "skill-c", Reason: "good for deploying"},
			{SkillName: "nonexistent", Reason: "should be skipped"},
		}, nil
	}
	results, err := SearchByAI(skills, "build and deploy app", cb)
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 matched results, got %d", len(results))
	}
}

func TestSearchByAI_LimitsTo5(t *testing.T) {
	skills := make([]*SkillMeta, 10)
	sels := make([]SkillSelection, 10)
	for i := range skills {
		name := fmt.Sprintf("skill-%d", i)
		skills[i] = &SkillMeta{Name: name, Description: "desc"}
		sels[i] = SkillSelection{SkillName: name, Reason: "reason"}
	}
	cb := func(prompt string, schema string) ([]SkillSelection, error) {
		return sels, nil
	}
	results, err := SearchByAI(skills, "need", cb)
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected max 5 results, got %d", len(results))
	}
}
*/
