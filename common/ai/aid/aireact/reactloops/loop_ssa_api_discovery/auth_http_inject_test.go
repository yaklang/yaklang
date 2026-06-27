package loop_ssa_api_discovery

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestApplyAuthCredentialToHTTPParams_StripsManualCookieAndInjectsHeadersText(t *testing.T) {
	cred := &store.AuthCredential{
		HeadersJSON: `{"Cookie":"SESSION=abc123","X-CSRF-Token":"tok"}`,
	}
	SyncCredentialHeaderFields(cred)

	params := aitool.InvokeParams{
		"headers": "Cookie: WRONG=manual\nAccept: application/json",
	}
	notes := applyAuthCredentialToHTTPParams(params, cred)
	if len(notes) == 0 {
		t.Fatal("expected injection notes")
	}
	h, _ := params["headers"].(string)
	if strings.Contains(h, "WRONG=manual") {
		t.Fatalf("manual cookie should be stripped, got %q", h)
	}
	if !strings.Contains(h, "SESSION=abc123") {
		t.Fatalf("expected injected session cookie, got %q", h)
	}
	if !strings.Contains(h, "Accept: application/json") {
		t.Fatalf("non-auth header should be kept, got %q", h)
	}
}

func TestStripManualAuthHeadersFromParams(t *testing.T) {
	params := aitool.InvokeParams{
		"headers": "Authorization: Bearer x\nUser-Agent: test",
	}
	_, stripped := stripManualAuthHeadersFromParams(params)
	if !stripped {
		t.Fatal("expected stripped authorization")
	}
	h, _ := params["headers"].(string)
	if strings.Contains(strings.ToLower(h), "authorization") {
		t.Fatalf("authorization should be removed, got %q", h)
	}
	if !strings.Contains(h, "User-Agent: test") {
		t.Fatalf("non-auth header kept, got %q", h)
	}
}

func TestResolveHTTPAuthCredentialID_PrefersExplicitID(t *testing.T) {
	if id := resolveHTTPAuthCredentialID(7, nil); id != 7 {
		t.Fatalf("explicit id expected 7, got %d", id)
	}
}
