package minimartian

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	mitmconfig "github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
)

type rawTunnelTestOpener func(*testing.T, string, string) net.Conn

// Unknown-protocol forwarding behavior contract
//
//   - SOCKS5 CONNECT and HTTP CONNECT may forward an unknown payload because
//     their handshakes provide an explicit upstream target.
//   - A TUN producer must explicitly mark its connection and provide an
//     OriginalDestination. A generic extra connection remains a normal proxy
//     frontend connection even when its LocalAddr happens to look routable.
//   - Recognized HTTP must continue through the HTTP MITM path; adding an
//     unknown fallback must not turn CONNECT or TUN into unconditional tunnels.
//   - A raw connection accepted directly by a MITM listener has no upstream
//     target. It must not use the listener's LocalAddr as a fallback because
//     that would reconnect to the proxy itself.
//
// The tests in this file exercise both sides of the contract: opaque QQ-like
// frames are echoed byte-for-byte without entering request modifiers, while
// ordinary HTTP reaches its upstream and is observed by the HTTP modifier.

func startRawTunnelTestProxy(t *testing.T, requestCount *atomic.Int32) (string, *Proxy, context.Context) {
	t.Helper()

	proxy := NewProxy()
	proxy.SetDisableSystemProxy(true)
	// Plain CONNECT tunnels only require a non-nil MITM config. These tests do
	// not exercise certificate generation or TLS interception.
	proxy.SetMITM(&mitmconfig.Config{})
	proxy.SetRequestModifier(RequestModifierFunc(func(*http.Request) error {
		requestCount.Add(1)
		return nil
	}))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- proxy.Serve(listener, ctx)
	}()

	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("timed out stopping test proxy")
		}
	})
	return listener.Addr().String(), proxy, ctx
}

func openRawTunnelTestTransparentIncoming(t *testing.T, proxy *Proxy, ctx context.Context, targetAddr string) net.Conn {
	t.Helper()
	clientConn, proxyConn := net.Pipe()
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(5*time.Second)))

	incoming := make(chan *WrapperedConn, 1)
	proxy.MergeExtraIncomingConnectionChannel(ctx, incoming)
	incoming <- NewWrapperedConnWithStrongLocalHostAndOriginalDestination(
		proxyConn,
		"127.0.0.1",
		targetAddr,
		nil,
	)
	close(incoming)
	return clientConn
}

func openRawTunnelTestSOCKS5(t *testing.T, proxyAddr, targetAddr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", proxyAddr, 3*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	require.NoError(t, err)
	authReply := make([]byte, 2)
	_, err = io.ReadFull(conn, authReply)
	require.NoError(t, err)
	require.Equal(t, []byte{0x05, 0x00}, authReply)

	host, port, err := utils.ParseStringToHostPort(targetAddr)
	require.NoError(t, err)
	ip := net.ParseIP(host).To4()
	require.NotNil(t, ip)
	request := []byte{0x05, 0x01, 0x00, 0x01}
	request = append(request, ip...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)
	_, err = conn.Write(request)
	require.NoError(t, err)

	replyHeader := make([]byte, 4)
	_, err = io.ReadFull(conn, replyHeader)
	require.NoError(t, err)
	require.Equal(t, byte(0x05), replyHeader[0])
	require.Equal(t, byte(0x00), replyHeader[1])
	var addressLength int
	switch replyHeader[3] {
	case 0x01:
		addressLength = net.IPv4len
	case 0x04:
		addressLength = net.IPv6len
	case 0x03:
		length := make([]byte, 1)
		_, err = io.ReadFull(conn, length)
		require.NoError(t, err)
		addressLength = int(length[0])
	default:
		t.Fatalf("unexpected SOCKS5 reply address type: %d", replyHeader[3])
	}
	_, err = io.ReadFull(conn, make([]byte, addressLength+2))
	require.NoError(t, err)
	return conn
}

func openRawTunnelTestHTTPConnect(t *testing.T, proxyAddr, targetAddr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", proxyAddr, 3*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetAddr, targetAddr)
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	return conn
}

func startRawTunnelTestEchoServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buffer := make([]byte, 4096)
				for {
					n, readErr := conn.Read(buffer)
					if n > 0 {
						if _, writeErr := conn.Write(buffer[:n]); writeErr != nil {
							return
						}
					}
					if readErr != nil {
						return
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	return listener.Addr().String()
}

func TestProxyTunnelUnknownProtocolTransparentForward(t *testing.T) {
	// Expected behavior: after either frontend proxy handshake has supplied a
	// target, a non-HTTP/non-TLS frame is an opaque TCP stream. Its bytes must
	// not be parsed, modified, recorded as HTTP, or replaced by a synthetic 502.
	openers := map[string]struct {
		open              rawTunnelTestOpener
		outerHTTPRequests int32
	}{
		"SOCKS5":       {open: openRawTunnelTestSOCKS5, outerHTTPRequests: 0},
		"HTTP CONNECT": {open: openRawTunnelTestHTTPConnect, outerHTTPRequests: 1},
	}

	for name, testCase := range openers {
		t.Run(name, func(t *testing.T) {
			var requestCount atomic.Int32
			proxyAddr, _, _ := startRawTunnelTestProxy(t, &requestCount)
			targetAddr := startRawTunnelTestEchoServer(t)
			conn := testCase.open(t, proxyAddr, targetAddr)
			defer conn.Close()

			// Model the QQ-like binary frame from issue #4155: a big-endian
			// length followed by binary fields and embedded NUL bytes.
			qqLikePacket := []byte{
				0x00, 0x00, 0x00, 0x20,
				0x00, 0x00, 0x00, 0x08,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x14, 0x08,
				0x00, 0x00, 0x00, 0x01,
				0x7f, 0x13, 0x37, 0x00,
			}
			_, err := conn.Write(qqLikePacket)
			require.NoError(t, err)
			echoed := make([]byte, len(qqLikePacket))
			_, err = io.ReadFull(conn, echoed)
			require.NoError(t, err)
			require.Equal(t, qqLikePacket, echoed)
			require.Equal(t, testCase.outerHTTPRequests, requestCount.Load(), "binary payload must not enter the HTTP request path")
		})
	}
}

func TestProxyTunnelHTTPStillUsesHTTPPath(t *testing.T) {
	// Expected behavior: the unknown fallback is selective. A recognizable HTTP
	// request inside the same SOCKS5/CONNECT tunnel remains interceptable HTTP.
	openers := map[string]struct {
		open                 rawTunnelTestOpener
		expectedRequestCount int32
	}{
		"SOCKS5":       {open: openRawTunnelTestSOCKS5, expectedRequestCount: 1},
		"HTTP CONNECT": {open: openRawTunnelTestHTTPConnect, expectedRequestCount: 2},
	}

	for name, testCase := range openers {
		t.Run(name, func(t *testing.T) {
			var requestCount atomic.Int32
			proxyAddr, _, _ := startRawTunnelTestProxy(t, &requestCount)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/normal-http" {
					t.Errorf("unexpected upstream path: %s", request.URL.Path)
					http.Error(writer, "unexpected path", http.StatusBadRequest)
					return
				}
				_, _ = io.WriteString(writer, "normal-http-ok")
			}))
			defer upstream.Close()
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)

			conn := testCase.open(t, proxyAddr, upstreamURL.Host)
			defer conn.Close()
			_, err = fmt.Fprintf(conn, "GET /normal-http HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", upstreamURL.Host)
			require.NoError(t, err)
			response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "normal-http-ok", string(body))
			require.Equal(t, testCase.expectedRequestCount, requestCount.Load(), "HTTP payload must still enter the HTTP MITM path")
		})
	}
}

func TestProxyTransparentIncomingUnknownProtocolForward(t *testing.T) {
	// Expected behavior: a target-aware transparent/TUN connection skips
	// frontend SOCKS5 negotiation and uses its OriginalDestination for raw
	// forwarding. The 05 02 prefix deliberately resembles a SOCKS5 greeting.
	var requestCount atomic.Int32
	_, proxy, ctx := startRawTunnelTestProxy(t, &requestCount)
	targetAddr := startRawTunnelTestEchoServer(t)
	conn := openRawTunnelTestTransparentIncoming(t, proxy, ctx, targetAddr)
	defer conn.Close()

	// A TUN connection already has its upstream target. Even a payload that
	// resembles the beginning of a SOCKS5 greeting must remain opaque data.
	qqLikePacket := []byte{
		0x05, 0x02, 0x00, 0x20,
		0x00, 0x00, 0x00, 0x08,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x14, 0x08,
		0x00, 0x00, 0x00, 0x01,
		0x7f, 0x13, 0x37, 0x00,
	}
	_, err := conn.Write(qqLikePacket)
	require.NoError(t, err)
	echoed := make([]byte, len(qqLikePacket))
	_, err = io.ReadFull(conn, echoed)
	require.NoError(t, err)
	require.Equal(t, qqLikePacket, echoed)
	require.Zero(t, requestCount.Load(), "transparent binary payload must not enter SOCKS5 or HTTP handling")
}

func TestProxyTransparentIncomingHTTPStillUsesHTTPPath(t *testing.T) {
	// Expected behavior: OriginalDestination controls the socket destination,
	// but a recognizable HTTP payload still enters MITM and preserves its Host
	// header for virtual-host routing and interception.
	var requestCount atomic.Int32
	_, proxy, ctx := startRawTunnelTestProxy(t, &requestCount)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/transparent-http" {
			t.Errorf("unexpected upstream path: %s", request.URL.Path)
			http.Error(writer, "unexpected path", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(writer, "transparent-http-ok")
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	conn := openRawTunnelTestTransparentIncoming(t, proxy, ctx, upstreamURL.Host)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "GET /transparent-http HTTP/1.1\r\nHost: transparent.example\r\nConnection: close\r\n\r\n")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "transparent-http-ok", string(body))
	require.Equal(t, int32(1), requestCount.Load(), "transparent HTTP payload must still enter the HTTP MITM path")
}
