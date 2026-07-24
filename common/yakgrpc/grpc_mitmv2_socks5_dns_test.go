package yakgrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
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

// genSelfSignedCertForDomain creates a TLS cert for the given domain and 127.0.0.1.
func genSelfSignedCertForDomain(t *testing.T, domain string) tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{domain},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// mockSocks5ProxyDNS is a SOCKS5 proxy that records the target address type
// (domain vs IP) and forwards to a fixed upstream regardless of the requested
// host — simulating a proxy that resolves the domain on its side.
type mockSocks5ProxyDNS struct {
	listener net.Listener
	upstream string
	mu       sync.Mutex
	atyp     byte
	host     string
	atypCh   chan byte
	hostCh   chan string
}

func newMockSocks5ProxyDNS(t *testing.T, upstream string) *mockSocks5ProxyDNS {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &mockSocks5ProxyDNS{
		listener: ln,
		upstream: upstream,
		atypCh:   make(chan byte, 10),
		hostCh:   make(chan string, 10),
	}
	go p.serve()
	return p
}

func (p *mockSocks5ProxyDNS) addr() string { return p.listener.Addr().String() }

func (p *mockSocks5ProxyDNS) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.handle(conn)
	}
}

func (p *mockSocks5ProxyDNS) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	// auth negotiation
	authBuf := make([]byte, 3)
	io.ReadFull(conn, authBuf)
	conn.Write([]byte{0x05, 0x00})

	// CONNECT request
	header := make([]byte, 4)
	io.ReadFull(conn, header)
	var host string
	switch header[3] {
	case 0x1:
		ip := make([]byte, 4)
		io.ReadFull(conn, ip)
		host = net.IP(ip).String()
	case 0x3:
		lenBuf := make([]byte, 1)
		io.ReadFull(conn, lenBuf)
		domain := make([]byte, lenBuf[0])
		io.ReadFull(conn, domain)
		host = string(domain)
	case 0x4:
		ip := make([]byte, 16)
		io.ReadFull(conn, ip)
		host = net.IP(ip).String()
	}
	portBuf := make([]byte, 2)
	io.ReadFull(conn, portBuf)
	_ = binary.BigEndian.Uint16(portBuf)

	p.mu.Lock()
	p.atyp = header[3]
	p.host = host
	p.mu.Unlock()
	p.atypCh <- header[3]
	p.hostCh <- host

	// connect to the fixed upstream (simulating proxy-side DNS resolution)
	upstream, err := net.Dial("tcp", p.upstream)
	if err != nil {
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()

	// reply success
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// bidirectional copy
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
}

func (p *mockSocks5ProxyDNS) close() {
	p.listener.Close()
}

// These tests verify that when a downstream proxy is configured, the MITM
// engine performs remote DNS resolution — it forwards the hostname to the
// proxy without any local DNS lookup or direct connection to the target.
//
// The target domain uses the .invalid TLD (RFC 6761), which is guaranteed
// never to resolve, so any local DNS would cause the request to fail.
//
// Each test asserts three dimensions:
//  1. Domain passthrough: the proxy receives the domain, not a resolved IP
//     (SOCKS5: ATYP=0x3; HTTP CONNECT: CONNECT line contains the domain).
//  2. Response integrity: the UUID from the mock server returns intact.
//  3. No direct connection: the mock server is connected exactly once
//     (by the proxy), proving MITM does not bypass the proxy to reach the
//     target directly.
//
// Chain: poc.DoGET → MITM (MITMV2 gRPC, downstream proxy) → mock proxy
// → mock server (returns UUID).

func TestGRPCMUSTPASS_MITMV2_Socks5DNSDomainEndToEnd(t *testing.T) {
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

	socksProxy := newMockSocks5ProxyDNS(t, fmt.Sprintf("127.0.0.1:%d", httpPort))
	defer socksProxy.close()

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
		DownstreamProxy: fmt.Sprintf("socks5://%s", socksProxy.addr()),
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

	// 1. domain passthrough: SOCKS5 proxy received DOMAIN (ATYP=0x3)
	select {
	case atyp := <-socksProxy.atypCh:
		t.Logf("SOCKS5 proxy received ATYP=0x%02x", atyp)
		require.Equal(t, byte(0x3), atyp, "SOCKS5 proxy should receive DOMAIN (0x3), not IP — no local DNS should happen")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SOCKS5 proxy to receive the request")
	}
	select {
	case host := <-socksProxy.hostCh:
		t.Logf("SOCKS5 proxy received host=%s", host)
		require.Equal(t, targetDomain, host, "SOCKS5 proxy should receive domain %q", targetDomain)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SOCKS5 proxy to receive host")
	}

	// 2. response integrity: UUID returned intact through the full chain
	select {
	case res := <-resultCh:
		require.NoError(t, res.err, "poc.DoGET should succeed through MITM+SOCKS5")
		require.NotNil(t, res.rsp, "response should not be nil")
		require.Contains(t, string(res.rsp.RawPacket), fmt.Sprintf("uuid:%s", testUUID),
			"response should contain the UUID from the mock server")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for poc.DoGET result")
	}

	// 3. no direct connection: mock server connected exactly once (by the proxy)
	require.Equal(t, int32(1), atomic.LoadInt32(&serverConnCount),
		"mock server should be connected exactly once — MITM must not connect directly to the target")

	t.Logf("MITMV2 SOCKS5 end-to-end test passed: domain %q (ATYP=0x3)", targetDomain)
}

func TestGRPCMUSTPASS_MITMV2_Socks5DNSHTTPNoTLS(t *testing.T) {
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

	socksProxy := newMockSocks5ProxyDNS(t, fmt.Sprintf("127.0.0.1:%d", httpPort))
	defer socksProxy.close()

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
		DownstreamProxy: fmt.Sprintf("socks5://%s", socksProxy.addr()),
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
	select {
	case atyp := <-socksProxy.atypCh:
		require.Equal(t, byte(0x3), atyp, "SOCKS5 proxy should receive DOMAIN for plain HTTP too")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
	select {
	case host := <-socksProxy.hostCh:
		require.Equal(t, targetDomain, host)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// 2. response integrity
	select {
	case res := <-resultCh:
		require.NoError(t, res.err, "poc.DoGET should succeed through MITM+SOCKS5")
		require.NotNil(t, res.rsp, "response should not be nil")
		require.Contains(t, string(res.rsp.RawPacket), fmt.Sprintf("uuid:%s", testUUID),
			"response should contain the UUID from the mock server")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for poc.DoGET result")
	}

	// 3. no direct connection
	require.Equal(t, int32(1), atomic.LoadInt32(&serverConnCount),
		"mock server should be connected exactly once — MITM must not connect directly to the target")

	t.Logf("MITMV2 SOCKS5 plain HTTP test passed: domain %q (ATYP=0x3)", targetDomain)
}
