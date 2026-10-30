package fp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestBuildWebPathRequestPacket_Order(t *testing.T) {
	webPath := "/admin/login"
	target := "127.0.0.1:8080"

	packet := string(buildWebPathRequestPacket(webPath, target))
	lines := strings.Split(packet, "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	require.Equal(t, "GET "+webPath+" HTTP/1.1", lines[0])
	require.Equal(t, "Host: "+target, lines[1])
}

type capturedHTTPRequest struct {
	firstLine string
	host      string
	path      string
}

func TestWebDetector_RouteRequestUsesPathAndHostOrder(t *testing.T) {
	const probePath = "/yak-webdetector-order-check"
	const probeToken = "yak-webdetector-order-ok"

	reqCh := make(chan capturedHTTPRequest, 64)
	mockCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host, port := utils.DebugMockHTTPExContext(mockCtx, func(req []byte) []byte {
		headers, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		firstLine := ""
		if headers != "" {
			lines := strings.Split(strings.ReplaceAll(headers, "\r\n", "\n"), "\n")
			if len(lines) > 0 {
				firstLine = lines[0]
			}
		}

		path := lowhttp.GetHTTPRequestPathWithoutQuery(req)
		hostHeader := lowhttp.GetHTTPPacketHeader(req, "Host")
		reqCh <- capturedHTTPRequest{
			firstLine: firstLine,
			host:      hostHeader,
			path:      path,
		}

		body := "root"
		if path == probePath {
			body = probeToken
		}
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body))
	})

	testRule := &rule.FingerPrintRule{
		Method:  "exp",
		WebPath: probePath,
		MatchParam: &rule.MatchMethodParam{
			Info: &schema.CPE{
				Vendor:  "yaklang",
				Product: "webdetector_order_check",
			},
			Params: []any{"body", probeToken},
			Op:     "=",
		},
	}

	config := NewConfig(
		WithOnlyEnableWebFingerprint(true),
		WithWebFingerprintRule([]*rule.FingerPrintRule{testRule}),
		WithProbeTimeout(2*time.Second),
		WithProbesMax(1),
	)
	config.DisableDefaultFingerprint = true

	matcher, err := NewDefaultFingerprintMatcher(config)
	require.NoError(t, err)

	result, err := matcher.MatchWithContext(context.Background(), host, port)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OPEN, result.State)

	expectedHost := utils.HostPort(host, port)
	probeReq := waitForPathRequest(t, reqCh, probePath, 5*time.Second)
	require.Equal(t, "GET "+probePath+" HTTP/1.1", probeReq.firstLine)
	require.Equal(t, expectedHost, probeReq.host)
}

func waitForPathRequest(t *testing.T, reqCh <-chan capturedHTTPRequest, path string, timeout time.Duration) capturedHTTPRequest {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case req := <-reqCh:
			if req.path == path {
				return req
			}
		case <-timer.C:
			t.Fatalf("timeout waiting for request path %s", path)
		}
	}
}

func TestWebDetector_HTTPWaitHonorsProbeContext(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		for _, cancelRequest := range []bool{false, true} {
			t.Run(fmt.Sprintf("pooled=%v/cancel=%v", pooled, cancelRequest), func(t *testing.T) {
				started := make(chan struct{}, 1)
				release := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case started <- struct{}{}:
					default:
					}
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}))
				defer server.Close()
				defer close(release)
				host, port, err := utils.ParseStringToHostPort(server.URL)
				require.NoError(t, err)
				config := NewConfig(WithProbeTimeout(100 * time.Millisecond))
				if cancelRequest {
					config.ProbeTimeout = 5 * time.Second
				}
				config.WebScanDisableConnPool = pooled
				matcher := &Matcher{Config: config}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				type outcome struct {
					open bool
					err  error
				}
				done := make(chan outcome, 1)
				go func() {
					// TLS has already established TCP reachability. Isolate the HTTP read.
					open, _, _, _, err := matcher.fetchBannerFromHostPortWithTLSSkipped(true, ctx, nil, nil, host, port, 20480, "")
					done <- outcome{open, err}
				}()
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("HTTP request did not start")
				}
				if cancelRequest {
					cancel()
				}
				select {
				case result := <-done:
					require.Error(t, result.err)
					require.True(t, result.open, "a slow HTTP response must not erase TCP reachability")
				case <-time.After(time.Second):
					t.Fatal("HTTP read outlived the probe timeout or scan cancellation")
				}
			})
		}
	}
}

func TestWebDetector_HTTPFailurePreservesPriorResult(t *testing.T) {
	for _, state := range []PortState{CLOSED, OPEN} {
		t.Run(string(state), func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			defer close(release)
			host, port, err := utils.ParseStringToHostPort(server.URL)
			require.NoError(t, err)
			matcher := &Matcher{Config: NewConfig(WithOnlyEnableWebFingerprint(true), WithProbeTimeout(200*time.Millisecond))}
			prior := &MatchResult{Target: host, Port: port, State: state}
			if state == OPEN {
				prior.Fingerprint = &FingerprintInfo{IP: host, Port: port, Proto: TCP,
					ServiceName: "fixture-service", Banner: "fixture-banner"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := matcher.webDetector(prior, ctx, matcher.Config, host, net.ParseIP(host), port)
			require.NoError(t, err)
			require.Equal(t, state, result.State, "TCP reachability alone must not change the web scan result")
			require.NotEmpty(t, result.Reason)
			require.NotNil(t, result.Fingerprint)
			require.True(t, result.Fingerprint.CheckedTLS, "retain TLS probe metadata for service fingerprint fallback")
			require.Empty(t, result.Fingerprint.HttpFlows)
			if state == OPEN {
				require.Equal(t, "fixture-service", result.Fingerprint.ServiceName)
				require.Equal(t, "fixture-banner", result.Fingerprint.Banner)
			}
		})
	}
}

func TestWebDetector_RedirectsShareHTTPProbeTimeout(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		timer := time.NewTimer(120 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer server.Close()
	host, port, err := utils.ParseStringToHostPort(server.URL)
	require.NoError(t, err)
	matcher := &Matcher{Config: NewConfig(WithProbeTimeout(180 * time.Millisecond))}
	start := time.Now()
	open, _, _, _, _ := matcher.fetchBannerFromHostPortWithTLSSkipped(true, context.Background(), nil, nil, host, port, 20480, "")
	require.Less(t, time.Since(start), 500*time.Millisecond, "redirect hops must not restart the HTTP probe timeout")
	require.True(t, open)
	require.LessOrEqual(t, requests.Load(), int32(2))
}

func TestWebDetector_PassiveScanDoesNotFetchExtraPaths(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprintf("active=%v", active), func(t *testing.T) {
			reqCh := make(chan capturedHTTPRequest, 64)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				path := req.URL.Path
				reqCh <- capturedHTTPRequest{path: path}
				body := "root-token"
				if path == "/favicon.ico" {
					body = "icon-token"
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			host, port, err := utils.ParseStringToHostPort(server.URL)
			require.NoError(t, err)
			makeRule := func(path, token string) *rule.FingerPrintRule {
				return &rule.FingerPrintRule{Method: "exp", WebPath: path, MatchParam: &rule.MatchMethodParam{
					Info: &schema.CPE{Product: token}, Params: []any{"body", token}, Op: "=",
				}}
			}
			config := NewConfig(WithOnlyEnableWebFingerprint(true), WithActiveMode(active), WithDisableIotWebFingerprint(true),
				WithWebFingerprintRule([]*rule.FingerPrintRule{makeRule("/", "root-token"), makeRule("/favicon.ico", "icon-token")}),
				WithProbeTimeout(time.Second))
			config.DisableDefaultFingerprint = true
			matcher, err := NewDefaultFingerprintMatcher(config)
			require.NoError(t, err)
			result, err := matcher.Match(host, port)
			require.NoError(t, err)
			require.Equal(t, OPEN, result.State)
			require.Contains(t, strings.Join(result.Fingerprint.CPEs, " "), "root-token")
			if active {
				require.Contains(t, strings.Join(result.Fingerprint.CPEs, " "), "icon-token")
			} else {
				require.NotContains(t, strings.Join(result.Fingerprint.CPEs, " "), "icon-token")
			}
			close(reqCh)
			for req := range reqCh {
				if !active {
					require.Equal(t, "/", req.path)
				}
			}
		})
	}
}

func TestWebDetector_ConcurrentTargetsKeepTheirOwnPathResponses(t *testing.T) {
	var rules []*rule.FingerPrintRule
	var servers []*httptest.Server
	for _, token := range []string{"first-target", "second-target"} {
		token := token
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/favicon.ico" {
				fmt.Fprint(w, token)
			} else {
				fmt.Fprint(w, "root")
			}
		}))
		defer server.Close()
		servers = append(servers, server)
		rules = append(rules, &rule.FingerPrintRule{Method: "exp", WebPath: "/favicon.ico", MatchParam: &rule.MatchMethodParam{
			Info: &schema.CPE{Product: token}, Params: []any{"body", token}, Op: "=",
		}})
	}
	config := NewConfig(WithOnlyEnableWebFingerprint(true), WithDisableIotWebFingerprint(true),
		WithWebFingerprintRule(rules), WithProbeTimeout(time.Second))
	config.DisableDefaultFingerprint = true
	matcher, err := NewDefaultFingerprintMatcher(config)
	require.NoError(t, err)
	var workers sync.WaitGroup
	for i, server := range servers {
		host, port, err := utils.ParseStringToHostPort(server.URL)
		require.NoError(t, err)
		workers.Add(1)
		go func(i int, host string, port int) {
			defer workers.Done()
			result, err := matcher.Match(host, port)
			if err != nil || result == nil {
				t.Errorf("target %d failed: %v", i, err)
				return
			}
			cpes := strings.Join(result.Fingerprint.CPEs, " ")
			if !strings.Contains(cpes, rules[i].MatchParam.Info.Product) || strings.Contains(cpes, rules[1-i].MatchParam.Info.Product) {
				t.Errorf("target %d used another target's path response: %s", i, cpes)
			}
		}(i, host, port)
	}
	workers.Wait()
}
