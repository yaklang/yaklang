package fp

import (
	"context"
	"fmt"
	"strings"
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
