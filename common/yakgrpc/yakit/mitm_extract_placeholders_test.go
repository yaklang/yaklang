package yakit

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestExpandMITMExtractPlaceholders(t *testing.T) {
	h := MITMExtractPlaceholderHost
	u := MITMExtractPlaceholderURI
	full := MITMExtractPlaceholderURL

	pStd := MITMExtractPlaceholders{
		Host:    "h.example",
		FullURL: "https://h.example/p?q=1",
		URI:     "/p?q=1",
	}
	pWithPort := MITMExtractPlaceholders{
		Host:    "192.168.0.2:8888",
		FullURL: "http://192.168.0.2:8888/api/foo",
		URI:     "/api/foo",
	}
	pFullMeta := MITMExtractPlaceholders{
		Host:        "api.example:8443",
		FullURL:     "https://api.example:8443/v1/a?k=1",
		URI:         "/v1/a?k=1",
		Method:      "PUT",
		Path:        "/v1/a",
		Scheme:      "https",
		Port:        "8443",
		RuntimeID:   "rt-abc",
		HiddenIndex: "hid-xyz",
		RemoteAddr:  "10.0.0.1:1234",
		IP:          "10.0.0.2",
		SourceType:  "mitm",
		Hash:        "hash1",
		StatusCode:  "201",
	}

	tests := []struct {
		name string
		in   string
		p    MITMExtractPlaceholders
		want string
	}{
		{
			name: "all_three_ordered",
			in:   h + "::" + u + "::" + full,
			p:    pStd,
			want: "h.example::/p?q=1::https://h.example/p?q=1",
		},
		{
			name: "host_slash_uri_like_user_template",
			in:   h + "/" + u,
			p:    pWithPort,
			want: "192.168.0.2:8888//api/foo",
		},
		{
			name: "host_slash_literal_suffix",
			in:   h + "/suffix",
			p:    pWithPort,
			want: "192.168.0.2:8888/suffix",
		},
		{
			name: "repeat_host",
			in:   h + "|" + h,
			p:    pStd,
			want: "h.example|h.example",
		},
		{
			name: "empty_input",
			in:   "",
			p:    pStd,
			want: "",
		},
		{
			name: "no_placeholders_passthrough",
			in:   "plain-token/$1",
			p:    pStd,
			want: "plain-token/$1",
		},
		{
			name: "empty_placeholder_fields",
			in:   h + u + full,
			p:    MITMExtractPlaceholders{},
			want: "",
		},
		{
			name: "lookalike_without_full_token",
			in:   "host__uri",
			p:    pStd,
			want: "host__uri",
		},
		{
			name: "suffix_after_host_token",
			in:   h + "2",
			p:    pStd,
			want: "h.example2",
		},
		{
			name: "url_replaced_before_host_so_url_substring_safe",
			in:   full + " then " + h,
			p:    pStd,
			want: "https://h.example/p?q=1 then h.example",
		},
		{
			name: "all_builtin_placeholders",
			in: strings.Join([]string{
				MITMExtractPlaceholderMethod,
				MITMExtractPlaceholderPath,
				MITMExtractPlaceholderScheme,
				MITMExtractPlaceholderSchema,
				MITMExtractPlaceholderPort,
				MITMExtractPlaceholderRuntimeID,
				MITMExtractPlaceholderHiddenIndex,
				MITMExtractPlaceholderTraceID,
				MITMExtractPlaceholderRemoteAddr,
				MITMExtractPlaceholderIP,
				MITMExtractPlaceholderSourceType,
				MITMExtractPlaceholderFlowHash,
				MITMExtractPlaceholderStatusCode,
			}, "|"),
			p: pFullMeta,
			want: strings.Join([]string{
				"PUT", "/v1/a", "https", "https", "8443", "rt-abc", "hid-xyz", "hid-xyz",
				"10.0.0.1:1234", "10.0.0.2", "mitm", "hash1", "201",
			}, "|"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandMITMExtractPlaceholders(tt.in, tt.p)
			require.Equal(t, tt.want, got, "ExpandMITMExtractPlaceholders(%q)", tt.in)
		})
	}
}

func TestBuildMITMExtractPlaceholders_fromFlowOnly(t *testing.T) {
	f := &schema.HTTPFlow{
		Url:         "https://api.test/foo?x=1",
		Host:        "api.test",
		Method:      "GET",
		HiddenIndex: "idx-1",
		RuntimeId:   "run-1",
		RemoteAddr:  "1.2.3.4:5",
		IPAddress:   "1.2.3.4",
		SourceType:  schema.HTTPFlow_SourceType_MITM,
		Hash:        "hfhash",
		StatusCode:  200,
	}
	p := BuildMITMExtractPlaceholders(nil, f)
	require.Equal(t, f.Url, p.FullURL)
	require.Equal(t, "api.test", p.Host)
	require.Equal(t, "/foo?x=1", p.URI)
	require.Equal(t, "/foo", p.Path)
	require.Equal(t, "https", p.Scheme)
	require.Equal(t, "", p.Port)
	require.Equal(t, "GET", p.Method)
	require.Equal(t, "idx-1", p.HiddenIndex)
	require.Equal(t, "run-1", p.RuntimeID)
	require.Equal(t, "1.2.3.4:5", p.RemoteAddr)
	require.Equal(t, "1.2.3.4", p.IP)
	require.Equal(t, schema.HTTPFlow_SourceType_MITM, p.SourceType)
	require.Equal(t, "hfhash", p.Hash)
	require.Equal(t, "200", p.StatusCode)
}

func TestBuildMITMExtractPlaceholders_portFromURL(t *testing.T) {
	f := &schema.HTTPFlow{
		Url:  "http://127.0.0.1:9999/x",
		Host: "127.0.0.1:9999",
	}
	p := BuildMITMExtractPlaceholders(nil, f)
	require.Equal(t, "9999", p.Port)
	require.Equal(t, "http", p.Scheme)
}

func TestBuildMITMExtractPlaceholders_httpsFromIsHTTPS(t *testing.T) {
	f := &schema.HTTPFlow{
		Host:    "only.host",
		IsHTTPS: true,
		Path:    "/p",
	}
	p := BuildMITMExtractPlaceholders(nil, f)
	require.Equal(t, "https", p.Scheme)
	require.Equal(t, "/p", p.URI)
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
	m := &MatchResult{MatchResult: "x" + MITMExtractPlaceholderHost + "y"}
	p := MITMExtractPlaceholders{Host: "h"}
	out := CloneMatchResultWithMITMPlaceholders(m, p)
	if out == m {
		t.Fatal("expected distinct pointer")
	}
	if m.MatchResult != "x"+MITMExtractPlaceholderHost+"y" {
		t.Fatal("original mutated")
	}
	if out.MatchResult != "xhy" {
		t.Fatalf("got %q", out.MatchResult)
	}
}
