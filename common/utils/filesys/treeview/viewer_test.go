package treeview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestTreeViewBasic(t *testing.T) {
	log.Info("Testing basic TreeView functionality")

	paths := []string{
		"src/main.go",
		"src/utils/helper.go",
		"src/utils/parser.go",
		"docs/README.md",
		"docs/api/endpoints.md",
		"tests/unit/test1.go",
		"tests/integration/test2.go",
	}

	tv := NewTreeView(paths)
	if tv == nil {
		t.Fatal("TreeView should not be nil")
	}

	result := tv.Print()
	log.Infof("Basic tree output:\n%s", result)

	if !strings.Contains(result, "src") {
		t.Error("Result should contain 'src' directory")
	}
}

func TestTreeViewDepthLimit(t *testing.T) {
	log.Info("Testing TreeView depth limit functionality")

	paths := []string{
		"level1/level2/level3/level4/level5/deep.txt",
		"level1/level2/file.txt",
		"level1/another.txt",
		"root.txt",
	}

	// 测试深度限制为 3
	tv := NewTreeViewWithLimits(paths, 3, 0)
	result := tv.Print()
	log.Infof("Depth limited (3) tree output:\n%s", result)

	// 检查是否有省略号表示深度限制
	if !strings.Contains(result, "...") {
		t.Error("Result should contain '...' to indicate depth limit")
	}

	// 检查 level5 不应该出现
	if strings.Contains(result, "level5") {
		t.Error("level5 should not appear with depth limit of 3")
	}
}

func TestTreeViewLineLimit(t *testing.T) {
	log.Info("Testing TreeView line limit functionality")

	// 创建很多文件路径
	var paths []string
	for i := 0; i < 20; i++ {
		paths = append(paths, fmt.Sprintf("file%d.txt", i))
	}

	// 测试行数限制为 5
	tv := NewTreeViewWithLimits(paths, 0, 5)
	result := tv.Print()
	log.Infof("Line limited (5) tree output:\n%s", result)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) > 6 { // 5 lines + possible "..." line
		t.Errorf("Result should have at most 6 lines, got %d", len(lines))
	}

	// 检查是否有省略号表示行数限制
	if !strings.Contains(result, "...") {
		t.Error("Result should contain '...' to indicate line limit")
	}
}

func TestTreeViewBothLimits(t *testing.T) {
	log.Info("Testing TreeView with both depth and line limits")

	paths := []string{
		"src/level1/level2/level3/deep1.txt",
		"src/level1/level2/level3/deep2.txt",
		"src/level1/level2/file1.txt",
		"src/level1/level2/file2.txt",
		"src/level1/file3.txt",
		"src/file4.txt",
		"docs/level1/level2/doc1.md",
		"docs/level1/level2/doc2.md",
		"docs/level1/doc3.md",
		"docs/doc4.md",
		"test1.txt",
		"test2.txt",
	}

	// 测试深度限制为 2，行数限制为 8
	tv := NewTreeViewWithLimits(paths, 2, 8)
	result := tv.Print()
	log.Infof("Both limits (depth=2, lines=8) tree output:\n%s", result)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) > 9 { // 8 lines + possible "..." line
		t.Errorf("Result should have at most 9 lines, got %d", len(lines))
	}

	// 检查是否有省略号
	if !strings.Contains(result, "...") {
		t.Error("Result should contain '...' to indicate limits")
	}
}

func TestTreeViewFromString(t *testing.T) {
	log.Info("Testing TreeViewFromString functionality")

	pathsStr := `src/main.go
src/utils/helper.go
docs/README.md
tests/test1.go`

	tv := NewTreeViewFromString(pathsStr)
	if tv == nil {
		t.Fatal("TreeView should not be nil")
	}

	result := tv.Print()
	log.Infof("String-based tree output:\n%s", result)

	if !strings.Contains(result, "src") {
		t.Error("Result should contain 'src' directory")
	}
}

func TestTreeViewFromStringWithLimits(t *testing.T) {
	log.Info("Testing TreeViewFromStringWithLimits functionality")

	pathsStr := `level1/level2/level3/level4/file1.txt
level1/level2/level3/level4/file2.txt
level1/level2/file3.txt
level1/file4.txt
file5.txt
file6.txt
file7.txt`

	tv := NewTreeViewFromStringWithLimits(pathsStr, 3, 6)
	result := tv.Print()
	log.Infof("String-based tree with limits output:\n%s", result)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) > 7 { // 6 lines + possible "..." line
		t.Errorf("Result should have at most 7 lines, got %d", len(lines))
	}
}

func TestTreeViewSearch(t *testing.T) {
	log.Info("Testing TreeView search functionality")

	paths := []string{
		"src/main.go",
		"src/utils/helper.go",
		"docs/main.md",
		"tests/main_test.go",
	}

	tv := NewTreeView(paths)
	results := tv.Search("main")

	log.Infof("Search results for 'main': %v", results)

	if len(results) == 0 {
		t.Error("Search should find paths containing 'main'")
	}

	for _, result := range results {
		if !strings.Contains(result, "main") {
			t.Errorf("Result '%s' should contain 'main'", result)
		}
	}
}

func TestTreeViewCount(t *testing.T) {
	log.Info("Testing TreeView count functionality")

	paths := []string{
		"src/main.go",         // file
		"src/utils/helper.go", // file (src, utils are dirs)
		"docs/README.md",      // file (docs is dir)
		"tests/test1.go",      // file (tests is dir)
	}

	tv := NewTreeView(paths)
	files, dirs := tv.Count()

	log.Infof("Count results - Files: %d, Dirs: %d", files, dirs)

	if files != 4 {
		t.Errorf("Expected 4 files, got %d", files)
	}

	if dirs != 4 { // src, utils, docs, tests
		t.Errorf("Expected 4 directories, got %d", dirs)
	}
}

func TestTreeViewEmptyPaths(t *testing.T) {
	log.Info("Testing TreeView with empty paths")

	tv := NewTreeView(nil)
	if tv == nil {
		t.Fatal("TreeView should not be nil even with nil paths")
	}

	result := tv.Print()
	log.Infof("Empty paths tree output:\n%s", result)

	// 应该只有根目录
	expected := ".\n"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestTreeViewZeroLimits(t *testing.T) {
	log.Info("Testing TreeView with zero limits (no limits)")

	paths := []string{
		"level1/level2/level3/level4/level5/deep.txt",
		"level1/level2/file.txt",
		"level1/another.txt",
	}

	tv := NewTreeViewWithLimits(paths, 0, 0) // 0 means no limits
	result := tv.Print()
	log.Infof("No limits tree output:\n%s", result)

	// 应该包含所有级别
	if !strings.Contains(result, "level5") {
		t.Error("Should contain level5 when no depth limit")
	}

	// 不应该有省略号
	if strings.Contains(result, "...") {
		t.Error("Should not contain '...' when no limits")
	}
}

func TestTreeViewCollapseSimple(t *testing.T) {
	log.Info("Testing TreeView single folder collapse functionality")

	paths := []string{
		"src/main/java/App.java",
		"src/main/resources/config.yml",
		"docs/README.md",
	}

	// 不合并
	tv1 := NewTreeViewWithOptions(paths, 0, 0, false)
	result1 := tv1.Print()
	log.Infof("Without collapse:\n%s", result1)

	// 合并单文件夹
	tv2 := NewTreeViewWithOptions(paths, 0, 0, true)
	result2 := tv2.Print()
	log.Infof("With collapse:\n%s", result2)

	// 验证合并效果：应该有合并的路径
	if !strings.Contains(result2, "src/main") {
		t.Error("Should contain collapsed path 'src/main'")
	}

	// 非合并版本不应该有合并路径
	if strings.Contains(result1, "src/main") {
		t.Error("Non-collapsed version should not contain 'src/main' as single path")
	}
}

func TestTreeViewCollapseDeep(t *testing.T) {
	log.Info("Testing TreeView deep single folder collapse")

	// 使用更深层的测试
	paths := []string{
		"a/b/c/d/file.txt",
	}

	// 先看看不合并的结构
	tv1 := NewTreeViewWithOptions(paths, 0, 0, false)
	result1 := tv1.Print()
	log.Infof("Without collapse:\n%s", result1)

	tv := NewTreeViewWithOptions(paths, 0, 0, true)
	result := tv.Print()
	log.Infof("With collapse:\n%s", result)

	// 应该合并中间的单一目录链 (a/b 合并，然后是d)
	if !strings.Contains(result, "a/b") {
		t.Error("Should contain collapsed path 'a/b'")
	}

	// d 不应该被合并，因为它直接包含文件
	if !strings.Contains(result, "d") {
		t.Error("Should contain directory 'd' that holds file")
	}
}

func TestTreeViewCollapseWithFiles(t *testing.T) {
	log.Info("Testing TreeView collapse with mixed files and folders")

	paths := []string{
		"src/file1.txt",          // src 下有文件，不应该合并
		"src/sub/deep/file2.txt", // sub/deep 应该合并
		"docs/guide/tutorial.md", // docs/guide 应该合并
		"config/app.yml",         // config 下直接有文件，不合并
	}

	tv := NewTreeViewWithOptions(paths, 0, 0, true)
	result := tv.Print()
	log.Infof("Mixed collapse result:\n%s", result)

	// src 不应该合并，因为它直接包含文件
	if strings.Contains(result, "src/sub") {
		t.Error("src should not be collapsed with sub because src contains file1.txt")
	}

	// sub 不应该被合并，因为它在src下，而src包含文件
	// deep 不应该被合并，因为它直接包含文件
	if strings.Contains(result, "sub/deep") {
		t.Error("Should not collapse 'sub/deep' because deep contains files")
	}

	// docs 不应该被合并，因为guide直接包含文件
	if strings.Contains(result, "docs/guide") {
		t.Error("Should not collapse 'docs/guide' because guide contains files")
	}

	// 但应该保留这些目录结构
	if !strings.Contains(result, "sub") && !strings.Contains(result, "guide") {
		t.Error("Should contain individual directories")
	}
}

func TestTreeViewCollapseWithLimits(t *testing.T) {
	log.Info("Testing TreeView collapse with depth and line limits")

	paths := []string{
		"project/src/main/java/com/example/App.java",
		"project/src/main/resources/application.yml",
		"project/docs/api/endpoints.md",
		"project/docs/setup/install.md",
		"project/tests/unit/AppTest.java",
		"project/tests/integration/IntegrationTest.java",
	}

	tv := NewTreeViewWithOptions(paths, 3, 8, true)
	result := tv.Print()
	log.Infof("Collapse with limits result:\n%s", result)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) > 9 { // 8 lines + possible "..." line
		t.Errorf("Result should respect line limit, got %d lines", len(lines))
	}

	// 应该有合并路径
	if !strings.Contains(result, "src/main") {
		t.Error("Should contain collapsed paths even with limits")
	}
}

func TestTreeViewCollapseStringConstructors(t *testing.T) {
	log.Info("Testing TreeView collapse with string constructors")

	pathsStr := `src/main/java/App.java
src/main/resources/config.yml
docs/guide/setup.md`

	tv1 := NewTreeViewFromStringWithOptions(pathsStr, 0, 0, false)
	result1 := tv1.Print()
	log.Infof("String constructor without collapse:\n%s", result1)

	tv2 := NewTreeViewFromStringWithOptions(pathsStr, 0, 0, true)
	result2 := tv2.Print()
	log.Infof("String constructor with collapse:\n%s", result2)

	// 验证合并效果
	if !strings.Contains(result2, "src/main") {
		t.Error("String constructor should support collapse")
	}

	// docs/guide 不应该被合并，因为guide直接包含文件
	if strings.Contains(result2, "docs/guide") {
		t.Error("Should not collapse docs/guide because guide contains files")
	}

	// 但应该保留guide目录
	if !strings.Contains(result2, "guide") {
		t.Error("Should contain guide directory")
	}
}
