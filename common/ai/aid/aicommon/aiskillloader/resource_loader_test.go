package aiskillloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

// --- ParseSkillResourcePath tests ---

func TestParseSkillResourcePath_Valid(t *testing.T) {
	tests := []struct {
		input     string
		wantSkill string
		wantPath  string
	}{
		{"@recon/osint.md", "recon", "osint.md"},
		{"@recon/docs/guide.md", "recon", "docs/guide.md"},
		{"@pentest-master/methodology.md", "pentest-master", "methodology.md"},
		{"@toolbox/scripts/deploy.sh", "toolbox", "scripts/deploy.sh"},
	}

	for _, tt := range tests {
		skill, path, err := ParseSkillResourcePath(tt.input)
		if err != nil {
			t.Errorf("ParseSkillResourcePath(%q) failed: %v", tt.input, err)
			continue
		}
		if skill != tt.wantSkill {
			t.Errorf("ParseSkillResourcePath(%q) skill = %q, want %q", tt.input, skill, tt.wantSkill)
		}
		if path != tt.wantPath {
			t.Errorf("ParseSkillResourcePath(%q) path = %q, want %q", tt.input, path, tt.wantPath)
		}
	}
}

func TestParseSkillResourcePath_SkillOnly(t *testing.T) {
	skill, path, err := ParseSkillResourcePath("@recon")
	if err != nil {
		t.Fatalf("ParseSkillResourcePath(@recon) failed: %v", err)
	}
	if skill != "recon" {
		t.Errorf("skill = %q, want %q", skill, "recon")
	}
	if path != "" {
		t.Errorf("path = %q, want empty", path)
	}
}

func TestParseSkillResourcePath_Invalid(t *testing.T) {
	tests := []string{
		"recon/osint.md",
		"",
		"@",
		"@ ",
	}

	for _, input := range tests {
		_, _, err := ParseSkillResourcePath(input)
		if err == nil {
			t.Errorf("ParseSkillResourcePath(%q) should fail", input)
		}
	}
}

// --- resolveFilePath tests ---

func buildResourceTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SKILL.md", buildTestSkillMD("test-res", "Test resource skill", "body"))
	vfs.AddFile("osint.md", "# OSINT Guide\nContent here.")
	vfs.AddFile("docs/guide.md", "# Guide\nDetailed guide.")
	vfs.AddFile("docs/reference.md", "# Reference\nRef content.")
	vfs.AddFile("scripts/deploy.sh", "#!/bin/bash\necho deploy")
	vfs.AddFile("deep/nested/docs.md", "# Deep Docs\nNested content.")
	return vfs
}

func TestResolveFilePath_ExactMatch(t *testing.T) {
	vfs := buildResourceTestVFS()
	paths, fuzzy, err := resolveFilePath(vfs, "osint.md")
	if err != nil {
		t.Fatalf("resolveFilePath exact match failed: %v", err)
	}
	if fuzzy {
		t.Error("exact match should not be fuzzy")
	}
	if len(paths) != 1 || paths[0] != "osint.md" {
		t.Errorf("expected [osint.md], got %v", paths)
	}
}

func TestResolveFilePath_DirMatch(t *testing.T) {
	vfs := buildResourceTestVFS()
	paths, _, err := resolveFilePath(vfs, "docs")
	if err != nil {
		t.Fatalf("resolveFilePath dir match failed: %v", err)
	}
	if len(paths) < 2 {
		t.Errorf("expected at least 2 files matching 'docs', got %d: %v", len(paths), paths)
	}
	hasGuide := false
	hasRef := false
	for _, p := range paths {
		if strings.Contains(p, "guide.md") {
			hasGuide = true
		}
		if strings.Contains(p, "reference.md") {
			hasRef = true
		}
	}
	if !hasGuide || !hasRef {
		t.Errorf("should find guide.md and reference.md in docs/, got %v", paths)
	}
}

func TestResolveFilePath_FuzzyByFilename(t *testing.T) {
	vfs := buildResourceTestVFS()
	paths, fuzzy, err := resolveFilePath(vfs, "nonexistent/docs.md")
	if err != nil {
		t.Fatalf("resolveFilePath fuzzy match failed: %v", err)
	}
	if !fuzzy {
		t.Error("should be fuzzy matched")
	}
	found := false
	for _, p := range paths {
		if strings.Contains(p, "docs.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("fuzzy match should find docs.md, got %v", paths)
	}
}

func TestResolveFilePath_FuzzyByNameWithoutExt(t *testing.T) {
	vfs := buildResourceTestVFS()
	paths, fuzzy, err := resolveFilePath(vfs, "deploy.sh")
	if err != nil {
		t.Fatalf("resolveFilePath fuzzy for deploy failed: %v", err)
	}
	_ = fuzzy
	found := false
	for _, p := range paths {
		if strings.Contains(p, "deploy.sh") {
			found = true
		}
	}
	if !found {
		t.Errorf("should find deploy.sh, got %v", paths)
	}
}

func TestResolveFilePath_FuzzyDirMatch(t *testing.T) {
	vfs := buildResourceTestVFS()
	paths, fuzzy, err := resolveFilePath(vfs, "nonexist/docs")
	if err != nil {
		t.Fatalf("resolveFilePath fuzzy dir match failed: %v", err)
	}
	if !fuzzy {
		t.Error("should be fuzzy matched")
	}
	if len(paths) < 1 {
		t.Error("fuzzy dir match should find files")
	}
}

func TestResolveFilePath_NotFound(t *testing.T) {
	vfs := buildResourceTestVFS()
	_, _, err := resolveFilePath(vfs, "completely_nonexistent_xyz.txt")
	if err == nil {
		t.Error("should fail for completely nonexistent file")
	}
}

// --- LoadSkillResource tests ---

func TestLoadSkillResource_ExactFile(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-res/SKILL.md", buildTestSkillMD("test-res", "Test resource skill", "body"))
	vfs.AddFile("test-res/osint.md", "# OSINT\nContent line 1\nContent line 2\n")
	vfs.AddFile("test-res/scripts/deploy.sh", "#!/bin/bash\necho deploy")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)

	if err := mgr.LoadSkill("test-res"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	result, err := mgr.LoadSkillResource("test-res", "osint.md")
	if err != nil {
		t.Fatalf("LoadSkillResource failed: %v", err)
	}
	if result.SkillName != "test-res" {
		t.Errorf("SkillName = %q, want %q", result.SkillName, "test-res")
	}
	if result.FuzzyMatched {
		t.Error("exact match should not be fuzzy")
	}
	if result.ContentSize == 0 {
		t.Error("ContentSize should be > 0")
	}
	if result.TotalLines == 0 {
		t.Error("TotalLines should be > 0")
	}

	rendered := mgr.Render("test")
	if !strings.Contains(rendered, "osint.md") {
		t.Error("rendered context should contain osint.md after LoadSkillResource")
	}
}

func TestLoadSkillResource_FuzzyMatch(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-res/SKILL.md", buildTestSkillMD("test-res", "Test", "body"))
	vfs.AddFile("test-res/deep/nested/reference.md", "# Reference\nContent here")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("test-res"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	result, err := mgr.LoadSkillResource("test-res", "reference.md")
	if err != nil {
		t.Fatalf("LoadSkillResource fuzzy failed: %v", err)
	}
	if !result.FuzzyMatched {
		t.Error("should be fuzzy matched since reference.md is not at root")
	}
	if !strings.Contains(result.MatchedPath, "reference.md") {
		t.Errorf("MatchedPath should contain reference.md, got %q", result.MatchedPath)
	}
}

func TestLoadSkillResource_AutoLoadSkill(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("auto-skill/SKILL.md", buildTestSkillMD("auto-skill", "Auto load test", "body"))
	vfs.AddFile("auto-skill/data.md", "# Data\nSome data")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)

	result, err := mgr.LoadSkillResource("auto-skill", "data.md")
	if err != nil {
		t.Fatalf("LoadSkillResource auto-load failed: %v", err)
	}
	if result.SkillName != "auto-skill" {
		t.Errorf("SkillName = %q, want %q", result.SkillName, "auto-skill")
	}

	if !mgr.IsSkillLoaded("auto-skill") {
		t.Error("auto-skill should be loaded after LoadSkillResource")
	}
}

func TestLoadSkillResource_NotFound(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-res/SKILL.md", buildTestSkillMD("test-res", "Test", "body"))

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("test-res"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	_, err = mgr.LoadSkillResource("test-res", "nonexistent_xyz.md")
	if err == nil {
		t.Error("should fail for nonexistent file")
	}
}

func TestLoadSkillResource_EmptyPath(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-res/SKILL.md", buildTestSkillMD("test-res", "Test", "body"))

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("test-res"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	_, err = mgr.LoadSkillResource("test-res", "")
	if err == nil {
		t.Error("should fail for empty path")
	}
}

func TestLoadSkillResource_SkillNotFound(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	_, err := mgr.LoadSkillResource("nonexistent", "file.md")
	if err == nil {
		t.Error("should fail for nonexistent skill")
	}
}

// --- FormatResourceLoadSummary tests ---

func TestFormatResourceLoadSummary(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:    "recon",
		FilePath:     "osint.md",
		ContentSize:  2048,
		TotalLines:   50,
		IsTruncated:  false,
		FuzzyMatched: false,
		MatchedPath:  "osint.md",
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "recon") {
		t.Error("summary should contain skill name")
	}
	if !strings.Contains(summary, "50 lines") {
		t.Error("summary should contain line count")
	}
	if !strings.Contains(summary, "2.0KB") {
		t.Error("summary should contain size in KB")
	}
}

func TestFormatResourceLoadSummary_Fuzzy(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:    "recon",
		FilePath:     "osint.md",
		ContentSize:  1024,
		TotalLines:   20,
		FuzzyMatched: true,
		MatchedPath:  "docs/osint.md",
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "fuzzy matched") {
		t.Error("summary should mention fuzzy match")
	}
	if !strings.Contains(summary, "docs/osint.md") {
		t.Error("summary should contain matched path")
	}
}

func TestFormatResourceLoadSummary_Truncated(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:   "recon",
		FilePath:    "large.md",
		ContentSize: 40960,
		TotalLines:  1000,
		IsTruncated: true,
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "truncated") {
		t.Error("summary should mention truncation")
	}
}

// --- IsScriptExtension tests ---

func TestIsScriptExtension(t *testing.T) {
	scriptExts := []string{".sh", ".bash", ".py", ".yak", ".js", ".ts", ".go", ".rb", ".pl", ".lua", ".ps1", ".bat", ".cmd"}
	for _, ext := range scriptExts {
		if !IsScriptExtension(ext) {
			t.Errorf("IsScriptExtension(%q) should be true", ext)
		}
	}

	if !IsScriptExtension(".SH") {
		t.Error("IsScriptExtension should be case-insensitive")
	}
	if !IsScriptExtension(".Py") {
		t.Error("IsScriptExtension should be case-insensitive for .Py")
	}

	nonScriptExts := []string{".md", ".txt", ".json", ".yaml", ".xml", ".html", ".csv", ".pdf", ""}
	for _, ext := range nonScriptExts {
		if IsScriptExtension(ext) {
			t.Errorf("IsScriptExtension(%q) should be false", ext)
		}
	}
}

// --- ScriptTypeLabel tests ---

func TestScriptTypeLabel(t *testing.T) {
	tests := map[string]string{
		".sh":  "shell script",
		".py":  "python script",
		".yak": "yak script",
		".go":  "go source",
		".js":  "javascript",
	}
	for ext, want := range tests {
		got := ScriptTypeLabel(ext)
		if got != want {
			t.Errorf("ScriptTypeLabel(%q) = %q, want %q", ext, got, want)
		}
	}

	if label := ScriptTypeLabel(".SH"); label != "shell script" {
		t.Errorf("ScriptTypeLabel should be case-insensitive, got %q", label)
	}

	if label := ScriptTypeLabel(".xyz"); label != "script" {
		t.Errorf("ScriptTypeLabel for unknown ext should be 'script', got %q", label)
	}
}

// --- ResolveAbsoluteFilePath tests ---

func TestResolveAbsoluteFilePath_LocalFS(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho hello"), 0755); err != nil {
		t.Fatalf("failed to write test script: %v", err)
	}

	localFS := filesys.NewRelLocalFs(tmpDir)
	absPath, ok := ResolveAbsoluteFilePath(localFS, "test.sh")
	if !ok {
		t.Fatal("should resolve absolute path for local filesystem")
	}
	if absPath != scriptPath {
		t.Errorf("absPath = %q, want %q", absPath, scriptPath)
	}
}

func TestResolveAbsoluteFilePath_LocalFS_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	localFS := filesys.NewRelLocalFs(tmpDir)
	_, ok := ResolveAbsoluteFilePath(localFS, "nonexistent.sh")
	if ok {
		t.Error("should not resolve path for nonexistent file")
	}
}

func TestResolveAbsoluteFilePath_SubDirFS(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "my-skill")
	scriptsDir := filepath.Join(skillDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}
	scriptFile := filepath.Join(scriptsDir, "run.sh")
	if err := os.WriteFile(scriptFile, []byte("#!/bin/bash\necho run"), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	rootFS := filesys.NewRelLocalFs(tmpDir)
	sfs := &subDirFS{parent: rootFS, subDir: "my-skill", dirName: "my-skill"}
	absPath, ok := ResolveAbsoluteFilePath(sfs, "scripts/run.sh")
	if !ok {
		t.Fatal("should resolve path through subDirFS -> RelLocalFs")
	}
	if absPath != scriptFile {
		t.Errorf("absPath = %q, want %q", absPath, scriptFile)
	}
}

func TestResolveAbsoluteFilePath_VirtualFS(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("script.sh", "#!/bin/bash")
	_, ok := ResolveAbsoluteFilePath(vfs, "script.sh")
	if ok {
		t.Error("VirtualFS should not resolve to absolute path")
	}
}

// --- LoadSkillResourceAsScript tests ---

func TestLoadSkillResourceAsScript_LocalFS_ResolvesPath(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "deploy-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}
	skillMD := buildTestSkillMD("deploy-skill", "Deploy skill", "# Deploy\nUse scripts/deploy.sh")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}
	scriptContent := "#!/bin/bash\necho deploying..."
	scriptFile := filepath.Join(skillDir, "scripts", "deploy.sh")
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write deploy.sh: %v", err)
	}

	loader, err := NewLocalSkillLoader(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	materializeCalled := false
	materialize := func(name, ext string, data any) string {
		materializeCalled = true
		return "/should/not/be/called"
	}

	result, err := mgr.LoadSkillResourceAsScript("deploy-skill", "scripts/deploy.sh", materialize)
	if err != nil {
		t.Fatalf("LoadSkillResourceAsScript failed: %v", err)
	}

	if result.ResourceType != "script" {
		t.Errorf("ResourceType = %q, want 'script'", result.ResourceType)
	}
	if result.AbsolutePath != scriptFile {
		t.Errorf("AbsolutePath = %q, want %q", result.AbsolutePath, scriptFile)
	}
	if result.MaterializedToArtifacts {
		t.Error("should not materialize when local path is available")
	}
	if materializeCalled {
		t.Error("materialize func should not be called for local FS")
	}
	if result.ContentSize != len(scriptContent) {
		t.Errorf("ContentSize = %d, want %d", result.ContentSize, len(scriptContent))
	}

	rendered := mgr.Render("test")
	if !strings.Contains(rendered, "Script Resource Reference") {
		t.Error("rendered context should contain script reference summary")
	}
	if !strings.Contains(rendered, scriptFile) {
		t.Error("rendered context should contain the absolute path")
	}
}

func TestLoadSkillResourceAsScript_VirtualFS_Materializes(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("virt-skill/SKILL.md", buildTestSkillMD("virt-skill", "Virtual skill", "body"))
	vfs.AddFile("virt-skill/tools/scan.py", "#!/usr/bin/env python3\nimport os\nprint('scanning')")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	var materializedName, materializedExt string
	var materializedData []byte
	materialize := func(name, ext string, data any) string {
		materializedName = name
		materializedExt = ext
		if b, ok := data.([]byte); ok {
			materializedData = b
		}
		return "/tmp/artifacts/scan_20260303.py"
	}

	result, err := mgr.LoadSkillResourceAsScript("virt-skill", "tools/scan.py", materialize)
	if err != nil {
		t.Fatalf("LoadSkillResourceAsScript failed: %v", err)
	}

	if result.ResourceType != "script" {
		t.Errorf("ResourceType = %q, want 'script'", result.ResourceType)
	}
	if !result.MaterializedToArtifacts {
		t.Error("should be materialized for virtual FS")
	}
	if result.AbsolutePath != "/tmp/artifacts/scan_20260303.py" {
		t.Errorf("AbsolutePath = %q, want artifacts path", result.AbsolutePath)
	}
	if materializedName != "scan" {
		t.Errorf("materialize name = %q, want 'scan'", materializedName)
	}
	if materializedExt != ".py" {
		t.Errorf("materialize ext = %q, want '.py'", materializedExt)
	}
	if !strings.Contains(string(materializedData), "scanning") {
		t.Error("materialized data should contain the script content")
	}

	rendered := mgr.Render("test")
	if !strings.Contains(rendered, "materialized") {
		t.Error("rendered context should mention materialization")
	}
}

func TestLoadSkillResourceAsScript_FuzzyMatch(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("fuzz-skill/SKILL.md", buildTestSkillMD("fuzz-skill", "Fuzzy test", "body"))
	vfs.AddFile("fuzz-skill/deep/nested/run.sh", "#!/bin/bash\necho run")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	materialize := func(name, ext string, data any) string {
		return "/tmp/artifacts/run_materialized.sh"
	}

	result, err := mgr.LoadSkillResourceAsScript("fuzz-skill", "run.sh", materialize)
	if err != nil {
		t.Fatalf("LoadSkillResourceAsScript fuzzy failed: %v", err)
	}
	if !result.FuzzyMatched {
		t.Error("should be fuzzy matched")
	}
	if !strings.Contains(result.MatchedPath, "run.sh") {
		t.Errorf("MatchedPath should contain run.sh, got %q", result.MatchedPath)
	}
}

func TestLoadSkillResourceAsScript_EmptyPath(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-skill/SKILL.md", buildTestSkillMD("test-skill", "Test", "body"))
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	_, err := mgr.LoadSkillResourceAsScript("test-skill", "", nil)
	if err == nil {
		t.Error("should fail for empty path")
	}
}

func TestLoadSkillResourceAsScript_NotFound(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("test-skill/SKILL.md", buildTestSkillMD("test-skill", "Test", "body"))
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	_, err := mgr.LoadSkillResourceAsScript("test-skill", "nonexistent.sh", nil)
	if err == nil {
		t.Error("should fail for nonexistent file")
	}
}

func TestLoadSkillResourceAsScript_NilMaterializeFunc(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("vm-skill/SKILL.md", buildTestSkillMD("vm-skill", "VM skill", "body"))
	vfs.AddFile("vm-skill/run.sh", "#!/bin/bash")
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	_, err := mgr.LoadSkillResourceAsScript("vm-skill", "run.sh", nil)
	if err == nil {
		t.Error("should fail when materialize is nil and FS is virtual")
	}
}

func TestLoadSkillResourceAsScript_MaterializeReturnsEmpty(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("em-skill/SKILL.md", buildTestSkillMD("em-skill", "Empty emit", "body"))
	vfs.AddFile("em-skill/run.sh", "#!/bin/bash")
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	materialize := func(name, ext string, data any) string { return "" }
	_, err := mgr.LoadSkillResourceAsScript("em-skill", "run.sh", materialize)
	if err == nil {
		t.Error("should fail when materialize returns empty path")
	}
}

func TestLoadSkillResourceAsScript_AutoLoadSkill(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("auto-skill/SKILL.md", buildTestSkillMD("auto-skill", "Auto", "body"))
	vfs.AddFile("auto-skill/setup.sh", "#!/bin/bash\nsetup")
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	materialize := func(name, ext string, data any) string { return "/tmp/setup.sh" }
	result, err := mgr.LoadSkillResourceAsScript("auto-skill", "setup.sh", materialize)
	if err != nil {
		t.Fatalf("auto-load failed: %v", err)
	}
	if result.SkillName != "auto-skill" {
		t.Errorf("SkillName = %q, want 'auto-skill'", result.SkillName)
	}
	if !mgr.IsSkillLoaded("auto-skill") {
		t.Error("skill should be auto-loaded")
	}
}

// --- FormatResourceLoadSummary script tests ---

func TestFormatResourceLoadSummary_Script(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:    "deploy",
		FilePath:     "scripts/deploy.sh",
		ContentSize:  2048,
		ResourceType: "script",
		AbsolutePath: "/path/to/deploy.sh",
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "script resource") {
		t.Error("script summary should mention 'script resource'")
	}
	if !strings.Contains(summary, "/path/to/deploy.sh") {
		t.Error("script summary should contain absolute path")
	}
	if !strings.Contains(summary, "2048 bytes") {
		t.Error("script summary should contain size in bytes")
	}
}

func TestFormatResourceLoadSummary_ScriptMaterialized(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:               "deploy",
		FilePath:                "scripts/deploy.sh",
		ContentSize:             1024,
		ResourceType:            "script",
		AbsolutePath:            "/tmp/artifacts/deploy.sh",
		MaterializedToArtifacts: true,
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "materialized") {
		t.Error("materialized script summary should mention 'materialized'")
	}
}

func TestFormatResourceLoadSummary_ScriptFuzzy(t *testing.T) {
	result := &SkillResourceLoadResult{
		SkillName:    "deploy",
		FilePath:     "deploy.sh",
		ContentSize:  512,
		ResourceType: "script",
		AbsolutePath: "/path/to/scripts/deploy.sh",
		FuzzyMatched: true,
		MatchedPath:  "scripts/deploy.sh",
	}
	summary := FormatResourceLoadSummary(result)
	if !strings.Contains(summary, "fuzzy matched") {
		t.Error("fuzzy script summary should mention fuzzy match")
	}
}

// --- Context limit tests with expanded limits ---

func TestSkillsContextManager_ExpandedLimits(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	var largeBody strings.Builder
	for i := 0; i < 300; i++ {
		largeBody.WriteString("Line of content for expanded limit testing.\n")
	}

	vfs.AddFile("large-skill/SKILL.md", buildTestSkillMD("large-skill", "Large skill", largeBody.String()))
	vfs.AddFile("large-skill/extra.md", "# Extra\nAdditional content.")

	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}

	mgr := NewSkillsContextManager(loader)
	if err := mgr.LoadSkill("large-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}

	rendered := mgr.Render("n")
	if !strings.Contains(rendered, "large-skill") {
		t.Error("rendered should contain large-skill")
	}

	if len(rendered) > SkillsContextMaxBytes+1024 {
		t.Errorf("rendered size %d should be within expanded limit %d", len(rendered), SkillsContextMaxBytes)
	}
}

// --- GrepSkillResources tests ---

func buildGrepTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("alpha/SKILL.md", buildTestSkillMD("alpha", "Alpha skill", "# Alpha\nAlpha body content"))
	vfs.AddFile("alpha/guide.md", "# Guide\nLine 1 of guide\nLine 2 with keyword SEARCHME here\nLine 3 of guide\nLine 4 of guide\nLine 5 of guide")
	vfs.AddFile("alpha/scripts/deploy.sh", "#!/bin/bash\necho SEARCHME deploy\necho done")
	vfs.AddFile("alpha/data.json", `{"key": "value", "search": "SEARCHME"}`)

	vfs.AddFile("beta/SKILL.md", buildTestSkillMD("beta", "Beta skill", "# Beta\nBeta body"))
	vfs.AddFile("beta/reference.md", "# Reference\nNo matching content here\nJust some text\n")
	vfs.AddFile("beta/notes.md", "# Notes\nAnother SEARCHME occurrence\nEnd of notes")
	return vfs
}

func TestGrepSkillResources_SingleSkill(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("SEARCHME", "alpha")
	if err != nil {
		t.Fatalf("GrepSkillResources failed: %v", err)
	}

	if result.TotalMatches < 2 {
		t.Errorf("expected at least 2 matches in alpha, got %d", result.TotalMatches)
	}
	if len(result.SearchedSkills) != 1 || result.SearchedSkills[0] != "alpha" {
		t.Errorf("expected searched skill [alpha], got %v", result.SearchedSkills)
	}
	if result.SkillName != "alpha" {
		t.Errorf("SkillName = %q, want 'alpha'", result.SkillName)
	}

	for _, m := range result.Matches {
		if m.SkillName != "alpha" {
			t.Errorf("match from wrong skill: %q", m.SkillName)
		}
		if m.LineNo <= 0 {
			t.Errorf("match line number should be positive, got %d", m.LineNo)
		}
		if m.Context == "" {
			t.Error("match context should not be empty")
		}
	}
}

func TestGrepSkillResources_AllSkills(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("SEARCHME", "")
	if err != nil {
		t.Fatalf("GrepSkillResources all skills failed: %v", err)
	}

	if len(result.SearchedSkills) < 2 {
		t.Errorf("expected at least 2 searched skills, got %d: %v", len(result.SearchedSkills), result.SearchedSkills)
	}
	if result.SkillName != "" {
		t.Errorf("SkillName should be empty for all-skill search, got %q", result.SkillName)
	}

	hasAlpha, hasBeta := false, false
	for _, m := range result.Matches {
		if m.SkillName == "alpha" {
			hasAlpha = true
		}
		if m.SkillName == "beta" {
			hasBeta = true
		}
	}
	if !hasAlpha {
		t.Error("should find matches in alpha skill")
	}
	if !hasBeta {
		t.Error("should find matches in beta skill")
	}
}

func TestGrepSkillResources_NoMatch(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("ZZZZZ_NONEXISTENT_PATTERN", "")
	if err != nil {
		t.Fatalf("GrepSkillResources should not error on no match: %v", err)
	}
	if result.TotalMatches != 0 {
		t.Errorf("expected 0 matches, got %d", result.TotalMatches)
	}
	if len(result.Matches) != 0 {
		t.Errorf("expected empty matches slice, got %d", len(result.Matches))
	}
}

func TestGrepSkillResources_RegexPattern(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources(`SEARCH\w+`, "alpha")
	if err != nil {
		t.Fatalf("GrepSkillResources regex failed: %v", err)
	}
	if result.TotalMatches < 2 {
		t.Errorf("expected at least 2 regex matches, got %d", result.TotalMatches)
	}
}

func TestGrepSkillResources_ContextLines(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("SEARCHME", "alpha")
	if err != nil {
		t.Fatalf("GrepSkillResources failed: %v", err)
	}

	for _, m := range result.Matches {
		if m.Context == "" {
			t.Errorf("context should not be empty for match in %s:%d", m.FilePath, m.LineNo)
		}
	}
}

func TestGrepSkillResources_Truncation(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("trunc-skill/SKILL.md", buildTestSkillMD("trunc-skill", "Truncation test", "body"))

	var largeBuf strings.Builder
	for i := 0; i < 200; i++ {
		largeBuf.WriteString(fmt.Sprintf("Line %d: FINDME here\n", i+1))
	}
	vfs.AddFile("trunc-skill/large.txt", largeBuf.String())

	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("FINDME", "trunc-skill")
	if err != nil {
		t.Fatalf("GrepSkillResources truncation failed: %v", err)
	}

	if !result.IsTruncated {
		t.Error("should be truncated when matches exceed limit")
	}
	if result.TotalMatches > grepMaxMatches {
		t.Errorf("should not collect more than %d matches, got %d", grepMaxMatches, result.TotalMatches)
	}
}

func TestGrepSkillResources_SkipBinary(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("bin-skill/SKILL.md", buildTestSkillMD("bin-skill", "Binary test", "body"))
	vfs.AddFile("bin-skill/text.md", "# Text\nSEARCHME in text")
	vfs.AddFile("bin-skill/binary.dat", string([]byte{0x00, 0x01, 0x02, 'S', 'E', 'A', 'R', 'C', 'H', 'M', 'E'}))

	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("SEARCHME", "bin-skill")
	if err != nil {
		t.Fatalf("GrepSkillResources binary skip failed: %v", err)
	}

	for _, m := range result.Matches {
		if m.FilePath == "binary.dat" {
			t.Error("should skip binary files")
		}
	}
	if result.TotalMatches < 1 {
		t.Error("should still find matches in text files")
	}
}

func TestGrepSkillResources_SkillNotFound(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	result, err := mgr.GrepSkillResources("pattern", "nonexistent")
	if err != nil {
		t.Fatalf("should not error, just return empty: %v", err)
	}
	if result.TotalMatches != 0 {
		t.Errorf("expected 0 matches for nonexistent skill, got %d", result.TotalMatches)
	}
}

func TestGrepSkillResources_EmptyPattern(t *testing.T) {
	vfs := buildGrepTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	_, err := mgr.GrepSkillResources("", "alpha")
	if err == nil {
		t.Error("should error on empty pattern")
	}
}

func TestFormatGrepSummary(t *testing.T) {
	result := &SkillGrepResult{
		Pattern:        "test",
		TotalMatches:   5,
		SearchedSkills: []string{"a", "b"},
		SearchedFiles:  10,
	}
	summary := FormatGrepSummary(result)
	if !strings.Contains(summary, "5 matches") {
		t.Error("summary should contain match count")
	}
	if !strings.Contains(summary, "all skills") {
		t.Error("summary should say 'all skills' when SkillName is empty")
	}

	result.SkillName = "specific"
	summary = FormatGrepSummary(result)
	if !strings.Contains(summary, "skill 'specific'") {
		t.Error("summary should mention specific skill name")
	}

	result.IsTruncated = true
	summary = FormatGrepSummary(result)
	if !strings.Contains(summary, "truncated") {
		t.Error("summary should mention truncation")
	}
}

func TestFormatGrepResultForView(t *testing.T) {
	result := &SkillGrepResult{
		Pattern:        "test",
		TotalMatches:   2,
		SearchedSkills: []string{"s1"},
		SearchedFiles:  3,
		Matches: []SkillGrepMatch{
			{SkillName: "s1", FilePath: "file.md", LineNo: 5, LineText: "test line", Context: "  4| before\n  5| test line\n  6| after\n"},
			{SkillName: "s1", FilePath: "other.md", LineNo: 10, LineText: "another test", Context: "  9| ctx\n 10| another test\n 11| ctx\n"},
		},
	}
	view := FormatGrepResultForView(result)
	if !strings.Contains(view, "Grep Results") {
		t.Error("view should contain header")
	}
	if !strings.Contains(view, "s1/file.md") {
		t.Error("view should contain file path")
	}
	if !strings.Contains(view, "s1/other.md") {
		t.Error("view should contain second file path")
	}
	if !strings.Contains(view, "2 matches") {
		t.Error("view should show match count")
	}
}

func TestViewWindow_ExpandedLimit(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, strings.Repeat("x", 80))
	}
	content := strings.Join(lines, "\n")
	vw := NewViewWindow("s", "f", content, "n")

	rendered, truncated := vw.Render()
	if !truncated {
		if len(content) > ViewWindowMaxBytes {
			t.Error("content exceeding limit should be truncated")
		}
	}
	if len(rendered) > ViewWindowMaxBytes+200 {
		t.Errorf("rendered size %d should be within expanded limit %d", len(rendered), ViewWindowMaxBytes)
	}
}
