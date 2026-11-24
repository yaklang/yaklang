package minimartian

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	mitmconfig "github.com/yaklang/yaklang/common/minimartian/mitm"
)

// Regression: CONNECT https://<ip>:<port> was broken because the MITM proxy
// minted the intercepted certificate for "127.0.0.1" whenever the ClientHello
// carried no SNI, and clients never send SNI for an IP literal (RFC 6066 §3).
// The client then rejected every interception with
//
//	x509: certificate is valid for 127.0.0.1, not <ip>
//
// The target below is a loopback alias (127.0.0.2) on the non-standard port
// 1000. It is deliberately not 127.0.0.1, otherwise a regression back to the
// hardcoded fallback could pass by coincidence. The proxy dialer redirects the
// connection to a local origin, so no test in this file touches the network.

const (
	ipLiteralTarget = "https://127.0.0.2:1000/"
	ipLiteralHost   = "127.0.0.2"
)

// startIPHTTPSProxy starts a MITM proxy whose CA is returned as a trust pool,
// and returns the proxy address. dial, when non-nil, replaces the proxy's
// dialer (used to redirect the loopback target to a local origin).
func startIPHTTPSProxy(t *testing.T, dial func(time.Duration, string) (net.Conn, error)) (string, *x509.CertPool) {
	t.Helper()

	ca, priv, err := mitmconfig.NewAuthority("mitm-ip-https-test-ca", "mitm-ip-https-test", time.Hour)
	require.NoError(t, err)
	mc, err := mitmconfig.NewConfig(ca, priv)
	require.NoError(t, err)

	proxy := NewProxy()
	proxy.SetDisableSystemProxy(true)
	proxy.SetMITM(mc)
	if dial != nil {
		proxy.SetDialer(dial)
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
			t.Error("timed out stopping test proxy")
		}
	})

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return listener.Addr().String(), pool
}

// getThroughProxy requests target through the proxy, verifying the
// intercepted certificate against the proxy CA. It returns the response, the
// leaf certificate the proxy presented to the client, and the SNI value the
// client actually sent (empty for an IP literal target).
func getThroughProxy(t *testing.T, proxyAddr string, caPool *x509.CertPool, target string) (*http.Response, *x509.Certificate, string) {
	t.Helper()

	proxyURL, err := url.Parse("http://" + proxyAddr)
	require.NoError(t, err)

	var (
		mu      sync.Mutex
		leaf    *x509.Certificate
		sentSNI string
	)
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
			VerifyConnection: func(state tls.ConnectionState) error {
				mu.Lock()
				// For a client connection ServerName is the SNI we sent.
				sentSNI = state.ServerName
				if len(state.PeerCertificates) > 0 {
					leaf = state.PeerCertificates[0]
				}
				mu.Unlock()
				return nil
			},
		},
		DisableKeepAlives: true,
	}
	t.Cleanup(transport.CloseIdleConnections)

	resp, err := (&http.Client{Transport: transport, Timeout: 15 * time.Second}).Get(target)
	mu.Lock()
	defer mu.Unlock()
	if err != nil && leaf != nil {
		t.Logf("intercepted leaf: CN=%q IPAddresses=%v DNSNames=%v", leaf.Subject.CommonName, leaf.IPAddresses, leaf.DNSNames)
	}
	require.NoError(t, err)
	return resp, leaf, sentSNI
}

func TestMITMProxyAllowsIPLiteralHTTPSOnNonStandardPort(t *testing.T) {
	// Local TLS origin. Its own certificate is irrelevant: only the
	// certificate the proxy presents to the client is ever verified.
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "origin-ok")
	}))
	t.Cleanup(origin.Close)

	var mu sync.Mutex
	var dialedTargets []string
	proxyAddr, caPool := startIPHTTPSProxy(t, func(timeout time.Duration, target string) (net.Conn, error) {
		mu.Lock()
		dialedTargets = append(dialedTargets, target)
		mu.Unlock()
		return net.DialTimeout("tcp", origin.Listener.Addr().String(), timeout)
	})

	// A verifying client must be able to complete the whole request through
	// the proxy. Before the fix this failed with
	// "certificate is valid for 127.0.0.1, not 127.0.0.2".
	resp, leaf, sentSNI := getThroughProxy(t, proxyAddr, caPool, ipLiteralTarget)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "origin-ok", string(body))

	// Precondition guard, not a restatement of the assertion below: it keeps
	// this test on the code path the fix touches. If a future client or Go
	// release started sending SNI for IP literals, the test would silently stop
	// exercising the no-SNI fallback, so it must fail loudly instead.
	require.Empty(t, sentSNI, "an IP literal target is requested without SNI (RFC 6066 section 3)")

	// The behavior under that precondition: the interception must be accepted
	// and the leaf minted for the CONNECT authority. cert() writes either an IP
	// SAN or a DNS SAN, so this also pins "not minted from a name".
	require.NotNil(t, leaf)
	var ips []string
	for _, ip := range leaf.IPAddresses {
		ips = append(ips, ip.String())
	}
	require.Equal(t, []string{ipLiteralHost}, ips, "leaf must be minted for the CONNECTed IP")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{ipLiteralHost + ":1000"}, dialedTargets, "proxy must dial the exact authority including the non-standard port")
}

// TestMITMProxyRealIPLiteralHTTPS is an opt-in live reproduction of the
// original report, skipped unless a target is supplied, so the default test
// run never reaches the network:
//
//	YAK_MITM_IP_HTTPS_TARGET=https://<ip>:<port>/ \
//	  go test ./common/minimartian/ -run TestMITMProxyRealIPLiteralHTTPS -v -count=1
func TestMITMProxyRealIPLiteralHTTPS(t *testing.T) {
	target := strings.TrimSpace(os.Getenv("YAK_MITM_IP_HTTPS_TARGET"))
	if target == "" {
		t.Skip("set YAK_MITM_IP_HTTPS_TARGET=https://<ip>:<port>/ to reproduce against a live target")
	}

	// Direct baseline. The reported origin rejects Go's default TLS 1.3
	// ClientHello (an origin quirk), so it is pinned to TLS 1.2.
	direct := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
	}, Timeout: 20 * time.Second}
	t.Cleanup(direct.CloseIdleConnections)
	directResp, err := direct.Get(target)
	require.NoError(t, err, "direct request is the baseline and must succeed")
	directBody, err := io.ReadAll(io.LimitReader(directResp.Body, 128))
	_ = directResp.Body.Close()
	require.NoError(t, err)
	t.Logf("direct: %s body=%q", directResp.Status, string(directBody))

	proxyAddr, caPool := startIPHTTPSProxy(t, nil)
	resp, leaf, _ := getThroughProxy(t, proxyAddr, caPool, target)
	defer resp.Body.Close()
	proxiedBody, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	require.NoError(t, err)
	t.Logf("via proxy: %s body=%q", resp.Status, string(proxiedBody))
	require.Equal(t, directResp.StatusCode, resp.StatusCode)
	require.NotNil(t, leaf)
	t.Logf("intercepted leaf: CN=%q IPAddresses=%v DNSNames=%v", leaf.Subject.CommonName, leaf.IPAddresses, leaf.DNSNames)
}
