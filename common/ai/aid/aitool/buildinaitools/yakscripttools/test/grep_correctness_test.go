package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/yak"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestGrepTool_ExecutionResultNotEmpty verifies that when grep matches are found,
// the tool's Execution Result (stdout via println) is NOT empty.
//
// Bug: output() only wrote to yakit.Info (IPC log) but never appended to findRes
// when contextBuffer==0. This meant the final println summary had 0 results in findRes,
// so the AI's result.Data was always empty even when matches existed.
func TestGrepTool_ExecutionResultNotEmpty(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	// Build a simple Go-like file with a known function
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		sb.WriteString(fmt.Sprintf("// padding line %d\n", i))
	}
	sb.WriteString("func main() {\n")
	sb.WriteString("\t// entry point\n")
	sb.WriteString("}\n")

	tmp, err := consts.TempFile("grep_execresult_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(sb.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":           tmp.Name(),
		"pattern":        "func main()",
		"pattern-mode":   "substr",
		"context-buffer": 0, // default - this triggered the bug
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	// The AI sees tool results through the stdout writer (which receives yakit.Info output as
	// "[info] ..." lines). The actual match content must appear in these [info] lines so the
	// AI can locate the entry point. Without the fix, grep produces only:
	// "[info] matches were found but no context text was collected" - which gives no location.
	//
	// Note: Yaklang println() writes directly to os.Stdout (not the stdout writer), so the
	// "[info]" prefixed lines (from yakit.Info) are what the AI tool framework captures.
	if !strings.Contains(stdoutStr, "func main()") {
		t.Fatalf("BUG: grep stdout does not contain the matched line 'func main()'.\n"+
			"The AI would only see 'matches were found but no context text was collected'\n"+
			"instead of the actual match location.\nstdout:\n%s", stdoutStr)
	}
	t.Log("✓ 'func main()' found in stdout (AI-visible content)")

	// After the fix: findRes has entries, so the summary lists actual matches.
	// Must have either the "=== Grep Results Summary:" line (from yakit.Info)
	// or the match lines with file:lineNo: format
	hasSummary := strings.Contains(stdoutStr, "=== Grep Results Summary:")
	hasMatchLine := strings.Contains(stdoutStr, "[match 1]")
	if !hasSummary && !hasMatchLine {
		t.Fatalf("BUG: stdout missing grep results summary or match lines.\n"+
			"Expected '[match 1]' or '=== Grep Results Summary:' in stdout.\nstdout:\n%s", stdoutStr)
	}
	t.Log("✓ Grep results summary/match lines present in stdout")
}

// TestGrepTool_CorrectLineNumber verifies that grep reports the correct line number
// for matched content, not a wrong/shifted line.
//
// Bug: countLineNumber used offset from workingContent (pattern search) but applied it
// to rawContent ([]byte). For large files, slight encoding differences or string
// representation issues could shift the offset, reporting the wrong line.
// Also, the match was reported at the wrong line in the actual run (1125 vs 1180).
func TestGrepTool_CorrectLineNumber(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	// Build a file with func main() at a known exact line number.
	// We want line 50 to have "func main() {" and verify grep reports line 50.
	const targetLine = 50
	var sb strings.Builder
	for i := 1; i < targetLine; i++ {
		sb.WriteString(fmt.Sprintf("// line %d: some content here to make the file larger\n", i))
	}
	sb.WriteString("func main() {\n") // line 50
	for i := targetLine + 1; i <= 200; i++ {
		sb.WriteString(fmt.Sprintf("// line %d: trailing content\n", i))
	}

	tmp, err := consts.TempFile("grep_lineno_correct_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(sb.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":           tmp.Name(),
		"pattern":        "func main()",
		"pattern-mode":   "substr",
		"context-buffer": 0,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	expectedMarker := fmt.Sprintf(":%d:", targetLine)
	if !strings.Contains(stdoutStr, expectedMarker) {
		t.Fatalf("BUG: grep did not report the correct line number %d.\n"+
			"Expected stdout to contain '%s' but got:\n%s",
			targetLine, tmp.Name()+expectedMarker, stdoutStr)
	}
	t.Logf("✓ grep correctly reports line number %d for 'func main()'", targetLine)
}

// TestGrepTool_RegexpCaretMatchesLineStart verifies that ^ in regexp mode matches
// the start of each line, not just the start of the entire file.
// Bug: Go's regexp defaults to single-line mode where ^ only matches the start of
// the entire string. When a user searches for "^func main\s*\(" across a file,
// the ^ only matches position 0 of the file, so if func main is NOT on line 1,
// the pattern silently matches nothing.
// Fix: automatically prepend (?m) to enable multi-line mode.
func TestGrepTool_RegexpCaretMatchesLineStart(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		sb.WriteString(fmt.Sprintf("// line %d\n", i))
	}
	sb.WriteString("func main() {\n") // line 21
	sb.WriteString("}\n")

	tmp, err := consts.TempFile("grep_caret_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(sb.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":         tmp.Name(),
		"pattern":      `^func main\s*\(`,
		"pattern-mode": "regexp",
		"limit":        10,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "matched 0 results") {
		t.Fatalf("BUG: regexp '^func main\\s*\\(' matched 0 results.\n"+
			"^ should match start of each line (multi-line mode), not just start of file.\n"+
			"stdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "func main") {
		t.Fatalf("expected grep to find 'func main' on line 21, but got:\n%s", stdoutStr)
	}
	t.Log("✓ regexp ^ correctly matches line start (multi-line mode)")
}

// TestGrepTool_RegexpDollarMatchesLineEnd verifies that $ in regexp mode matches
// the end of each line, not just the end of the entire file.
func TestGrepTool_RegexpDollarMatchesLineEnd(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	content := "line one\nline two target$\nline three\n"
	tmp, err := consts.TempFile("grep_dollar_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":         tmp.Name(),
		"pattern":      `target\$$`,
		"pattern-mode": "regexp",
		"limit":        10,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "matched 0 results") {
		t.Fatalf("BUG: regexp 'target\\$$' matched 0 results.\n"+
			"$ should match end of each line (multi-line mode).\nstdout:\n%s", stdoutStr)
	}
	t.Log("✓ regexp $ correctly matches line end (multi-line mode)")
}

// TestGrepTool_SameLineDedup verifies that when a pattern matches multiple times
// on the same line, grep reports the line only ONCE — matching bash grep behavior.
// Bash `grep 'foo'` on "foo bar foo baz foo\n" outputs 1 line, not 3.
func TestGrepTool_SameLineDedup(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	content := "aaa bbb aaa ccc aaa\nxxx\naaa end\n"
	tmp, err := consts.TempFile("grep_dedup_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":    tmp.Name(),
		"pattern": "aaa",
		"limit":   20,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	// "aaa" appears on line 1 (3 times) and line 3 (1 time).
	// Bash grep would output 2 lines. Current grep.yak may report 4 matches.
	// We expect exactly 2 match entries in findRes (one per line).
	if !strings.Contains(stdoutStr, "2 matches") {
		t.Fatalf("expected 2 matches (one per matching line, like bash grep), got:\n%s", stdoutStr)
	}
}

// TestGrepTool_RegexpSameLineDedup verifies same-line dedup works in regexp mode too.
// Pattern `\d+` on "a1b2c3\n" should report 1 line, not 3 matches.
func TestGrepTool_RegexpSameLineDedup(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	content := "a1b2c3\nno digits here\nx9y\n"
	tmp, err := consts.TempFile("grep_re_dedup_test_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":         tmp.Name(),
		"pattern":      `\d+`,
		"pattern-mode": "regexp",
		"limit":        20,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	// \d+ matches 3 times on line 1 ("1","2","3") and once on line 3 ("9").
	// Like bash grep -E '\d+', we should get 2 matching lines, not 4 individual matches.
	if !strings.Contains(stdoutStr, "2 matches") {
		t.Fatalf("expected 2 matches (one per matching line), got:\n%s", stdoutStr)
	}
}

// TestGrepTool_LargeFileCorrectLineNumber tests that line numbers are correct
// even in large files with many lines (simulating the yaklang/yak.go scenario
// where func main() is at line 1125 but grep reported line 1180).
func TestGrepTool_LargeFileCorrectLineNumber(t *testing.T) {
	grepTool := getGrepToolFromEmbed(t)

	// Simulate a large file like yak.go: func main() at line 1125,
	// with many different-length lines before it.
	const targetLine = 1125
	var sb strings.Builder
	for i := 1; i < targetLine; i++ {
		// Vary line lengths to exercise the byte-offset calculation more thoroughly
		sb.WriteString(fmt.Sprintf("// line %d: %s\n", i, strings.Repeat("x", i%80)))
	}
	sb.WriteString("func main() {\n") // line 1125
	for i := targetLine + 1; i <= 1200; i++ {
		sb.WriteString(fmt.Sprintf("// line %d: trailing\n", i))
	}

	tmp, err := consts.TempFile("grep_largefile_lineno_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.WriteString(sb.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err = grepTool.Callback(context.Background(), aitool.InvokeParams{
		"path":           tmp.Name(),
		"pattern":        "func main()",
		"pattern-mode":   "substr",
		"context-buffer": 0,
	}, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}

	stdoutStr := stdout.String()
	t.Logf("stdout:\n%s", stdoutStr)

	expectedMarker := fmt.Sprintf(":%d:", targetLine)
	if !strings.Contains(stdoutStr, expectedMarker) {
		t.Fatalf("BUG: grep reported wrong line number for 'func main()' in large file.\n"+
			"Expected to find ':%d:' in output but got:\n%s",
			targetLine, stdoutStr)
	}
	t.Logf("✓ grep correctly identifies line %d in a large (%d-line) file", targetLine, 1200)
}
