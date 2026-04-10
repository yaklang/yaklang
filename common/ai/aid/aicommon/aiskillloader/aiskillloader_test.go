package aiskillloader

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

// --- helpers ---

// buildTestSkillMD builds a valid SKILL.md content string.
func buildTestSkillMD(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body
}

// buildTestVFS builds a VirtualFS simulating a skills root directory.
// Each skill is a sub-directory containing a SKILL.md.
func buildTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("deploy-app/SKILL.md", buildTestSkillMD(
		"deploy-app",
		"Deploy the application to staging or production.",
		"# Deploy App\n\nRun `scripts/deploy.sh`.\n",
	))
	vfs.AddFile("deploy-app/scripts/deploy.sh", "#!/bin/bash\necho deploy")
	vfs.AddFile("code-review/SKILL.md", buildTestSkillMD(
		"code-review",
		"Perform an automated code review.",
		"# Code Review\n\nUse linters.\n",
	))
	vfs.AddFile("code-review/references/REFERENCE.md", "# Reference\nSome extra docs.")
	// Directory without SKILL.md should be skipped.
	vfs.AddFile("no-skill-dir/readme.txt", "nothing here")
	return vfs
}

// --- ParseSkillMeta tests ---

func TestParseSkillMeta_ValidFull(t *testing.T) {
	content := "---\nname: my-skill\ndescription: A useful skill.\nlicense: MIT\ncompatibility: linux\nmetadata:\n  author: test\ndisable-model-invocation: true\n---\n# My Skill\n\nDetailed instructions."
	meta, err := ParseSkillMeta(content)
	if err != nil {
		t.Fatalf("ParseSkillMeta failed: %v", err)
	}
	if meta.Name != "my-skill" {
		t.Fatalf("expected name 'my-skill', got %q", meta.Name)
	}
	if meta.Description != "A useful skill." {
		t.Fatalf("expected description 'A useful skill.', got %q", meta.Description)
	}
	if meta.License != "MIT" {
		t.Fatalf("expected license 'MIT', got %q", meta.License)
	}
	if meta.Compatibility != "linux" {
		t.Fatalf("expected compatibility 'linux', got %q", meta.Compatibility)
	}
	if meta.Metadata["author"] != "test" {
		t.Fatalf("expected metadata.author 'test', got %q", meta.Metadata["author"])
	}
	if !meta.DisableModelInvocation {
		t.Fatal("expected disable-model-invocation to be true")
	}
	if !strings.Contains(meta.Body, "# My Skill") {
		t.Fatalf("body should contain markdown content, got %q", meta.Body)
	}
}

func TestParseSkillMeta_MinimalValid(t *testing.T) {
	content := "---\nname: simple\ndescription: Simple skill.\n---\nBody here."
	meta, err := ParseSkillMeta(content)
	if err != nil {
		t.Fatalf("ParseSkillMeta failed: %v", err)
	}
	if meta.Name != "simple" {
		t.Fatalf("expected name 'simple', got %q", meta.Name)
	}
	if meta.Body != "Body here." {
		t.Fatalf("expected body 'Body here.', got %q", meta.Body)
	}
}

func TestParseSkillMeta_NoFrontmatter(t *testing.T) {
	_, err := ParseSkillMeta("# Just markdown")
	if err == nil {
		t.Fatal("expected error for content without frontmatter")
	}
}

func TestParseSkillMeta_MissingClosingDelimiter(t *testing.T) {
	_, err := ParseSkillMeta("---\nname: broken\n")
	if err == nil {
		t.Fatal("expected error for missing closing delimiter")
	}
}

func TestParseSkillMeta_EmptyName(t *testing.T) {
	// Missing name still parses but validation warns
	content := "---\ndescription: no name\n---\nbody"
	meta, err := ParseSkillMeta(content)
	if err != nil {
		t.Fatalf("ParseSkillMeta should not fail on empty name: %v", err)
	}
	if meta.Name != "" {
		t.Fatalf("expected empty name, got %q", meta.Name)
	}
	// Validate should fail
	if meta.Validate() == nil {
		t.Fatal("Validate should return error for empty name")
	}
}

func TestSkillMeta_BriefString(t *testing.T) {
	meta := &SkillMeta{
		Name:          "test-skill",
		Description:   "Test description",
		License:       "Apache-2.0",
		Compatibility: "all",
	}
	brief := meta.BriefString()
	if !strings.Contains(brief, "test-skill") {
		t.Fatal("BriefString should contain name")
	}
	if !strings.Contains(brief, "Test description") {
		t.Fatal("BriefString should contain description")
	}
	if !strings.Contains(brief, "Apache-2.0") {
		t.Fatal("BriefString should contain license")
	}
}

// --- FSSkillLoader tests ---

func TestFSSkillLoader_LoadFromVirtualFS(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	if !loader.HasSkills() {
		t.Fatal("loader should have skills")
	}

	skills := loader.AllSkillMetas()
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
}

func TestFSSkillLoader_LoadSkill(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	loaded, err := loader.LoadSkill("deploy-app")
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if loaded.Meta.Name != "deploy-app" {
		t.Fatalf("expected name 'deploy-app', got %q", loaded.Meta.Name)
	}
	if !strings.Contains(loaded.SkillMDContent, "Deploy App") {
		t.Fatalf("SKILL.md content should contain markdown body, got %q", loaded.SkillMDContent)
	}
	if loaded.FileSystem == nil {
		t.Fatal("FileSystem should not be nil")
	}
}

func TestFSSkillLoader_LoadSkill_NotFound(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	_, err = loader.LoadSkill("nonexistent")
	if err == nil {
		t.Fatal("LoadSkill should fail for nonexistent skill")
	}
}

func TestSkillsContextManager_SearchSkills(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	// Search by name
	results, err := mgr.SearchSkills("deploy")
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'deploy', got %d", len(results))
	}
	if results[0].Name != "deploy-app" {
		t.Fatalf("expected 'deploy-app', got %q", results[0].Name)
	}

	// Search by description
	results, err = mgr.SearchSkills("code review")
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'code review', got %d", len(results))
	}

	// Search with no match
	results, err = mgr.SearchSkills("zzzzz")
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for 'zzzzz', got %d", len(results))
	}
}

func TestFSSkillLoader_GetFileSystem(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	fs, err := loader.GetFileSystem("deploy-app")
	if err != nil {
		t.Fatalf("GetFileSystem failed: %v", err)
	}

	// Should be able to read files via the sub-filesystem
	content, err := fs.ReadFile("scripts/deploy.sh")
	if err != nil {
		t.Fatalf("ReadFile via sub-fs failed: %v", err)
	}
	if !strings.Contains(string(content), "echo deploy") {
		t.Fatalf("expected script content, got %q", string(content))
	}
}

func TestFSSkillLoader_SubDirFS_ReadOnly(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	fs, err := loader.GetFileSystem("deploy-app")
	if err != nil {
		t.Fatalf("GetFileSystem failed: %v", err)
	}

	// Write operations should fail
	if err := fs.WriteFile("test.txt", []byte("data"), 0644); err == nil {
		t.Fatal("WriteFile should fail on read-only filesystem")
	}
	if err := fs.Delete("scripts/deploy.sh"); err == nil {
		t.Fatal("Delete should fail on read-only filesystem")
	}
	if err := fs.MkdirAll("new-dir", 0755); err == nil {
		t.Fatal("MkdirAll should fail on read-only filesystem")
	}
	if err := fs.Rename("a", "b"); err == nil {
		t.Fatal("Rename should fail on read-only filesystem")
	}
}

func TestFSSkillLoader_EmptyVFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed on empty VFS: %v", err)
	}
	if loader.HasSkills() {
		t.Fatal("empty VFS should have no skills")
	}
	skills := loader.AllSkillMetas()
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

// --- RenderFileSystemTree tests ---

func TestRenderFileSystemTree_BasicTree(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SKILL.md", "content")
	vfs.AddFile("scripts/deploy.sh", "script")
	vfs.AddFile("references/REF.md", "ref")

	tree := RenderFileSystemTreeFull(vfs)
	if tree == "" {
		t.Fatal("tree should not be empty")
	}
	if !strings.Contains(tree, "SKILL.md") {
		t.Fatal("tree should contain SKILL.md")
	}
	if !strings.Contains(tree, "scripts/") {
		t.Fatal("tree should contain scripts/")
	}
}

func TestRenderFileSystemTree_FoldedLimit(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	// Add many files to exceed folded limit
	for i := 0; i < 100; i++ {
		vfs.AddFile("dir/file_"+strings.Repeat("x", 20)+"_"+string(rune('a'+i%26))+".txt", "data")
	}

	folded := RenderFileSystemTreeFolded(vfs)
	if len(folded) > FileTreeFoldedLimit+10 { // small tolerance for the truncation logic
		t.Fatalf("folded tree should be within limit, got %d bytes", len(folded))
	}
}

func TestRenderFileSystemTree_EmptyFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	tree := RenderFileSystemTreeFull(vfs)
	if tree != "" {
		t.Fatalf("empty filesystem tree should be empty, got %q", tree)
	}
}

// --- ViewWindow tests ---

func TestViewWindow_BasicRender(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5"
	vw := NewViewWindow("test-skill", "SKILL.md", content, "abc123")

	rendered, truncated := vw.Render()
	if truncated {
		t.Fatal("small content should not be truncated")
	}
	if !strings.Contains(rendered, "1 | line1") {
		t.Fatal("rendered should contain '1 | line1'")
	}
	if !strings.Contains(rendered, "5 | line5") {
		t.Fatal("rendered should contain '5 | line5'")
	}
	if !strings.Contains(rendered, "<|VIEW_WINDOW_abc123|>") {
		t.Fatal("rendered should contain VIEW_WINDOW header tag")
	}
	if !strings.Contains(rendered, "<|VIEW_WINDOW_END_abc123|>") {
		t.Fatal("rendered should contain VIEW_WINDOW footer tag")
	}
}

func TestViewWindow_OffsetRendering(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5"
	vw := NewViewWindow("test-skill", "SKILL.md", content, "abc123")

	vw.SetOffset(3)
	if vw.GetOffset() != 3 {
		t.Fatalf("expected offset 3, got %d", vw.GetOffset())
	}

	rendered, _ := vw.Render()
	// Should have leading ellipsis
	if !strings.Contains(rendered, "...\n3 | line3") {
		t.Fatal("rendered with offset 3 should start with '...' then '3 | line3'")
	}
	// Should not contain line 1
	if strings.Contains(rendered, "1 | line1") {
		t.Fatal("rendered with offset 3 should not contain '1 | line1'")
	}
}

func TestViewWindow_OffsetClamping(t *testing.T) {
	content := "a\nb\nc"
	vw := NewViewWindow("s", "f", content, "n")

	vw.SetOffset(-5)
	if vw.GetOffset() != 1 {
		t.Fatalf("negative offset should clamp to 1, got %d", vw.GetOffset())
	}

	vw.SetOffset(999)
	if vw.GetOffset() != 3 {
		t.Fatalf("offset beyond total lines should clamp to 3, got %d", vw.GetOffset())
	}
}

func TestViewWindow_EmptyContent(t *testing.T) {
	// Note: strings.Split("", "\n") returns [""], so empty string has 1 line.
	vw := NewViewWindow("s", "f", "", "n")
	rendered, truncated := vw.Render()
	// The empty string is still treated as one line (the empty line)
	if !strings.Contains(rendered, "1 | ") {
		t.Fatalf("empty content should still render one empty line, got %q", rendered)
	}
	if truncated {
		t.Fatal("empty content should not be truncated")
	}
	if vw.TotalLines() != 1 {
		t.Fatalf("expected 1 line for empty content, got %d", vw.TotalLines())
	}
}

func TestViewWindow_LargeContentTruncation(t *testing.T) {
	// Build content that exceeds 15KB
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, strings.Repeat("x", 100))
	}
	content := strings.Join(lines, "\n")
	vw := NewViewWindow("s", "f", content, "n")

	rendered, truncated := vw.Render()
	if !truncated {
		t.Fatal("large content should be truncated")
	}
	if !vw.IsTruncated {
		t.Fatal("IsTruncated flag should be set")
	}
	if len(rendered) > ViewWindowMaxBytes+100 { // tolerance
		t.Fatalf("rendered size should be within limit, got %d", len(rendered))
	}
	// Should have trailing ellipsis
	if !strings.Contains(rendered, "...\n<|VIEW_WINDOW_END_") {
		t.Fatal("truncated content should end with '...' before end tag")
	}
}

func TestViewWindow_RenderWithInfo(t *testing.T) {
	content := "line1\nline2"
	vw := NewViewWindow("my-skill", "SKILL.md", content, "nonce1")
	info := vw.RenderWithInfo()
	if !strings.Contains(info, "File: SKILL.md (Skill: my-skill)") {
		t.Fatal("RenderWithInfo should contain file info header")
	}
	if !strings.Contains(info, "Total Lines: 2") {
		t.Fatal("RenderWithInfo should contain total lines")
	}
}

func TestGenerateNonce_Deterministic(t *testing.T) {
	n1 := GenerateNonce("skill-a", "SKILL.md")
	n2 := GenerateNonce("skill-a", "SKILL.md")
	if n1 != n2 {
		t.Fatal("GenerateNonce should be deterministic")
	}
	if len(n1) != 8 {
		t.Fatalf("nonce should be 8 chars, got %d", len(n1))
	}

	n3 := GenerateNonce("skill-b", "SKILL.md")
	if n1 == n3 {
		t.Fatal("different inputs should produce different nonces")
	}
}

// --- SkillsContextManager tests ---

func TestSkillsContextManager_NilLoader(t *testing.T) {
	mgr := NewSkillsContextManager(nil)
	if mgr.HasRegisteredSkills() {
		t.Fatal("nil loader should report no skills")
	}
	err := mgr.LoadSkill("anything")
	if err == nil {
		t.Fatal("LoadSkill with nil loader should fail")
	}
}

func TestSkillsContextManager_LoadAndRender(t *testing.T) {
	vfs := buildTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if !mgr.HasRegisteredSkills() {
		t.Fatal("manager should report skills available")
	}

	// Render before loading any skill - should show available skills hint
	rendered := mgr.Render("test_nonce")
	if !strings.Contains(rendered, "SKILLS_CONTEXT_test_nonce") {
		t.Fatal("render should contain SKILLS_CONTEXT tags")
	}
	if !strings.Contains(rendered, "deploy-app") {
		t.Fatal("render should list available skills")
	}

	// Load a skill
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered = mgr.Render("test_nonce")
	if !strings.Contains(rendered, "=== Skill: deploy-app ===") {
		t.Fatalf("rendered should contain full skill header, got: %s", rendered)
	}
	if !strings.Contains(rendered, "VIEW_WINDOW") {
		t.Fatal("rendered should contain VIEW_WINDOW for SKILL.md content")
	}
}

func TestSkillsContextManager_LoadDuplicate(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("first LoadSkill failed: %v", err)
	}
	// Loading same skill again should not fail
	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("duplicate LoadSkill should not fail: %v", err)
	}
}

func TestSkillsContextManager_LoadNonexistent(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	err := mgr.LoadSkill("nonexistent")
	if err == nil {
		t.Fatal("LoadSkill for nonexistent should fail")
	}
}

func TestSkillsContextManager_FoldingOnOverflow(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	mgr.SetMaxTokens(50)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	if err := mgr.LoadSkill("code-review"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered := mgr.Render("nonce")
	// At least one skill should be folded
	if !strings.Contains(rendered, "(folded)") {
		t.Fatal("with small limit, at least one skill should be folded")
	}
	// HasTruncatedViews should be true due to folding
	if !mgr.HasTruncatedViews() {
		t.Fatal("HasTruncatedViews should be true when skills are folded")
	}
}

func TestSkillsContextManager_ChangeViewOffset(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	// Change offset for SKILL.md (default file path)
	if err := mgr.ChangeViewOffset("deploy-app", "", 3); err != nil {
		t.Fatalf("ChangeViewOffset failed: %v", err)
	}

	// Change offset for nonexistent skill
	if err := mgr.ChangeViewOffset("nonexistent", "", 1); err == nil {
		t.Fatal("ChangeViewOffset for nonexistent skill should fail")
	}
}

func TestSkillsContextManager_ChangeViewOffset_NewFile(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	if err := mgr.LoadSkill("deploy-app"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	// Open a new file that wasn't in the initial view
	if err := mgr.ChangeViewOffset("deploy-app", "scripts/deploy.sh", 1); err != nil {
		t.Fatalf("ChangeViewOffset for new file failed: %v", err)
	}

	rendered := mgr.Render("n")
	if !strings.Contains(rendered, "scripts/deploy.sh") {
		t.Fatal("rendered should include the newly opened file")
	}
}

func TestSkillsContextManager_EmptyRender(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	rendered := mgr.Render("n")
	if rendered != "" {
		t.Fatalf("render with no skills should be empty, got %q", rendered)
	}
}

func TestSkillsContextManager_HasTruncatedViews_NoSkillsLoaded(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	if mgr.HasTruncatedViews() {
		t.Fatal("HasTruncatedViews should be false when no skills are loaded")
	}
}

// --- ResolveIncludes tests ---

func TestResolveIncludes_Basic(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("extra.md", "# Extra Content\nSome included text.")

	content := "Before\n<!-- include: extra.md -->\nAfter"
	resolved := ResolveIncludes(content, vfs)

	if !strings.Contains(resolved, "# Extra Content") {
		t.Errorf("resolved content should contain included file, got %q", resolved)
	}
	if !strings.Contains(resolved, "Before") || !strings.Contains(resolved, "After") {
		t.Error("surrounding content should be preserved")
	}
	if strings.Contains(resolved, "<!-- include:") {
		t.Error("include directive should be replaced")
	}
}

func TestResolveIncludes_MultipleIncludes(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a.md", "Content A")
	vfs.AddFile("b.md", "Content B")

	content := "Start\n<!-- include: a.md -->\nMiddle\n<!-- include: b.md -->\nEnd"
	resolved := ResolveIncludes(content, vfs)

	if !strings.Contains(resolved, "Content A") {
		t.Error("should contain Content A")
	}
	if !strings.Contains(resolved, "Content B") {
		t.Error("should contain Content B")
	}
	if !strings.Contains(resolved, "Middle") {
		t.Error("surrounding text should be preserved")
	}
}

func TestResolveIncludes_MissingFile(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	content := "Before\n<!-- include: nonexistent.md -->\nAfter"
	resolved := ResolveIncludes(content, vfs)

	if !strings.Contains(resolved, "include error") {
		t.Errorf("should contain error marker for missing file, got %q", resolved)
	}
	if !strings.Contains(resolved, "nonexistent.md") {
		t.Error("error should mention the missing file name")
	}
}

func TestResolveIncludes_NilFS(t *testing.T) {
	content := "Before\n<!-- include: file.md -->\nAfter"
	resolved := ResolveIncludes(content, nil)

	if resolved != content {
		t.Error("nil filesystem should return content unchanged")
	}
}

func TestResolveIncludes_NoDirectives(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	content := "Just regular markdown content."
	resolved := ResolveIncludes(content, vfs)

	if resolved != content {
		t.Error("content without directives should be unchanged")
	}
}

func TestResolveIncludes_SpacingVariants(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("file.md", "included")

	tests := []string{
		"<!-- include: file.md -->",
		"<!--include: file.md-->",
		"<!--  include:  file.md  -->",
		"<!-- include:file.md -->",
	}

	for _, directive := range tests {
		resolved := ResolveIncludes(directive, vfs)
		if !strings.Contains(resolved, "included") {
			t.Errorf("directive %q should resolve, got %q", directive, resolved)
		}
	}
}

func TestResolveIncludes_LargeFile(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	largeContent := strings.Repeat("x", ViewWindowMaxBytes+1000)
	vfs.AddFile("large.md", largeContent)

	content := "<!-- include: large.md -->"
	resolved := ResolveIncludes(content, vfs)

	if len(resolved) > ViewWindowMaxBytes+200 {
		t.Errorf("included content should be truncated, got size %d", len(resolved))
	}
	if !strings.Contains(resolved, "truncated") {
		t.Error("truncated include should have truncation note")
	}
}

func TestResolveIncludes_SkillMDWithIncludes_LazyLoading(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("inc-skill/SKILL.md", buildTestSkillMD("inc-skill", "Skill with includes",
		"# Main\n\n<!-- include: guide.md -->\n\nEnd"))
	vfs.AddFile("inc-skill/guide.md", "# Guide\nStep 1\nStep 2")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("inc-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered := mgr.Render("n")

	// Include directives should NOT be eagerly expanded
	if strings.Contains(rendered, "# Guide") {
		t.Error("rendered context should NOT contain eagerly expanded guide content")
	}
	if strings.Contains(rendered, "Step 1") {
		t.Error("rendered context should NOT contain eagerly expanded guide steps")
	}

	// Instead, should contain a resource loading hint
	if !strings.Contains(rendered, "load_skill_resources") {
		t.Error("rendered context should contain load_skill_resources hint for included file")
	}
	if !strings.Contains(rendered, "@inc-skill/guide.md") {
		t.Error("rendered context should contain the resource path @inc-skill/guide.md")
	}
}

// --- TransformIncludesToResourceHints tests ---

func TestTransformIncludesToResourceHints_Basic(t *testing.T) {
	content := "Before\n<!-- include: guide.md -->\nAfter"
	result := TransformIncludesToResourceHints(content, "my-skill")

	if strings.Contains(result, "<!-- include:") {
		t.Error("include directive should be replaced")
	}
	if !strings.Contains(result, "load_skill_resources") {
		t.Error("result should contain load_skill_resources hint")
	}
	if !strings.Contains(result, "@my-skill/guide.md") {
		t.Error("result should contain the resource path @my-skill/guide.md")
	}
	if !strings.Contains(result, "Before") || !strings.Contains(result, "After") {
		t.Error("surrounding content should be preserved")
	}
}

func TestTransformIncludesToResourceHints_MultipleIncludes(t *testing.T) {
	content := "Start\n<!-- include: a.md -->\nMiddle\n<!-- include: b.md -->\nEnd"
	result := TransformIncludesToResourceHints(content, "test-skill")

	if !strings.Contains(result, "@test-skill/a.md") {
		t.Error("should contain hint for a.md")
	}
	if !strings.Contains(result, "@test-skill/b.md") {
		t.Error("should contain hint for b.md")
	}
	if !strings.Contains(result, "Middle") {
		t.Error("surrounding text should be preserved")
	}
	if strings.Contains(result, "<!-- include:") {
		t.Error("no include directives should remain")
	}
}

func TestTransformIncludesToResourceHints_NoDirectives(t *testing.T) {
	content := "Just regular markdown content."
	result := TransformIncludesToResourceHints(content, "my-skill")

	if result != content {
		t.Error("content without directives should be unchanged")
	}
}

func TestTransformIncludesToResourceHints_SubdirectoryPath(t *testing.T) {
	content := "<!-- include: references/detailed-guide.md -->"
	result := TransformIncludesToResourceHints(content, "deploy-app")

	if !strings.Contains(result, "@deploy-app/references/detailed-guide.md") {
		t.Errorf("should contain full resource path, got %q", result)
	}
}

func TestTransformIncludesToResourceHints_SpacingVariants(t *testing.T) {
	tests := []string{
		"<!-- include: file.md -->",
		"<!--include: file.md-->",
		"<!--  include:  file.md  -->",
		"<!-- include:file.md -->",
	}

	for _, directive := range tests {
		result := TransformIncludesToResourceHints(directive, "s")
		if !strings.Contains(result, "load_skill_resources") {
			t.Errorf("directive %q should be transformed, got %q", directive, result)
		}
		if !strings.Contains(result, "@s/file.md") {
			t.Errorf("directive %q should produce @s/file.md hint, got %q", directive, result)
		}
	}
}

// --- DetectCrossSkillReferences tests ---

func TestDetectCrossSkillReferences_Basic(t *testing.T) {
	content := "See ../recon/SKILL.md and ../exploitation/guide.md for details."
	refs := DetectCrossSkillReferences(content, "pentest-master")

	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %d: %v", len(refs), refs)
	}
	if refs[0] != "exploitation" || refs[1] != "recon" {
		t.Errorf("expected [exploitation, recon], got %v", refs)
	}
}

func TestDetectCrossSkillReferences_ExcludesSelf(t *testing.T) {
	content := "See ../recon/SKILL.md and ../recon/other.md for more."
	refs := DetectCrossSkillReferences(content, "recon")

	if len(refs) != 0 {
		t.Errorf("self-references should be excluded, got %v", refs)
	}
}

func TestDetectCrossSkillReferences_Deduplicate(t *testing.T) {
	content := "Use ../toolbox/a.md and ../toolbox/b.md and ../recon/c.md"
	refs := DetectCrossSkillReferences(content, "test")

	if len(refs) != 2 {
		t.Fatalf("expected 2 unique references, got %d: %v", len(refs), refs)
	}
}

func TestDetectCrossSkillReferences_NoRefs(t *testing.T) {
	content := "This skill has no cross-references."
	refs := DetectCrossSkillReferences(content, "test")

	if len(refs) != 0 {
		t.Errorf("expected 0 references, got %v", refs)
	}
}

func TestDetectCrossSkillReferences_InRenderedOutput(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("xref-skill/SKILL.md", buildTestSkillMD("xref-skill", "Cross-ref test",
		"# XRef\n\nSee ../recon/SKILL.md and ../exploitation/guide.md\n"))

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("xref-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered := mgr.Render("n")
	if !strings.Contains(rendered, "Related Skills") {
		t.Error("rendered output should contain Related Skills hint")
	}
	if !strings.Contains(rendered, "recon") {
		t.Error("rendered output should mention recon as related skill")
	}
	if !strings.Contains(rendered, "exploitation") {
		t.Error("rendered output should mention exploitation as related skill")
	}
}

// --- LoadSkills batch tests ---

func TestSkillsContextManager_LoadSkills(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	results := mgr.LoadSkills([]string{"deploy-app", "code-review"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["deploy-app"] != nil {
		t.Errorf("deploy-app should load successfully: %v", results["deploy-app"])
	}
	if results["code-review"] != nil {
		t.Errorf("code-review should load successfully: %v", results["code-review"])
	}

	skills := mgr.GetCurrentSelectedSkills()
	if len(skills) != 2 {
		t.Errorf("expected 2 loaded skills, got %d", len(skills))
	}
}

func TestSkillsContextManager_LoadSkills_PartialFailure(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	results := mgr.LoadSkills([]string{"deploy-app", "nonexistent"})
	if results["deploy-app"] != nil {
		t.Errorf("deploy-app should succeed: %v", results["deploy-app"])
	}
	if results["nonexistent"] == nil {
		t.Error("nonexistent should fail")
	}

	if !mgr.IsSkillLoaded("deploy-app") {
		t.Error("deploy-app should be loaded despite other failures")
	}
}

func TestSkillsContextManager_LoadSkills_EmptyNames(t *testing.T) {
	vfs := buildTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	results := mgr.LoadSkills([]string{"", "  ", "deploy-app"})
	if len(results) != 1 {
		t.Errorf("expected 1 result (empty names skipped), got %d", len(results))
	}
	if results["deploy-app"] != nil {
		t.Errorf("deploy-app should succeed: %v", results["deploy-app"])
	}
}
