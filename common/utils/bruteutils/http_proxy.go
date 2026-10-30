package bruteutils

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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
	port := 80
	if proxy.Scheme == "https" {
		port = 443
	}
	if rawPort := proxy.Port(); rawPort != "" {
		port, err = strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Use ordinary HTTP forwarding: some HTTP proxies do not allow CONNECT to
	// port 80. Send the absolute-form request directly to the proxy endpoint.
	packet, err := lowhttp.BuildLegacyProxyRequest(lowhttp.BasicRequest())
	if err != nil {
		return false
	}
	// Use only the supplied credentials, ignoring any embedded in the target URL.
	if username != "" || password != "" {
		packet = lowhttp.ReplaceHTTPPacketHeader(packet, "Proxy-Authorization", "Basic "+codec.EncodeBase64(username+":"+password))
	}
	response, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes(packet),
		lowhttp.WithHost(proxy.Hostname()),
		lowhttp.WithPort(port),
		lowhttp.WithHttps(proxy.Scheme == "https"),
		lowhttp.WithContext(ctx),
		lowhttp.WithConnectTimeout(defaultTimeout),
		lowhttp.WithTimeout(defaultTimeout),
		lowhttp.WithRedirectTimes(0),
		lowhttp.WithEnableSystemProxyFromEnv(false),
		lowhttp.WithExtendDialXOption(netx.DialX_WithDisableProxy(true)),
		lowhttp.WithConnPool(false),
		lowhttp.WithDisableSession(true),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithMaxContentLength(1<<20),
	)
	return err == nil && response != nil && response.GetStatusCode() == http.StatusOK &&
		bytes.Contains(lowhttp.GetHTTPPacketBody(response.RawPacket), ExampleChallengeContent)
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
