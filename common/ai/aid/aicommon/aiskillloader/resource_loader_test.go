package aiskillloader

import (
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
