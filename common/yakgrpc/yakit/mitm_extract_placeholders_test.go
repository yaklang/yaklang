package yakit

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func TestExpandMITMExtractPlaceholders(t *testing.T) {
	p := MITMExtractPlaceholders{Host: "h.example", FullURL: "https://h.example/p?q=1", URI: "/p?q=1"}
	s := ExpandMITMExtractPlaceholders("__host__::__uri__::__url__", p)
	want := "h.example::/p?q=1::https://h.example/p?q=1"
	if s != want {
		t.Fatalf("got %q want %q", s, want)
	}
}

func TestBuildMITMExtractPlaceholders_fromFlowOnly(t *testing.T) {
	f := &schema.HTTPFlow{
		Url:  "https://api.test/foo?x=1",
		Host: "api.test",
	}
	p := BuildMITMExtractPlaceholders(nil, f)
	if p.FullURL != f.Url {
		t.Fatalf("FullURL %q", p.FullURL)
	}
	if p.Host != "api.test" {
		t.Fatalf("Host %q", p.Host)
	}
	if p.URI != "/foo?x=1" {
		t.Fatalf("URI %q", p.URI)
	}
}

func TestBuildMITMExtractPlaceholders_reqOverrides(t *testing.T) {
	u, _ := url.Parse("https://ctx.example/a?b=2")
	req := &http.Request{URL: u, Host: "ctx.example"}
	f := &schema.HTTPFlow{Url: "https://ignored/", Host: "ignored"}
	p := BuildMITMExtractPlaceholders(req, f)
	if p.URI != "/a?b=2" {
		t.Fatalf("URI %q", p.URI)
	}
	if p.Host != "ctx.example" {
		t.Fatalf("Host %q", p.Host)
	}
}

func TestCloneMatchResultWithMITMPlaceholders(t *testing.T) {
	m := &MatchResult{MatchResult: "x__host__y"}
	p := MITMExtractPlaceholders{Host: "h"}
	out := CloneMatchResultWithMITMPlaceholders(m, p)
	if out == m {
		t.Fatal("expected distinct pointer")
	}
	if m.MatchResult != "x__host__y" {
		t.Fatal("original mutated")
	}
	if out.MatchResult != "xhy" {
		t.Fatalf("got %q", out.MatchResult)
	}
}