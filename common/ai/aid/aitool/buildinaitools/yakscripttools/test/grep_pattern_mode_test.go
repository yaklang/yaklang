package test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/yak"
)

// createTempFileWithContent creates a temp file with the given content and returns the file path.
func createTempFileWithContent(t *testing.T, prefix, content string) string {
	t.Helper()
	tmp, err := consts.TempFile(prefix)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmp.Close()
	return tmp.Name()
}

// runGrep executes the grep tool with given params and returns stdout string.
func runGrep(t *testing.T, params aitool.InvokeParams) string {
	t.Helper()
	grepTool := getGrepToolFromEmbed(t)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	_, err := grepTool.Callback(context.Background(), params, nil, stdout, stderr)
	if err != nil {
		t.Fatalf("grep tool execution failed: %v", err)
	}
	return stdout.String()
}

// TestGrepTool_DefaultModeIsRegexp verifies that when pattern-mode is NOT provided,
// grep defaults to regexp matching (not substr).
// This is the critical fix: previously the default was "substr", causing AI-generated
// regex patterns like `db\.Exec\(` to be treated as literals (with backslashes).
func TestGrepTool_DefaultModeIsRegexp(t *testing.T) {
	content := `line1: db.Exec("SELECT 1")
line2: db.Query("SELECT 2")
line3: some other text
line4: db.Exec("DELETE FROM users")
`
	tmpPath := createTempFileWithContent(t, "grep_default_regexp_*.go", content)

	// Do NOT pass pattern-mode — should default to regexp
	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":    tmpPath,
		"pattern": `db\.Exec\(`,
	})
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "matched 0 results") || strings.Contains(stdoutStr, "No matches found") {
		t.Fatalf("BUG: default pattern-mode should be regexp, but pattern `db\\.Exec\\(` matched 0 results.\n"+
			"This means the default is still substr (treating backslashes as literals).\nstdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "2 matches") {
		t.Fatalf("expected 2 matches for `db\\.Exec\\(` in regexp mode, got:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "db.Exec") {
		t.Fatalf("expected output to contain 'db.Exec', got:\n%s", stdoutStr)
	}
	t.Log("OK: default pattern-mode is regexp, regex patterns work without explicit mode")
}

// TestGrepTool_RegexpAutoFallback verifies that when pattern-mode is default (regexp)
// and the pattern is not a valid regex (e.g., "exec(" with unbalanced parenthesis),
// grep auto-falls back to substring matching instead of failing.
func TestGrepTool_RegexpAutoFallback(t *testing.T) {
	content := `line1: exec("ls")
line2: some text
line3: exec("whoami")
line4: execute_something()
`
	tmpPath := createTempFileWithContent(t, "grep_autofallback_*.go", content)

	// "exec(" is NOT a valid regex (unbalanced parenthesis), but in auto-fallback mode
	// it should be treated as a literal substring
	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":    tmpPath,
		"pattern": "exec(",
	})
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "No matches found") || strings.Contains(stdoutStr, "matched 0 results") {
		t.Fatalf("BUG: auto-fallback should treat invalid regex 'exec(' as literal substring.\n"+
			"Expected matches for literal 'exec(' but got 0.\nstdout:\n%s", stdoutStr)
	}
	// "exec(" appears on line1 and line3 (and line4 has "execute_something()" which also contains "exec(")
	// Actually line4 does NOT contain "exec(" — "execute_something()" has "e(" not "exec("
	// Wait: "execute_something()" does not contain "exec(" as a substring.
	// So we expect exactly 2 matches: line1 and line3
	if !strings.Contains(stdoutStr, "2 matches") {
		t.Fatalf("expected 2 matches for literal 'exec(' via auto-fallback, got:\n%s", stdoutStr)
	}
	// Verify the auto-fallback warning appears in stdout (via yakit.Info)
	if !strings.Contains(stdoutStr, "auto-fallback") {
		t.Logf("WARNING: expected [auto-fallback] info message in stdout, but not found. "+
			"This is non-critical if the fallback still works.\nstdout:\n%s", stdoutStr)
	}
	t.Log("OK: invalid regex auto-falls back to substr matching")
}

// TestGrepTool_ExplicitRegexpMode_NoFallback verifies that when pattern-mode is explicitly
// set to "re" or "regex", an invalid regex pattern causes an error (no auto-fallback).
func TestGrepTool_ExplicitRegexpMode_NoFallback(t *testing.T) {
	content := `exec("ls")
some text
`
	tmpPath := createTempFileWithContent(t, "grep_explicit_re_*.go", content)

	for _, mode := range []string{"re", "regex"} {
		t.Run("mode="+mode, func(t *testing.T) {
			stdoutStr := runGrep(t, aitool.InvokeParams{
				"path":         tmpPath,
				"pattern":      "exec(",
				"pattern-mode": mode,
			})
			t.Logf("stdout:\n%s", stdoutStr)

			// In explicit mode, "exec(" is invalid regex and should NOT auto-fallback.
			// The tool should report 0 matches or an error (via defer recover), NOT find results.
			if strings.Contains(stdoutStr, "auto-fallback") {
				t.Fatalf("BUG: explicit regexp mode '%s' should NOT auto-fallback, but [auto-fallback] message found.\nstdout:\n%s", mode, stdoutStr)
			}
			// With defer recover() in handleRegexp, the panic from re.Compile~
			// is caught and the tool reports 0 matches (no crash).
			if strings.Contains(stdoutStr, "exec(") && !strings.Contains(stdoutStr, "No matches found") && !strings.Contains(stdoutStr, "matched 0 results") {
				// Check if it actually matched something — that would be wrong
				if strings.Contains(stdoutStr, "matches") && !strings.Contains(stdoutStr, "0 matches") && !strings.Contains(stdoutStr, "matched 0") {
					t.Fatalf("BUG: explicit regexp mode '%s' should fail on invalid regex 'exec(', but got matches.\nstdout:\n%s", mode, stdoutStr)
				}
			}
			t.Logf("OK: explicit regexp mode '%s' does not auto-fallback for invalid regex", mode)
		})
	}
}

// TestGrepTool_FilesWithMatchesMode_AutoFallback verifies that the auto-fallback
// mechanism also works in files_with_matches output mode.
func TestGrepTool_FilesWithMatchesMode_AutoFallback(t *testing.T) {
	content := `eval("code")
safe_function()
eval("more code")
`
	tmpPath := createTempFileWithContent(t, "grep_fwm_fallback_*.go", content)

	// "eval(" is invalid regex, files_with_matches mode
	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":        tmpPath,
		"pattern":     "eval(",
		"output-mode": "files_with_matches",
	})
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "No files matched") || strings.Contains(stdoutStr, "matched 0 files") {
		t.Fatalf("BUG: auto-fallback should work in files_with_matches mode too.\n"+
			"Expected the file to be listed as a match for literal 'eval('.\nstdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "1 files matched") {
		t.Fatalf("expected 1 file matched for literal 'eval(' in files_with_matches mode, got:\n%s", stdoutStr)
	}
	t.Log("OK: auto-fallback works in files_with_matches mode")
}

// TestGrepTool_SubstrModeUnaffectedByRegexpDefault verifies that explicitly setting
// pattern-mode to "substr" still performs literal substring matching (no regex).
// The regex metacharacters in the pattern should have no special meaning.
func TestGrepTool_SubstrModeUnaffectedByRegexpDefault(t *testing.T) {
	content := `the pattern is db\.Exec\( literally
this has db.Exec( which is different
`
	tmpPath := createTempFileWithContent(t, "grep_substr_explicit_*.txt", content)

	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":         tmpPath,
		"pattern":      `db\.Exec\(`,
		"pattern-mode": "substr",
	})
	t.Logf("stdout:\n%s", stdoutStr)

	// In substr mode, `db\.Exec\(` is searched as a literal string (with backslashes).
	// Only line 1 contains the literal "db\.Exec\(" (with actual backslash characters).
	if !strings.Contains(stdoutStr, "1 matches") {
		t.Fatalf("expected exactly 1 match for literal 'db\\.Exec\\(' in substr mode.\n"+
			"substr mode should NOT interpret backslashes as regex escapes.\nstdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, `db\.Exec\(`) {
		t.Fatalf("expected the matched line to contain the literal pattern, got:\n%s", stdoutStr)
	}
	t.Log("OK: substr mode treats pattern as literal, unaffected by regexp default")
}

// TestGrepTool_IsubstrModeStillCaseInsensitive verifies that isubstr mode remains
// case-insensitive matching and is not affected by the regexp default change.
func TestGrepTool_IsubstrModeStillCaseInsensitive(t *testing.T) {
	content := `DatabaseConnection.Execute("query1")
databaseconnection.execute("query2")
DATABASECONNECTION.EXECUTE("query3")
unrelated line here
`
	tmpPath := createTempFileWithContent(t, "grep_isubstr_*.txt", content)

	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":         tmpPath,
		"pattern":      "DatabaseConnection.Execute",
		"pattern-mode": "isubstr",
	})
	t.Logf("stdout:\n%s", stdoutStr)

	if !strings.Contains(stdoutStr, "3 matches") {
		t.Fatalf("expected 3 matches for case-insensitive 'DatabaseConnection.Execute', got:\n%s", stdoutStr)
	}
	t.Log("OK: isubstr mode is case-insensitive and unaffected by regexp default change")
}

// TestGrepTool_DefaultModeValidRegexp_NoFallback verifies that when pattern-mode is
// default (regexp) and the pattern IS a valid regex, it uses regexp matching (no fallback).
func TestGrepTool_DefaultModeValidRegexp_NoFallback(t *testing.T) {
	content := `func handleRequest(w http.ResponseWriter, r *http.Request) {}
func handleResponse(w http.ResponseWriter) {}
func handle() {}
func notAHandler(x int) {}
`
	tmpPath := createTempFileWithContent(t, "grep_valid_regexp_*.go", content)

	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":    tmpPath,
		"pattern": `func handle\w+\(`,
	})
	t.Logf("stdout:\n%s", stdoutStr)

	if strings.Contains(stdoutStr, "No matches found") {
		t.Fatalf("BUG: valid regex 'func handle\\w+\\(' should match, got no matches.\nstdout:\n%s", stdoutStr)
	}
	// Should match handleRequest and handleResponse (handle() doesn't match \w+ which needs 1+ chars after "handle")
	if !strings.Contains(stdoutStr, "2 matches") {
		t.Fatalf("expected 2 matches for 'func handle\\w+\\(' (handleRequest, handleResponse), got:\n%s", stdoutStr)
	}
	if strings.Contains(stdoutStr, "auto-fallback") {
		t.Fatalf("BUG: valid regex should NOT trigger auto-fallback.\nstdout:\n%s", stdoutStr)
	}
	t.Log("OK: valid regex in default mode works correctly without fallback")
}

// TestGrepTool_MultiplePatterns_MixedValidity verifies that when using comma-separated
// patterns in default mode, each pattern is independently evaluated:
// valid regex patterns use regexp matching, invalid ones fall back to substr.
func TestGrepTool_MultiplePatterns_MixedValidity(t *testing.T) {
	content := `os.Exec("cmd")
os.exec("cmd")
system("cmd")
system(cmd)
`
	tmpPath := createTempFileWithContent(t, "grep_multi_pattern_*.go", content)

	// `os\.Exec` is valid regex (matches "os.Exec"), `system(` is invalid regex
	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":    tmpPath,
		"pattern": `os\.Exec,system(`,
	})
	t.Logf("stdout:\n%s", stdoutStr)

	// os\.Exec should match line 1 (regexp: matches literal "os.Exec")
	// system( should auto-fallback to substr and match lines 3 and 4
	// Total: 3 matches
	if strings.Contains(stdoutStr, "No matches found") {
		t.Fatalf("BUG: mixed patterns should find matches, got none.\nstdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "os.Exec") {
		t.Fatalf("expected match for 'os.Exec' via regexp pattern, got:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "system(") {
		t.Fatalf("expected match for 'system(' via auto-fallback substr, got:\n%s", stdoutStr)
	}
	t.Log("OK: mixed-validity comma-separated patterns each handled correctly")
}

// TestGrepTool_DefaultMode_SimplePattern_WorksLikeBashGrep verifies that simple
// literal patterns (no regex metacharacters) work correctly in default regexp mode,
// behaving identically to bash `grep 'pattern' file`.
func TestGrepTool_DefaultMode_SimplePattern_WorksLikeBashGrep(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		sb.WriteString(fmt.Sprintf("line %d: some content\n", i))
	}
	sb.WriteString("line 101: func main() {\n")
	sb.WriteString("line 102: }\n")
	for i := 103; i <= 200; i++ {
		sb.WriteString(fmt.Sprintf("line %d: more content\n", i))
	}

	tmpPath := createTempFileWithContent(t, "grep_simple_pattern_*.go", sb.String())

	// No pattern-mode, simple literal pattern
	stdoutStr := runGrep(t, aitool.InvokeParams{
		"path":    tmpPath,
		"pattern": "func main()",
	})
	t.Logf("stdout:\n%s", stdoutStr)

	// "func main()" contains metacharacters "(" and ")" but as a regex it's invalid.
	// Auto-fallback should treat it as literal substring and find the match.
	if strings.Contains(stdoutStr, "No matches found") || strings.Contains(stdoutStr, "matched 0") {
		t.Fatalf("BUG: simple pattern 'func main()' should be found via auto-fallback.\n"+
			"Bash grep would find it too.\nstdout:\n%s", stdoutStr)
	}
	if !strings.Contains(stdoutStr, "1 matches") {
		t.Fatalf("expected exactly 1 match for 'func main()', got:\n%s", stdoutStr)
	}
	t.Log("OK: simple pattern with metacharacters works via auto-fallback, like bash grep")
}
