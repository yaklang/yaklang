package bruteutils

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minimartian"
	mitmconfig "github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	httpProxyOldExamplePage = "<html><title>Example Domain</title><body>This domain is for use in illustrative examples in documents.</body></html>"
	httpProxyNewExamplePage = "<html><title>Example Domain</title><body>This domain is for use in documentation examples without needing permission.</body></html>"
)

func TestHTTPProxyDetection(t *testing.T) {
	tests := []struct {
		name          string
		page          string
		status        int
		proxyUsername string
		proxyPassword string
		username      string
		password      string
		urlTarget     bool
		httpsProxy    bool
		targetUser    *url.Userinfo
		want          bool
	}{
		{name: "old_example_page", page: httpProxyOldExamplePage, want: true},
		{name: "new_example_page", page: httpProxyNewExamplePage, want: true},
		{name: "full_proxy_url", page: httpProxyNewExamplePage, urlTarget: true, want: true},
		{name: "https_proxy", page: httpProxyNewExamplePage, urlTarget: true, httpsProxy: true, want: true},
		{name: "correct_credentials", page: httpProxyNewExamplePage, proxyUsername: "admin", proxyPassword: "admin", username: "admin", password: "admin", want: true},
		{name: "incorrect_credentials", page: httpProxyNewExamplePage, proxyUsername: "admin", proxyPassword: "admin", username: "admin", password: "wrong"},
		{name: "missing_credentials", page: httpProxyNewExamplePage, proxyUsername: "admin", proxyPassword: "admin"},
		{name: "special_characters", page: httpProxyNewExamplePage, proxyUsername: "admin@/+#%", proxyPassword: "@/:+#%", username: "admin@/+#%", password: "@/:+#%", want: true},
		{name: "explicit_credentials_override_url", page: httpProxyNewExamplePage, proxyUsername: "admin", proxyPassword: "admin", username: "admin", password: "admin", urlTarget: true, targetUser: url.UserPassword("wrong", "wrong"), want: true},
		{name: "unauthorized_probe_clears_url_credentials", page: httpProxyNewExamplePage, urlTarget: true, targetUser: url.UserPassword("admin", "admin"), want: true},
		{name: "authenticated_url_does_not_bypass_unauthorized_probe", page: httpProxyNewExamplePage, proxyUsername: "admin", proxyPassword: "admin", urlTarget: true, targetUser: url.UserPassword("admin", "admin")},
		{name: "ordinary_http_page", page: "<html><title>Welcome</title><body>It works.</body></html>"},
		{name: "failed_response_with_matching_body", page: httpProxyNewExamplePage, status: http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This proxy deliberately supports HTTP forwarding only. CONNECT is
			// unnecessary for the plaintext challenge and may be denied by ACLs.
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodConnect {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.Method != http.MethodGet || r.RequestURI != "http://example.com/" || r.Host != "example.com" {
					t.Errorf("unexpected proxy request: %s %s (Host %q)", r.Method, r.RequestURI, r.Host)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				wantAuth := ""
				if tt.proxyUsername != "" || tt.proxyPassword != "" {
					wantAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(tt.proxyUsername+":"+tt.proxyPassword))
				}
				if r.Header.Get("Proxy-Authorization") != wantAuth {
					w.Header().Set("Proxy-Authenticate", `Basic realm="test proxy"`)
					w.WriteHeader(http.StatusProxyAuthRequired)
					return
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = io.WriteString(w, tt.page)
			})
			var proxy *httptest.Server
			if tt.httpsProxy {
				proxy = httptest.NewTLSServer(handler)
			} else {
				proxy = httptest.NewServer(handler)
			}
			defer proxy.Close()

			target := strings.TrimPrefix(proxy.URL, "http://")
			if tt.urlTarget {
				proxyURL, err := url.Parse(proxy.URL)
				require.NoError(t, err)
				proxyURL.User = tt.targetUser
				target = proxyURL.String()
			}
			require.Equal(t, tt.want, testHTTPProxy(target, tt.username, tt.password))
		})
	}
}

func TestHTTPProxyRejectsInvalidTargets(t *testing.T) {
	for _, target := range []string{
		"", "%", "http://", "http://127.0.0.1:0", "http://127.0.0.1:70000",
		"socks5://127.0.0.1:2080", "http://127.0.0.1:invalid",
		"http://127.0.0.1:2080/path", "http://127.0.0.1:2080?query=true", "http://127.0.0.1:2080#fragment",
	} {
		t.Run(target, func(t *testing.T) {
			require.False(t, testHTTPProxy(target, "admin", "admin"))
		})
	}
}

func TestHTTPProxyDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Host == "example.com" {
			http.Redirect(w, r, "http://redirect.example/", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, httpProxyNewExamplePage)
	}))
	defer proxy.Close()
	require.False(t, testHTTPProxy(strings.TrimPrefix(proxy.URL, "http://"), "", ""))
	require.Equal(t, int32(1), requests.Load(), "redirects must not turn a non-proxy response into a successful probe")
}

func TestHTTPProxyCanceledContext(t *testing.T) {
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, httpProxyNewExamplePage)
	}))
	defer proxy.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, testHTTPProxyWithContext(ctx, strings.TrimPrefix(proxy.URL, "http://"), "", ""))
	require.Zero(t, requests.Load())
}

func TestHTTPProxyMITMAuthentication(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		t.Run(fmt.Sprintf("authentication_%t", authenticated), func(t *testing.T) {
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, httpProxyNewExamplePage)
			}))
			defer origin.Close()
			proxy := minimartian.NewProxy()
			proxy.SetMITM(&mitmconfig.Config{})
			proxy.SetDisableSystemProxy(true)
			proxy.SetLowhttpConfig([]lowhttp.LowhttpOpt{
				lowhttp.WithETCHosts(map[string]string{"example.com": "127.0.0.1"}),
			})
			proxy.SetDialer(func(timeout time.Duration, target string) (net.Conn, error) {
				if target != "127.0.0.1:80" {
					return nil, fmt.Errorf("unexpected probe destination: %s", target)
				}
				return net.DialTimeout("tcp", strings.TrimPrefix(origin.URL, "http://"), timeout)
			})
			if authenticated {
				proxy.SetAuth("admin@/", "@/:+#%")
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- proxy.Serve(listener, ctx) }()
			t.Cleanup(func() {
				cancel()
				_ = listener.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("timed out stopping MITM proxy")
				}
			})
			target := listener.Addr().String()
			if authenticated {
				require.False(t, testHTTPProxy(target, "", ""))
				require.False(t, testHTTPProxy(target, "admin@/", "wrong"))
				require.True(t, testHTTPProxy(target, "admin@/", "@/:+#%"))
			} else {
				require.True(t, testHTTPProxy(target, "", ""))
			}
		})
	}
}
