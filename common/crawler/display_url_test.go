package crawler

import (
	"strings"
	"testing"
)

func TestRedactURLForDisplayPreservesHarmlessURLBytes(t *testing.T) {
	raw := "HTTPS://User:Pass@Example.COM/a%2Fb;v?keep=a%20b&Token=top-secret;API_KEY=second-secret&flag#client-secret"
	got := RedactURLForDisplay(raw)

	for _, leaked := range []string{"User", "Pass", "top-secret", "second-secret", "client-secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("display URL leaked %q: %q", leaked, got)
		}
	}
	for _, preserved := range []string{
		"https://Example.COM/a%2Fb;v?",
		"keep=a%20b",
		"Token=[REDACTED]",
		"API_KEY=[REDACTED]",
		"&flag",
	} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("display URL lost %q: %q", preserved, got)
		}
	}
}

func TestRedactURLForDisplayCredentialKeyVariants(t *testing.T) {
	raw := "/gateway?session=s1&signature=s2&credential=s3&password=s4&secret=s5&api-key=s6&apikey=s7&api_key=s8&access-key=s9&accesskey=s10&access_key=s11&oauth=s12&jwt=s13&auth=s14&code=s15&keep=visible"
	got := RedactURLForDisplay(raw)

	for _, leaked := range []string{
		"=s1", "=s2", "=s3", "=s4", "=s5", "=s6", "=s7", "=s8",
		"=s9", "=s10", "=s11", "=s12", "=s13", "=s14", "=s15",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("display URL leaked %q: %q", leaked, got)
		}
	}
	if !strings.Contains(got, "keep=visible") {
		t.Fatalf("display URL lost harmless query bytes: %q", got)
	}
	if count := strings.Count(got, "[REDACTED]"); count != 15 {
		t.Fatalf("expected 15 redactions, got %d in %q", count, got)
	}
}

func TestRedactURLForDisplayEncodedSensitiveKeyAndMalformedURL(t *testing.T) {
	got := RedactURLForDisplay("/api?%74%6f%6b%65%6e=encoded-secret&keep=x+y")
	if strings.Contains(got, "encoded-secret") || !strings.Contains(got, "%74%6f%6b%65%6e=[REDACTED]") || !strings.Contains(got, "keep=x+y") {
		t.Fatalf("encoded credential key was not safely redacted: %q", got)
	}

	malformed := RedactURLForDisplay("https://user:password@[::1?token=secret")
	if malformed != "[REDACTED: malformed URL]" {
		t.Fatalf("malformed URL should fail closed, got %q", malformed)
	}
}

func TestSanitizeTextForDisplayEscapesSectionBreakingControls(t *testing.T) {
	raw := "text\x1b\u0085\u2028\u2029\nnext"
	got := SanitizeTextForDisplay(raw)
	for _, forbidden := range []string{"\x1b", "\u0085", "\u2028", "\u2029", "\n"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("display text retained control %q: %q", forbidden, got)
		}
	}
	for _, escaped := range []string{`\x1b`, `\u0085`, `\u2028`, `\u2029`, `\x0a`} {
		if !strings.Contains(got, escaped) {
			t.Fatalf("display text lost escaped marker %q: %q", escaped, got)
		}
	}

	oversized := SanitizeTextForDisplay(strings.Repeat("x", maxCrawlerDisplayTextBytes+1))
	if !strings.Contains(oversized, "OMITTED") || strings.Contains(oversized, strings.Repeat("x", 128)) {
		t.Fatalf("oversized display text was not safely omitted: %q", oversized)
	}
}
