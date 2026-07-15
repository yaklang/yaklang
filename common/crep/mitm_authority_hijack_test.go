package crep

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestMITMRequestHijackAuthorityChangesUpstreamPort(t *testing.T) {
	for _, isHTTPS := range []bool{false, true} {
		isHTTPS := isHTTPS
		t.Run(map[bool]string{false: "http", true: "https"}[isHTTPS], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			var originalHits atomic.Int32
			var modifiedHits atomic.Int32
			response := func(body string) []byte {
				return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body))
			}

			originalHost, originalPort := utils.DebugMockHTTPServerWithContext(ctx, isHTTPS, false, false, false, false, func([]byte) []byte {
				originalHits.Add(1)
				return response("original")
			})
			modifiedHost, modifiedPort := utils.DebugMockHTTPServerWithContext(ctx, isHTTPS, false, false, false, false, func([]byte) []byte {
				modifiedHits.Add(1)
				return response("modified")
			})

			modifiedAddr := utils.HostPort(modifiedHost, modifiedPort)
			proxy, err := NewMITMServer(
				MITM_SetDisableSystemProxy(true),
				MITM_SetHTTPRequestHijackRaw(func(_ bool, req *http.Request, raw []byte) []byte {
					modified := lowhttp.ReplaceHTTPPacketHost(raw, modifiedAddr)
					firstLine := strings.SplitN(string(modified), "\r\n", 2)[0]
					parts := strings.SplitN(firstLine, " ", 3)
					if len(parts) == 3 {
						if requestURL, parseErr := url.Parse(parts[1]); parseErr == nil && requestURL.IsAbs() {
							requestURL.Host = modifiedAddr
							modified = lowhttp.ReplaceHTTPPacketFirstLine(modified, parts[0]+" "+requestURL.String()+" "+parts[2])
						}
					}
					httpctx.SetRequestModified(req, "test manual hijack")
					httpctx.SetHijackedRequestBytes(req, modified)
					return modified
				}),
			)
			require.NoError(t, err)

			proxyAddr := utils.HostPort("127.0.0.1", utils.GetRandomAvailableTCPPort())
			ready := make(chan struct{})
			go func() {
				_ = proxy.ServeWithListenedCallback(ctx, proxyAddr, func() { close(ready) })
			}()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("MITM proxy did not start")
			}

			originalAddr := utils.HostPort(originalHost, originalPort)
			packet := []byte(fmt.Sprintf("GET /authority-port HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", originalAddr))
			rsp, err := lowhttp.HTTPWithoutRedirect(
				lowhttp.WithRequest(packet),
				lowhttp.WithHttps(isHTTPS),
				lowhttp.WithProxy("http://"+proxyAddr),
				lowhttp.WithEnableSystemProxyFromEnv(false),
				lowhttp.WithConnectTimeout(3*time.Second),
				lowhttp.WithTimeout(5*time.Second),
			)
			require.NoError(t, err)
			require.Contains(t, string(lowhttp.GetHTTPPacketBody(rsp.RawPacket)), "modified")
			require.EqualValues(t, 0, originalHits.Load(), "the original upstream port must not receive the edited request")
			require.EqualValues(t, 1, modifiedHits.Load(), "the edited upstream port should receive the request")
		})
	}
}

func TestMITMRequestHijackHostOnlyPreservesUpstream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var receivedHost atomic.Value
	originalHost, originalPort := utils.DebugMockHTTPServerWithContext(ctx, false, false, false, false, false, func(req []byte) []byte {
		receivedHost.Store(lowhttp.GetHTTPPacketHeader(req, "Host"))
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\noriginal")
	})

	proxy, err := NewMITMServer(
		MITM_SetDisableSystemProxy(true),
		MITM_SetHTTPRequestHijackRaw(func(_ bool, req *http.Request, raw []byte) []byte {
			modified := lowhttp.ReplaceHTTPPacketHost(raw, "virtual.example")
			httpctx.SetRequestModified(req, "test host-only hijack")
			httpctx.SetHijackedRequestBytes(req, modified)
			return modified
		}),
	)
	require.NoError(t, err)

	proxyAddr := utils.HostPort("127.0.0.1", utils.GetRandomAvailableTCPPort())
	ready := make(chan struct{})
	go func() {
		_ = proxy.ServeWithListenedCallback(ctx, proxyAddr, func() { close(ready) })
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("MITM proxy did not start")
	}

	originalAddr := utils.HostPort(originalHost, originalPort)
	packet := []byte(fmt.Sprintf("GET /host-only HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", originalAddr))
	rsp, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithRequest(packet),
		lowhttp.WithProxy("http://"+proxyAddr),
		lowhttp.WithEnableSystemProxyFromEnv(false),
		lowhttp.WithConnectTimeout(3*time.Second),
		lowhttp.WithTimeout(5*time.Second),
	)
	require.NoError(t, err)
	require.Equal(t, "original", string(lowhttp.GetHTTPPacketBody(rsp.RawPacket)))
	require.Equal(t, "virtual.example", receivedHost.Load())
}
