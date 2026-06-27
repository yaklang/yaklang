package loop_ssa_api_discovery

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolResultTextContent_PrefersStdout(t *testing.T) {
	stdout := "[info] redirect #1 [302]\nCookie: JSESSIONID=abc"
	result := &aitool.ToolResult{
		Data: &aitool.ToolExecutionResult{Stdout: stdout},
	}
	got := toolResultTextContent(result)
	if got != stdout {
		t.Fatalf("got %q want raw stdout", got)
	}
	if loginProbeSuccessful(got) != loginProbeSuccessful(stdout) {
		t.Fatal("probe outcome should match raw stdout")
	}
}

func TestToolResultTextContent_JSONWrappedRedirectFollow(t *testing.T) {
	if !loginProbeSuccessful(task78RedirectFollowStdout) {
		t.Fatal("fixture sanity")
	}
	result := &aitool.ToolResult{
		Data: &aitool.ToolExecutionResult{Stdout: task78RedirectFollowStdout},
	}
	content := toolResultTextContent(result)
	if !strings.Contains(content, "redirect #1") || !strings.Contains(content, "Cookie:") {
		t.Fatalf("expected redirect follow stdout in content, len=%d", len(content))
	}
	if !loginProbeSuccessful(content) {
		t.Fatal("toolResultTextContent should restore redirect-follow login success")
	}
	out := analyzeHTTPOutputForLogin(
		"POST",
		"http://192.168.1.4:8080/admin/login",
		"username=admin2&password=admin123",
		"",
		content,
	)
	if out == nil || !out.Success {
		t.Fatal("expected successful login probe outcome")
	}
	if !strings.Contains(out.HeadersJSON, "PUBLICCMS_ADMIN=") {
		t.Fatalf("headers=%s", out.HeadersJSON)
	}
}

func TestExtractCookieHeaderPairs_UnescapesLiteralNewlines(t *testing.T) {
	wrapped := `{"stdout":"[info] redirect #1\nCookie: A=1; B=2\n"}`
	pairs := extractCookieHeaderPairsFromHTTPOutput(wrapped)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 cookies, got %v", pairs)
	}
}
