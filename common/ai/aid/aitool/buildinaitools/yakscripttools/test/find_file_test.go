package test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func getFindFileTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/fs/find_file.yak")
	if err != nil {
		t.Fatalf("failed to read find_file.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("find_file", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse find_file.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execFindFileTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) string {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String()
}

// buildFindTestDir creates:
//
//	<root>/
//	  go.mod
//	  Makefile
//	  README.md
//	  main.go
//	  sub/
//	    helper.go
//	    helper_test.go
//	  sub2/
//	    data.json
func buildFindTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", full, err)
		}
	}
	write("go.mod", "module example.com/test\n\ngo 1.22\n")
	write("Makefile", "build:\n\tgo build ./...\n")
	write("README.md", "# Test project\n")
	write("main.go", "package main\n")
	write("sub/helper.go", "package sub\n")
	write("sub/helper_test.go", "package sub\n")
	write("sub2/data.json", "{}\n")
	return root
}

// TestFindFile_GlobExactFilename verifies that glob mode with a plain filename like "go.mod"
// correctly matches the file, even though the full path contains directory separators.
// This was the root cause of the recon failure: gobwas/glob "go.mod" never matched
// "/absolute/path/to/go.mod" when applied to the full path. The fix changes glob to
// match against the basename only.
func TestFindFile_GlobExactFilename(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":          root,
		"pattern":      "go.mod",
		"pattern-type": "glob",
		"type":         "f",
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "go.mod"), "glob 'go.mod' must match the go.mod file")
	assert.Assert(t, strings.Contains(stdout, "matched: 1"), "should report exactly 1 match")
}

// TestFindFile_GlobWildcard verifies that "*.go" in glob mode matches .go files by basename.
func TestFindFile_GlobWildcard(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":          root,
		"pattern":      "*.go",
		"pattern-type": "glob",
		"type":         "f",
		"max":          20,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "main.go"), "*.go glob must match main.go")
	assert.Assert(t, strings.Contains(stdout, "helper.go"), "*.go glob must match helper.go")
	assert.Assert(t, strings.Contains(stdout, "helper_test.go"), "*.go glob must match helper_test.go")
	assert.Assert(t, !strings.Contains(stdout, "go.mod"), "*.go glob must NOT match go.mod")
	assert.Assert(t, !strings.Contains(stdout, "data.json"), "*.go glob must NOT match data.json")
}

// TestFindFile_GlobMultipleFilenamesCommaSeparatedFails documents the known wrong usage:
// passing comma-separated filenames as a single glob pattern must NOT accidentally match
// individual filenames (the whole comma string is one invalid pattern → 0 matches).
func TestFindFile_GlobMultipleFilenamesCommaSeparatedFails(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":          root,
		"pattern":      "go.mod,Makefile,README.md",
		"pattern-type": "glob",
		"type":         "f",
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "matched: 0"),
		"comma-separated multi-pattern in glob mode must match 0 files (it is one invalid glob, not three)")
}

// TestFindFile_RegexpMultipleFilenames verifies the correct way to find multiple filenames:
// use regexp with alternation.
func TestFindFile_RegexpMultipleFilenames(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":          root,
		"pattern":      `(go\.mod|Makefile|README\.md)`,
		"pattern-type": "regexp",
		"type":         "f",
		"max":          20,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "go.mod"), "regexp alternation must match go.mod")
	assert.Assert(t, strings.Contains(stdout, "Makefile"), "regexp alternation must match Makefile")
	assert.Assert(t, strings.Contains(stdout, "README.md"), "regexp alternation must match README.md")
	assert.Assert(t, strings.Contains(stdout, "matched: 3"), "should report exactly 3 matches")
}

// TestFindFile_SubstrMode verifies the default substr mode works correctly.
func TestFindFile_SubstrMode(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":     root,
		"pattern": "helper",
		"type":    "f",
		"max":     20,
	})
	t.Logf("stdout:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "helper.go"), "substr 'helper' must match helper.go")
	assert.Assert(t, strings.Contains(stdout, "helper_test.go"), "substr 'helper' must match helper_test.go")
	assert.Assert(t, !strings.Contains(stdout, "main.go"), "substr 'helper' must NOT match main.go")
}

// TestFindFile_NonExistentDir verifies that searching a non-existent directory
// produces a friendly error message instead of silently returning empty results or panicking.
func TestFindFile_NonExistentDir(t *testing.T) {
	tool := getFindFileTool(t)
	nonExistent := filepath.Join(t.TempDir(), "this_dir_does_not_exist")

	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":     nonExistent,
		"pattern": "anything",
	})
	t.Logf("stdout:\n%s", stdout)

	stdoutLower := strings.ToLower(stdout)
	hasErrorMsg := strings.Contains(stdoutLower, "not exist") ||
		strings.Contains(stdoutLower, "does not exist") ||
		strings.Contains(stdoutLower, "error") ||
		strings.Contains(stdoutLower, "no such file")
	assert.Assert(t, hasErrorMsg,
		"searching a non-existent directory should produce a clear error message, got: %s", stdout)
}

// TestFindFile_MinSizeShouldNotFilterDirectories verifies that min-size only
// applies to files, not directories. When type="" (match all) and min-size is
// set, directories should still appear in results regardless of their reported size.
//
// The test dir has two directories named "sub" and "sub2" which contain "sub" in
// their path. With min-size=99999, no regular file can pass, but directories
// matching the pattern should still be returned.
func TestFindFile_MinSizeShouldNotFilterDirectories(t *testing.T) {
	tool := getFindFileTool(t)
	root := buildFindTestDir(t)

	// First verify without min-size: pattern "sub" with type="d" should match sub and sub2
	baseStdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":     root,
		"pattern": "sub",
		"type":    "d",
		"max":     20,
	})
	t.Logf("baseline (no min-size, type=d):\n%s", baseStdout)
	assert.Assert(t, strings.Contains(baseStdout, "dir:"),
		"baseline: pattern 'sub' with type=d should match directories; got: %s", baseStdout)

	// Now with min-size=99999 and type="" (match all): directories should still appear
	stdout := execFindFileTool(t, tool, aitool.InvokeParams{
		"dir":      root,
		"pattern":  "sub",
		"min-size": 99999,
		"max":      20,
	})
	t.Logf("with min-size=99999:\n%s", stdout)

	assert.Assert(t, strings.Contains(stdout, "dir:"),
		"directories should not be filtered out by min-size; expected 'dir:' entries but got: %s", stdout)
}
