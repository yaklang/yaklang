package minimartian

import "testing"

func TestProxy_SetDownstreamProxyConfig_BypassExactHost(t *testing.T) {
	p := NewProxy()
	p.SetDownstreamProxyConfig(
		[]string{"http://default.proxy:8080"},
		map[string][]string{
			"!example.com": nil, // bypass even with default proxy configured
			"foo.com":      []string{"http://route.proxy:8081"},
		},
	)

	if got := p.selectProxiesForHost("example.com"); got != nil {
		t.Fatalf("expected bypass host to return nil proxies, got: %#v", got)
	}

	if got := p.selectProxiesForHost("foo.com"); len(got) != 1 || got[0] != "http://route.proxy:8081" {
		t.Fatalf("expected foo.com to use route proxy, got: %#v", got)
	}

	if got := p.selectProxiesForHost("bar.com"); len(got) != 1 || got[0] != "http://default.proxy:8080" {
		t.Fatalf("expected bar.com to use default proxy, got: %#v", got)
	}
}

func TestProxy_SetDownstreamProxyConfig_BypassWildcard(t *testing.T) {
	p := NewProxy()
	p.SetDownstreamProxyConfig(
		[]string{"http://default.proxy:8080"},
		map[string][]string{
			"!*.example.com": nil,
		},
	)

	// Suffix matcher should match both the root domain and subdomains.
	if got := p.selectProxiesForHost("example.com"); got != nil {
		t.Fatalf("expected example.com to bypass, got: %#v", got)
	}
	if got := p.selectProxiesForHost("a.example.com"); got != nil {
		t.Fatalf("expected a.example.com to bypass, got: %#v", got)
	}

	// Non-matching host should use default proxy.
	if got := p.selectProxiesForHost("other.com"); len(got) != 1 || got[0] != "http://default.proxy:8080" {
		t.Fatalf("expected other.com to use default proxy, got: %#v", got)
	}
}

func TestProxy_SetDownstreamProxyConfig_BypassWinsOverRoute(t *testing.T) {
	p := NewProxy()
	p.SetDownstreamProxyConfig(
		[]string{"http://default.proxy:8080"},
		map[string][]string{
			"example.com":  []string{"http://route.proxy:8081"},
			"!example.com": []string{"http://ignored.proxy:9999"},
		},
	)

	if got := p.selectProxiesForHost("example.com"); got != nil {
		t.Fatalf("expected bypass to take precedence, got: %#v", got)
	}
}
