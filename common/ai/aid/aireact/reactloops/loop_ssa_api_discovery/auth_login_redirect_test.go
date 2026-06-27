package loop_ssa_api_discovery

import (
	"strings"
	"testing"
)

func TestSuggestPostLoginVerifyPaths_AdminLoginIndexHTML(t *testing.T) {
	paths := SuggestPostLoginVerifyPaths("/admin/login", "index.html", "/admin")
	if len(paths) == 0 {
		t.Fatal("expected candidates")
	}
	if paths[0] != "/admin/index.html" {
		t.Fatalf("first candidate should be /admin/index.html, got %q", paths[0])
	}
	foundAdmin := false
	for _, p := range paths {
		if p == "/admin" {
			foundAdmin = true
		}
		if p == "/admin/login/index.html" {
			t.Fatalf("should not suggest wrong path %q", p)
		}
	}
	if !foundAdmin {
		t.Fatalf("expected /admin in candidates: %v", paths)
	}
}

func TestPostLoginVerifyURLHint_WrongVerifyURL(t *testing.T) {
	hint := postLoginVerifyURLHint("/admin/login", "http://host/admin/login/index.html", "/admin")
	if hint == "" {
		t.Fatal("expected hint for wrong verify url")
	}
	if !strings.Contains(hint, "/admin/index.html") {
		t.Fatalf("hint should mention /admin/index.html: %s", hint)
	}
}

func TestParentURLPath(t *testing.T) {
	if got := parentURLPath("/admin/login"); got != "/admin" {
		t.Fatalf("got %q", got)
	}
}
