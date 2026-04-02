package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

// getTreeTool 从 embed FS 加载 tree.yak 工具（不走 DB）
func getTreeTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/fs/tree.yak")
	if err != nil {
		t.Fatalf("failed to read tree.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("tree", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse tree.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

// execTreeTool 执行 tree 工具并返回 stdout
func execTreeTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) string {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tree tool execution error (may be expected): %v", err)
	}
	return w1.String()
}

// extractEntryLines 从 tree 输出中提取条目行（文件名/目录名行）
// 过滤掉：[info]/[warn]/[error] 日志行、分隔线、统计行、警告说明行
// 返回每行缩进+名称，如 "  subdir/" 或 "    file.go"
func extractEntryLines(output string) []string {
	var result []string
	for _, line := range strings.Split(output, "\n") {
		// 跳过空行
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 跳过 yakit 日志行（[info] / [warn] / [error] 前缀，或 ANSI 颜色码包裹的日志）
		stripped := strings.TrimSpace(line)
		// 去掉 ANSI 颜色码后检查
		plainLine := stripANSI(stripped)
		if strings.HasPrefix(plainLine, "[info]") ||
			strings.HasPrefix(plainLine, "[warn]") ||
			strings.HasPrefix(plainLine, "[error]") ||
			strings.HasPrefix(plainLine, "[INFO]") ||
			strings.HasPrefix(plainLine, "[WARN]") ||
			strings.HasPrefix(plainLine, "[ERROR]") {
			continue
		}
		// 跳过分隔线（=== 或 ***）
		if strings.HasPrefix(plainLine, "===") || strings.HasPrefix(plainLine, "***") {
			continue
		}
		// 跳过 Directory: 开头的头部行
		if strings.HasPrefix(plainLine, "Directory:") {
			continue
		}
		// 跳过统计/提示行（包含特定关键词）
		lower := strings.ToLower(plainLine)
		if strings.Contains(lower, "directories") ||
			strings.Contains(lower, " files") ||
			strings.Contains(lower, "entries") ||
			strings.Contains(lower, "next page") ||
			strings.Contains(lower, "showing entries") ||
			strings.Contains(lower, "you are not") ||
			strings.Contains(lower, "use dirs-only") ||
			strings.Contains(lower, "call tree") ||
			strings.Contains(lower, "increase max-lines") ||
			strings.Contains(lower, "other options") ||
			strings.Contains(lower, "page starting") ||
			(strings.Contains(lower, "offset=") && strings.Contains(lower, "→")) {
			continue
		}
		result = append(result, line)
	}
	return result
}

// stripANSI 去除字符串中的 ANSI 颜色转义码
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// 跳过 ESC[ ... m 序列
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// isTruncated 检查输出是否包含截断警告
func isTruncated(output string) bool {
	return strings.Contains(output, "WARNING: OUTPUT IS TRUNCATED")
}

// extractNextPageOffset 从截断输出的末尾提取 NEXT PAGE 提示中的 offset 值
func extractNextPageOffset(output string) int {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "NEXT PAGE:") && strings.Contains(line, "offset=") {
			// 格式: *** NEXT PAGE: tree(path="...", offset=N, max-lines=M, dirs-only=...) ***
			parts := strings.Split(line, "offset=")
			if len(parts) < 2 {
				continue
			}
			rest := parts[1]
			// rest 形如 "1000, max-lines=..." 或 "1000, dirs-only=..."
			end := strings.IndexAny(rest, ", )")
			if end < 0 {
				continue
			}
			val := 0
			_, _ = fmt.Sscanf(rest[:end], "%d", &val)
			return val
		}
	}
	return -1
}

// buildTreeTestDir 创建一个确定性的测试目录结构：
//
//	root/
//	  a.go
//	  b.go
//	  c.go
//	  d.go
//	  e.go
//	  dira/
//	    f.go
//	    g.go
//	  dirb/
//	    h.go
//	    dirc/
//	      i.go
//	      j.go
//
// 总共：3 个目录（dira/ dirb/ dirc/）+ 10 个文件 = 13 条目（不含根）
// dirs-only 模式：3 条目（dira/ dirb/ dirc/）
func buildTreeTestDir(t *testing.T) string {
	t.Helper()
	tmp, err := consts.TempFile("tree-test-*")
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	_ = tmp.Close()
	root := tmp.Name() + "_dir"
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", full, err)
		}
	}

	write("a.go", "")
	write("b.go", "")
	write("c.go", "")
	write("d.go", "")
	write("e.go", "")
	write("dira/f.go", "")
	write("dira/g.go", "")
	write("dirb/h.go", "")
	write("dirb/dirc/i.go", "")
	write("dirb/dirc/j.go", "")

	return root
}

// TestTreeTool_BasicOutput 验证无参数时能输出所有条目（文件+目录）
func TestTreeTool_BasicOutput(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// max-lines=0 不限制，exclude="" 禁用默认过滤，确保输出全部
	out := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
	})

	t.Logf("output:\n%s", out)
	assert.Assert(t, !isTruncated(out), "should not be truncated with max-lines=0")

	lines := extractEntryLines(out)
	// 3 dirs + 10 files = 13 entries
	assert.Equal(t, 13, len(lines), "should have 13 entries total, got %d: %v", len(lines), lines)
}

// TestTreeTool_DirsOnly 验证 dirs-only 模式只输出目录
func TestTreeTool_DirsOnly(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	out := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
	})

	t.Logf("dirs-only output:\n%s", out)
	assert.Assert(t, !isTruncated(out), "should not be truncated")

	lines := extractEntryLines(out)
	// dira/ dirb/ dirb/dirc/ = 3 dirs
	assert.Equal(t, 3, len(lines), "dirs-only should have 3 entries, got %d: %v", len(lines), lines)
	for _, line := range lines {
		assert.Assert(t, strings.HasSuffix(strings.TrimSpace(line), "/"),
			"dirs-only entry should end with '/': %q", line)
	}
}

// TestTreeTool_Offset_Normal 验证普通模式下 offset 能正确跳过前 N 条
func TestTreeTool_Offset_Normal(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// 先获取全量结果（无 offset 无截断）
	fullOut := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
	})
	fullLines := extractEntryLines(fullOut)
	total := len(fullLines)
	assert.Equal(t, 13, total, "prerequisite: need 13 entries")

	// offset=5：应跳过前 5 条，返回剩余 8 条
	out5 := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
		"offset":    5,
	})
	t.Logf("offset=5 output:\n%s", out5)
	assert.Assert(t, !isTruncated(out5), "offset=5 should not be truncated with max-lines=0")

	lines5 := extractEntryLines(out5)
	assert.Equal(t, total-5, len(lines5),
		"offset=5 should return %d entries, got %d: %v", total-5, len(lines5), lines5)

	// offset=5 的结果应与全量的后 8 条一致
	for i, line := range lines5 {
		assert.Equal(t, fullLines[5+i], line,
			"offset=5 entry[%d] mismatch: want %q got %q", i, fullLines[5+i], line)
	}
}

// TestTreeTool_Offset_DirsOnly 验证 dirs-only 模式下 offset 能正确跳过前 N 条目录
func TestTreeTool_Offset_DirsOnly(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// dirs-only 全量（3 个目录）
	fullOut := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
	})
	fullLines := extractEntryLines(fullOut)
	assert.Equal(t, 3, len(fullLines), "dirs-only prerequisite: 3 dirs")

	// offset=1：跳过第 1 个目录，返回后 2 个
	out1 := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
		"offset":    1,
	})
	t.Logf("dirs-only offset=1 output:\n%s", out1)
	assert.Assert(t, !isTruncated(out1), "dirs-only offset=1 should not be truncated")

	lines1 := extractEntryLines(out1)
	assert.Equal(t, 2, len(lines1),
		"dirs-only offset=1 should return 2 dirs, got %d: %v", len(lines1), lines1)

	// 结果应与全量第 2、3 条一致
	for i, line := range lines1 {
		assert.Equal(t, fullLines[1+i], line,
			"dirs-only offset=1 entry[%d] mismatch: want %q got %q", i, fullLines[1+i], line)
	}

	// offset=2：跳过前 2 个，返回最后 1 个
	out2 := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
		"offset":    2,
	})
	lines2 := extractEntryLines(out2)
	assert.Equal(t, 1, len(lines2),
		"dirs-only offset=2 should return 1 dir, got %d: %v", len(lines2), lines2)
	assert.Equal(t, fullLines[2], lines2[0])

	// offset=3（等于总数）：应返回空
	out3 := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
		"offset":    3,
	})
	lines3 := extractEntryLines(out3)
	assert.Equal(t, 0, len(lines3),
		"dirs-only offset=3 should return 0 dirs, got %d: %v", len(lines3), lines3)
}

// TestTreeTool_Pagination_Completeness 验证分页后合并结果与全量结果一致
// 这是最关键的测试：确保 offset+max-lines 分页不丢不重
func TestTreeTool_Pagination_Completeness(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// 全量结果（无截断）
	fullOut := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
	})
	fullLines := extractEntryLines(fullOut)
	total := len(fullLines)
	assert.Equal(t, 13, total, "prerequisite: 13 entries")

	// 用 pageSize=4 分页，收集所有页
	pageSize := 4
	var allPaged []string
	offset := 0
	for {
		out := execTreeTool(t, tool, aitool.InvokeParams{
			"path":      root,
			"max-lines": pageSize,
			"exclude":   "",
			"offset":    offset,
		})
		page := extractEntryLines(out)
		allPaged = append(allPaged, page...)

		if !isTruncated(out) {
			break
		}
		// 用工具输出的 NEXT PAGE 提示中的 offset 推进
		nextOffset := extractNextPageOffset(out)
		assert.Assert(t, nextOffset > offset,
			"NEXT PAGE offset %d should be greater than current offset %d", nextOffset, offset)
		offset = nextOffset
	}

	t.Logf("full(%d): %v", len(fullLines), fullLines)
	t.Logf("paged(%d): %v", len(allPaged), allPaged)

	assert.Equal(t, len(fullLines), len(allPaged),
		"paginated total should match full total: want %d got %d", len(fullLines), len(allPaged))
	for i := range fullLines {
		assert.Equal(t, fullLines[i], allPaged[i],
			"entry[%d] mismatch: want %q got %q", i, fullLines[i], allPaged[i])
	}
}

// TestTreeTool_Pagination_DirsOnly_Completeness 同上，验证 dirs-only 模式分页完整性
func TestTreeTool_Pagination_DirsOnly_Completeness(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// dirs-only 全量
	fullOut := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"dirs-only": true,
		"max-lines": 0,
		"exclude":   "",
	})
	fullLines := extractEntryLines(fullOut)
	assert.Equal(t, 3, len(fullLines), "prerequisite: 3 dirs")

	// pageSize=1 强制每页只有 1 条，验证 3 次翻页
	pageSize := 1
	var allPaged []string
	offset := 0
	for {
		out := execTreeTool(t, tool, aitool.InvokeParams{
			"path":      root,
			"dirs-only": true,
			"max-lines": pageSize,
			"exclude":   "",
			"offset":    offset,
		})
		page := extractEntryLines(out)
		allPaged = append(allPaged, page...)

		if !isTruncated(out) {
			break
		}
		nextOffset := extractNextPageOffset(out)
		assert.Assert(t, nextOffset > offset,
			"NEXT PAGE offset %d should be greater than current offset %d", nextOffset, offset)
		offset = nextOffset
	}

	t.Logf("dirs-only full: %v", fullLines)
	t.Logf("dirs-only paged: %v", allPaged)

	assert.Equal(t, 3, len(allPaged),
		"paginated dirs-only total should be 3, got %d: %v", len(allPaged), allPaged)
	for i := range fullLines {
		assert.Equal(t, fullLines[i], allPaged[i],
			"dirs-only entry[%d] mismatch: want %q got %q", i, fullLines[i], allPaged[i])
	}
}

// TestTreeTool_TruncationWarning_NextPageHint 验证截断输出包含正确的 NEXT PAGE 提示
func TestTreeTool_TruncationWarning_NextPageHint(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	// max-lines=3，offset=0 → 只输出前 3 条，应截断
	out := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 3,
		"exclude":   "",
		"offset":    0,
	})
	t.Logf("truncated output:\n%s", out)

	assert.Assert(t, isTruncated(out), "output should be truncated with max-lines=3")

	// NEXT PAGE 提示中的 offset 应为 3（已输出 3 条）
	nextOffset := extractNextPageOffset(out)
	assert.Equal(t, 3, nextOffset,
		"NEXT PAGE offset should be 3 (0 + 3 shown), got %d", nextOffset)

	// 继续翻页：offset=3，max-lines=3
	out2 := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 3,
		"exclude":   "",
		"offset":    3,
	})
	t.Logf("page2 output:\n%s", out2)
	assert.Assert(t, isTruncated(out2), "page2 should also be truncated (13 total, 6 shown)")

	nextOffset2 := extractNextPageOffset(out2)
	assert.Equal(t, 6, nextOffset2,
		"page2 NEXT PAGE offset should be 6 (3 + 3 shown), got %d", nextOffset2)
}

// TestTreeTool_Offset_ExceedsTotal 验证 offset 超过总条目数时返回空（不报错）
func TestTreeTool_Offset_ExceedsTotal(t *testing.T) {
	tool := getTreeTool(t)
	root := buildTreeTestDir(t)

	out := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
		"offset":    9999,
	})
	t.Logf("offset=9999 output:\n%s", out)
	assert.Assert(t, !isTruncated(out), "should not be truncated when offset exceeds total")
	lines := extractEntryLines(out)
	assert.Equal(t, 0, len(lines),
		"offset beyond total should return 0 entries, got %d: %v", len(lines), lines)
}

// TestTreeTool_LargeDir_PaginationCoversAll 用更大的目录结构验证分页不丢失条目
// 创建 50 个文件，用 pageSize=7 分页，确保合并后与全量一致
func TestTreeTool_LargeDir_PaginationCoversAll(t *testing.T) {
	tool := getTreeTool(t)

	// 创建含 50 个文件的目录
	tmp, err := consts.TempFile("tree-large-*")
	if err != nil {
		t.Fatalf("TempFile: %v", err)
	}
	_ = tmp.Close()
	root := tmp.Name() + "_dir"
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// 10 个子目录，每目录 5 个文件 = 10 dirs + 50 files = 60 entries
	for i := 0; i < 10; i++ {
		subDir := filepath.Join(root, fmt.Sprintf("pkg%02d", i))
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", subDir, err)
		}
		for j := 0; j < 5; j++ {
			if err := os.WriteFile(filepath.Join(subDir, fmt.Sprintf("file%02d.go", j)), nil, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}
	}

	// 全量
	fullOut := execTreeTool(t, tool, aitool.InvokeParams{
		"path":      root,
		"max-lines": 0,
		"exclude":   "",
	})
	fullLines := extractEntryLines(fullOut)
	total := len(fullLines)
	assert.Equal(t, 60, total, "prerequisite: 60 entries (10 dirs + 50 files)")

	// 用 pageSize=7 分页
	pageSize := 7
	var allPaged []string
	offset := 0
	pages := 0
	for {
		out := execTreeTool(t, tool, aitool.InvokeParams{
			"path":      root,
			"max-lines": pageSize,
			"exclude":   "",
			"offset":    offset,
		})
		page := extractEntryLines(out)
		allPaged = append(allPaged, page...)
		pages++

		if !isTruncated(out) {
			break
		}
		nextOffset := extractNextPageOffset(out)
		assert.Assert(t, nextOffset > offset,
			"page %d: NEXT PAGE offset %d should be > current %d", pages, nextOffset, offset)
		offset = nextOffset

		if pages > 100 {
			t.Fatal("pagination loop not terminating after 100 pages")
		}
	}

	t.Logf("covered %d entries across %d pages", len(allPaged), pages)

	// 排序比较（filesys.Recursive 顺序在不同 OS 可能不同，用 sort 消除顺序差异）
	fullSorted := make([]string, len(fullLines))
	copy(fullSorted, fullLines)
	sort.Strings(fullSorted)

	pagedSorted := make([]string, len(allPaged))
	copy(pagedSorted, allPaged)
	sort.Strings(pagedSorted)

	assert.Equal(t, len(fullSorted), len(pagedSorted),
		"paginated total %d should equal full total %d", len(pagedSorted), len(fullSorted))
	for i := range fullSorted {
		assert.Equal(t, fullSorted[i], pagedSorted[i],
			"sorted entry[%d] mismatch: want %q got %q", i, fullSorted[i], pagedSorted[i])
	}
}
