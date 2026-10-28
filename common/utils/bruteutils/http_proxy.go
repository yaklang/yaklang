package bruteutils

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/netx"
)

// Match the page identity rather than its explanatory text, which has changed.
var ExampleChallengeContent = []byte("<title>Example Domain</title>")

func testHTTPProxy(host string, username string, password string) bool {
	return testHTTPProxyWithContext(context.Background(), host, username, password)
}

func testHTTPProxyWithContext(ctx context.Context, host, username, password string) bool {
	host = strings.TrimSpace(host)
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	proxy, err := url.Parse(host)
	if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" ||
		(proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Fragment != "" {
		return false
	}
	if port := proxy.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}

	// An unauthenticated probe must not reuse credentials embedded in the target.
	proxy.User = nil
	if username != "" || password != "" {
		proxy.User = url.UserPassword(username, password)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Use ordinary HTTP forwarding: some HTTP proxies do not allow CONNECT to
	// port 80. The transport adds Proxy-Authorization only to the proxy request.
	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxy),
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return netx.DialContextWithoutProxy(ctx, addr)
		},
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, // Proxy certificates may be self-signed.
		TLSHandshakeTimeout: defaultTimeout,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   defaultTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/", nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return err == nil && bytes.Contains(body, ExampleChallengeContent)
}

var httpProxyAuth = &DefaultServiceAuthInfo{
	ServiceName:  "http",
	DefaultPorts: "80",
	DefaultUsernames: []string{
		"root", "admin",
	},
	DefaultPasswords: []string{
		"root", "admin",
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testHTTPProxyWithContext(i.Context, i.Target, i.Username, i.Password)
		return result
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		result.Ok = testHTTPProxyWithContext(i.Context, i.Target, "", "")
		return result
	},
}
