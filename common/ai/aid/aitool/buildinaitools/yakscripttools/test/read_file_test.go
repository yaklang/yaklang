package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func getReadFileTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/fs/read_file.yak")
	if err != nil {
		t.Fatalf("failed to read read_file.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("read_file", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse read_file.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execReadFileTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout string, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

// buildReadTestFile creates a temp file with N numbered lines.
func buildReadTestFile(t *testing.T, lines int) string {
	t.Helper()
	tmp, err := consts.TempFile("read_file_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	for i := 1; i <= lines; i++ {
		if _, err := fmt.Fprintf(tmp, "line %d: hello world content here\n", i); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
	}
	return tmp.Name()
}

// TestReadFile_NonExistentFile verifies that reading a nonexistent file produces
// a helpful error message that instructs the AI to use find_file or grep.
func TestReadFile_NonExistentFile(t *testing.T) {
	tool := getReadFileTool(t)
	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file": "/this/path/does/not/exist/at/all.go",
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "is not existed"),
		"should report file not found")
	// After our fix, the error message should guide the AI to use find_file/grep
	assert.Assert(t, strings.Contains(stdout, "find_file") || strings.Contains(stdout, "grep"),
		"error message should guide AI to use find_file or grep to locate the correct path")
}

// TestReadFile_LinesMode verifies the basic lines-mode output:
// lines are numbered and the correct range is returned.
func TestReadFile_LinesMode(t *testing.T) {
	tool := getReadFileTool(t)
	path := buildReadTestFile(t, 100)

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":  path,
		"mode":  "lines",
		"lines": 20,
	})
	t.Logf("stdout:\n%s", stdout)

	// Lines should be numbered (e.g. "  1 | line 1: ...")
	assert.Assert(t, strings.Contains(stdout, "1 |") || strings.Contains(stdout, "1|"),
		"lines mode must number output lines")
	assert.Assert(t, strings.Contains(stdout, "line 1:"),
		"first line content should appear in output")
	// Should not contain line 21 since we requested only 20
	assert.Assert(t, !strings.Contains(stdout, "line 21:"),
		"output should stop at line 20 when lines=20")
}

// TestReadFile_LinesPagination verifies that the offset parameter correctly skips lines.
func TestReadFile_LinesPagination(t *testing.T) {
	tool := getReadFileTool(t)
	path := buildReadTestFile(t, 100)

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":   path,
		"mode":   "lines",
		"offset": 50,
		"lines":  10,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "line 50:") || strings.Contains(stdout, "line 51:"),
		"pagination with offset=50 should start at line 50 or 51")
	assert.Assert(t, !strings.Contains(stdout, "line 1:"),
		"paginated read starting at line 50 must not include line 1")
}

// TestReadFile_AutoMode_AlwaysNumberedLines verifies that auto mode (the default)
// always outputs numbered lines even for large files — it must NOT switch to chunk mode.
// This is the fix for the code-audit scenario where AI was getting unchunked byte output.
func TestReadFile_AutoMode_AlwaysNumberedLines(t *testing.T) {
	tool := getReadFileTool(t)

	// Create a file larger than 20KB (the old chunk threshold)
	tmp, err := consts.TempFile("read_file_large_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	// Write enough content to exceed 20KB
	for i := 1; i <= 800; i++ {
		if _, err := fmt.Fprintf(tmp, "// line %d: func Example%d() { return somePackage.DoSomethingWithValue(%d) }\n", i, i, i); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
	}
	tmpName := tmp.Name()

	info, _ := os.Stat(tmpName)
	t.Logf("large file size: %d bytes (%.1f KB)", info.Size(), float64(info.Size())/1024)
	assert.Assert(t, info.Size() > 20000, "test file must be larger than 20KB to trigger old chunk behavior")

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file": tmpName,
		// no mode specified → "auto"
	})
	t.Logf("stdout (first 500 chars):\n%s", stdout[:min(500, len(stdout))])

	// In the fixed version, auto mode always uses lines mode — output must contain line numbers.
	// The old broken behavior was: output "CHUNK[Total: ...]: \"...\"" with Go-quoted string (no line numbers).
	assert.Assert(t, !strings.Contains(stdout, "CHUNK[Total:"),
		"auto mode must NOT produce CHUNK output (which has no line numbers)")
	// Must have numbered lines
	hasLineNumbers := strings.Contains(stdout, "1 |") || strings.Contains(stdout, "  1 |")
	assert.Assert(t, hasLineNumbers, "auto mode must always output numbered lines for code readability")
}

// TestReadFile_ChunkMode verifies explicit chunk mode works and does NOT include line numbers
// (which is expected behavior for chunk mode — the caller chose it explicitly).
func TestReadFile_ChunkMode(t *testing.T) {
	tool := getReadFileTool(t)
	path := buildReadTestFile(t, 50)

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":       path,
		"mode":       "chunk",
		"chunk-size": 500,
		"offset":     0,
	})
	t.Logf("stdout:\n%s", stdout)

	// chunk mode should report offset and byte count
	assert.Assert(t, strings.Contains(stdout, "CHUNK[") || strings.Contains(stdout, "offset:"),
		"chunk mode must report offset/byte info")
	// chunk mode output should contain the actual file content
	assert.Assert(t, strings.Contains(stdout, "line 1:"),
		"chunk mode should include file content")
	// chunk mode must NOT use the old Go-quoted %#v format
	assert.Assert(t, !strings.Contains(stdout, `\n`),
		"chunk output must not contain Go-escaped \\n sequences (was using %%#v format)")
}

// TestReadFile_LargeLineCount verifies that requesting more lines than defaultLineHardCap (400)
// is honored in explicit lines mode (the hard cap only applies to autoLineMode).
func TestReadFile_LargeLineCount(t *testing.T) {
	tool := getReadFileTool(t)
	path := buildReadTestFile(t, 500)

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":  path,
		"mode":  "lines",
		"lines": 300,
	})
	t.Logf("stdout (tail 200 chars):\n%s", stdout[max(0, len(stdout)-200):])

	// Should have read 300 lines
	assert.Assert(t, strings.Contains(stdout, "displayed-lines: 300"),
		"explicit lines=300 must display exactly 300 lines")
	assert.Assert(t, strings.Contains(stdout, "line 300:"),
		"last line of the requested range (line 300) should appear in output")
	assert.Assert(t, !strings.Contains(stdout, "line 301:"),
		"should not read beyond the requested 300 lines")
}

// TestReadFile_BinaryFileDetection verifies that binary files are rejected with a helpful message.
func TestReadFile_BinaryFileDetection(t *testing.T) {
	tool := getReadFileTool(t)

	// Create a fake binary file using null bytes
	tmp, err := consts.TempFile("read_file_binary_*.bin")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.Write([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpName := tmp.Name()

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file": tmpName,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "binary") || strings.Contains(stdout, "not a text file"),
		"binary file should be detected and rejected with a helpful message")
}

// TestReadFile_SourceFile_NumberedOutput verifies reading a real source file with lines mode
// returns properly numbered output (regression test against the log observations).
func TestReadFile_SourceFile_NumberedOutput(t *testing.T) {
	tool := getReadFileTool(t)

	// Use the read_file.yak itself as the test subject (it's a known text file)
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/fs/read_file.yak")
	if err != nil {
		t.Fatalf("failed to read read_file.yak from embed FS: %v", err)
	}

	// Write it to a temp file so read_file tool can read it
	tmp, err := consts.TempFile("read_file_source_*.yak")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.Write(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpName := tmp.Name()

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":  tmpName,
		"mode":  "lines",
		"lines": 30,
	})
	t.Logf("stdout:\n%s", stdout)

	// Should have numbered lines starting from 1
	assert.Assert(t, strings.Contains(stdout, "1 |") || strings.Contains(stdout, "  1 |"),
		"output must contain line 1 with number prefix")
	// Should contain actual yak source keywords
	assert.Assert(t, strings.Contains(stdout, "__DESC__") || strings.Contains(stdout, "VERBOSE"),
		"should read actual yak source content")
}

// TestReadFile_RelativePath verifies read_file handles absolute paths correctly.
func TestReadFile_RelativePath(t *testing.T) {
	tool := getReadFileTool(t)

	// Create a file in a temp dir and reference it with an absolute path
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fpath, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	stdout, _ := execReadFileTool(t, tool, aitool.InvokeParams{
		"file":  fpath,
		"mode":  "lines",
		"lines": 10,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "hello"),
		"should read file content correctly")
	assert.Assert(t, strings.Contains(stdout, "world"),
		"should read file content correctly")
}
