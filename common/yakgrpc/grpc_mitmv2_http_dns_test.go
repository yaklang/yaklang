package yakgrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// startRecordingConnectProxyToUpstream starts an HTTP CONNECT proxy that records
// the target host from the CONNECT request line and forwards to a fixed upstream
// regardless of the requested host — simulating a proxy that resolves the domain
// on its side (mirrors the SOCKS5 mock proxy behaviour).
func startRecordingConnectProxyToUpstream(t *testing.T, upstream string) (proxyURL string, getTargets func() []string, closeFn func()) {
	t.Helper()

	var (
		mu      sync.Mutex
		targets []string
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}

		conn, rw, err := hijacker.Hijack()
		if err != nil {
			return
		}

		if r.Host != "" && r.Host != "/" {
			mu.Lock()
			targets = append(targets, r.Host)
			mu.Unlock()
		}

		upstreamConn, err := net.DialTimeout("tcp", upstream, 5*time.Second)
		if err != nil {
			_, _ = rw.WriteString("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			_ = rw.Flush()
			_ = conn.Close()
			return
		}

		_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = rw.Flush()

		go func() {
			defer upstreamConn.Close()
			defer conn.Close()
			_, _ = io.Copy(upstreamConn, conn)
		}()
		go func() {
			defer upstreamConn.Close()
			defer conn.Close()
			_, _ = io.Copy(conn, upstreamConn)
		}()
	})

	server := &http.Server{Handler: handler}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() {
		_ = server.Serve(ln)
	}()

	getTargets = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), targets...)
	}

	closeFn = func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), time.Second)
		defer c()
		_ = server.Shutdown(shutdownCtx)
		_ = ln.Close()
	}

	return "http://" + ln.Addr().String(), getTargets, closeFn
}

// These tests verify that when an HTTP CONNECT downstream proxy is configured,
// the MITM engine forwards the hostname to the proxy without any local DNS
// lookup or direct connection to the target. HTTP CONNECT proxies always send
// the original domain by default.
//
// The target domain uses the .invalid TLD (RFC 6761), which is guaranteed
// never to resolve, so any local DNS would cause the request to fail.
//
// Each test asserts three dimensions:
//  1. Domain passthrough: the CONNECT line carries the domain, not a resolved IP.
//  2. Response integrity: the UUID from the mock server returns intact.
//  3. No direct connection: the mock server is connected exactly once
//     (by the proxy), proving MITM does not bypass the proxy to reach the
//     target directly.
//
// Chain: poc.DoGET → MITM (MITMV2 gRPC, downstream HTTP proxy) → mock proxy
// → mock server (returns UUID).

func TestGRPCMUSTPASS_MITMV2_HttpProxyDNSDomainEndToEnd(t *testing.T) {
	const targetDomain = "mitmv2-dns-test.invalid"

	cert := genSelfSignedCertForDomain(t, targetDomain)
	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpPort := httpLn.Addr().(*net.TCPAddr).Port
	tlsLn := tls.NewListener(httpLn, &tls.Config{Certificates: []tls.Certificate{cert}})
	testUUID := uuid.New().String()
	var serverConnCount int32

	go func() {
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&serverConnCount, 1)
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(15 * time.Second))
				buf := make([]byte, 4096)
				c.Read(buf)
				body := fmt.Sprintf("uuid:%s", testUUID)
				resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
				c.Write([]byte(resp))
			}(conn)
		}
	}()
	defer tlsLn.Close()

	downstreamProxy, getTargets, closeProxy := startRecordingConnectProxyToUpstream(t, fmt.Sprintf("127.0.0.1:%d", httpPort))
	defer closeProxy()

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)
	err = stream.Send(&ypb.MITMV2Request{
		Host:            "127.0.0.1",
		Port:            uint32(mitmPort),
		DownstreamProxy: downstreamProxy,
	})
	require.NoError(t, err)

	mitmProxy := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)
	targetURL := fmt.Sprintf("https://%s:%d/", targetDomain, httpPort)

	type dogetResult struct {
		rsp *lowhttp.LowhttpResponse
		err error
	}
	resultCh := make(chan dogetResult, 1)

	started := false
	for {
		data, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") && !started {
				started = true
				go func() {
					time.Sleep(500 * time.Millisecond)
					rsp, _, reqErr := poc.DoGET(
						targetURL,
						poc.WithProxy(mitmProxy),
						poc.WithForceHTTPS(true),
						poc.WithHost(targetDomain),
						poc.WithTimeout(15),
					)
					resultCh <- dogetResult{rsp: rsp, err: reqErr}
					cancel()
				}()
			}
		}
	}

	require.True(t, started, "MITM server should have started")

	// 1. domain passthrough: CONNECT line carries the domain, not a resolved IP
	require.Eventually(t, func() bool {
		return len(getTargets()) > 0
	}, 10*time.Second, 100*time.Millisecond)

	targets := getTargets()
	t.Logf("CONNECT proxy received targets: %v", targets)
	require.NotEmpty(t, targets, "CONNECT proxy should have received a target")
	require.Contains(t, targets[0], targetDomain, "CONNECT proxy should receive the domain %q, not a resolved IP", targetDomain)

	// 2. response integrity: UUID returned intact through the full chain
	select {
	case res := <-resultCh:
		require.NoError(t, res.err, "poc.DoGET should succeed through MITM+HTTP proxy")
		require.NotNil(t, res.rsp, "response should not be nil")
		require.Contains(t, string(res.rsp.RawPacket), fmt.Sprintf("uuid:%s", testUUID),
			"response should contain the UUID from the mock server")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for poc.DoGET result")
	}

	// 3. no direct connection: mock server connected exactly once (by the proxy)
	require.Equal(t, int32(1), atomic.LoadInt32(&serverConnCount),
		"mock server should be connected exactly once — MITM must not connect directly to the target")

	t.Logf("MITMV2 HTTP-proxy end-to-end test passed: CONNECT received domain %q", targets[0])
}

func TestGRPCMUSTPASS_MITMV2_HttpProxyDNSHTTPNoTLS(t *testing.T) {
	const targetDomain = "mitmv2-dns-test.invalid"

	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpPort := httpLn.Addr().(*net.TCPAddr).Port
	testUUID := uuid.New().String()
	var serverConnCount int32

	go func() {
		for {
			conn, err := httpLn.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&serverConnCount, 1)
			go func(c net.Conn) {
				defer c.Close()
				c.SetDeadline(time.Now().Add(15 * time.Second))
				buf := make([]byte, 4096)
				c.Read(buf)
				body := fmt.Sprintf("uuid:%s", testUUID)
				resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
				c.Write([]byte(resp))
			}(conn)
		}
	}()
	defer httpLn.Close()

	downstreamProxy, getTargets, closeProxy := startRecordingConnectProxyToUpstream(t, fmt.Sprintf("127.0.0.1:%d", httpPort))
	defer closeProxy()

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)
	err = stream.Send(&ypb.MITMV2Request{
		Host:            "127.0.0.1",
		Port:            uint32(mitmPort),
		DownstreamProxy: downstreamProxy,
	})
	require.NoError(t, err)

	mitmProxy := fmt.Sprintf("http://127.0.0.1:%d", mitmPort)
	targetURL := fmt.Sprintf("http://%s:%d/", targetDomain, httpPort)

	type dogetResult struct {
		rsp *lowhttp.LowhttpResponse
		err error
	}
	resultCh := make(chan dogetResult, 1)

	started := false
	for {
		data, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") && !started {
				started = true
				go func() {
					time.Sleep(500 * time.Millisecond)
					rsp, _, reqErr := poc.DoGET(
						targetURL,
						poc.WithProxy(mitmProxy),
						poc.WithHost(targetDomain),
						poc.WithTimeout(15),
					)
					resultCh <- dogetResult{rsp: rsp, err: reqErr}
					cancel()
				}()
			}
		}
	}

	require.True(t, started, "MITM server should have started")

	// 1. domain passthrough
	require.Eventually(t, func() bool {
		return len(getTargets()) > 0
	}, 10*time.Second, 100*time.Millisecond)

	targets := getTargets()
	t.Logf("CONNECT proxy received targets: %v", targets)
	require.NotEmpty(t, targets, "CONNECT proxy should have received a target")
	require.Contains(t, targets[0], targetDomain, "CONNECT proxy should receive the domain %q, not a resolved IP", targetDomain)

	// 2. response integrity
	select {
	case res := <-resultCh:
		require.NoError(t, res.err, "poc.DoGET should succeed through MITM+HTTP proxy")
		require.NotNil(t, res.rsp, "response should not be nil")
		require.Contains(t, string(res.rsp.RawPacket), fmt.Sprintf("uuid:%s", testUUID),
			"response should contain the UUID from the mock server")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for poc.DoGET result")
	}

	// 3. no direct connection
	require.Equal(t, int32(1), atomic.LoadInt32(&serverConnCount),
		"mock server should be connected exactly once — MITM must not connect directly to the target")

	t.Logf("MITMV2 HTTP-proxy plain HTTP test passed: CONNECT received domain %q", targets[0])
}
