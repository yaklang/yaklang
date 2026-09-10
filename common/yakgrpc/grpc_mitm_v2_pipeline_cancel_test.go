//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITMV2_H2CancellationClearsPipeline(t *testing.T) {
	for _, mode := range []string{"reset_stream", "close_browser_connection"} {
		t.Run(mode, func(t *testing.T) {
			ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
			defer stop()
			received, canceled := make(chan struct{}, 1), make(chan struct{}, 1)
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- struct{}{}
				select {
				case <-r.Context().Done():
					canceled <- struct{}{}
				case <-ctx.Done():
				}
			}))
			origin.EnableHTTP2 = true
			origin.StartTLS()
			defer origin.Close()
			defer stop()
			client, err := NewLocalClient()
			require.NoError(t, err)
			stream, err := client.MITMV2(ctx)
			require.NoError(t, err)
			port := utils.GetRandomAvailableTCPPort()
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				Host: "127.0.0.1", Port: uint32(port), EnableHttp2: true,
				SetAutoForward: true, AutoForwardValue: true, DisableSystemProxy: true,
			}))
			require.True(t, waitMITMV2Started(stream, 10*time.Second))
			statsCh := make(chan *ypb.MITMPipelineStats, 32)
			go func() {
				defer close(statsCh)
				for {
					response, err := stream.Recv()
					if err != nil {
						return
					}
					if stats := response.GetPipelineStats(); stats != nil {
						select {
						case statsCh <- stats:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
			waitStats := func(predicate func(*ypb.MITMPipelineStats) bool) bool {
				timer := time.NewTimer(4 * time.Second)
				defer timer.Stop()
				for {
					select {
					case stats, ok := <-statsCh:
						if !ok {
							return false
						}
						if predicate(stats) {
							return true
						}
					case <-timer.C:
						return false
					}
				}
			}
			proxyURL, err := url.Parse("http://" + utils.HostPort("127.0.0.1", port))
			require.NoError(t, err)
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true}
			defer transport.CloseIdleConnections()
			requestCtx, cancelRequest := context.WithCancel(ctx)
			defer cancelRequest()
			connection := make(chan net.Conn, 1)
			requestCtx = httptrace.WithClientTrace(requestCtx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { connection <- info.Conn }})
			req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, origin.URL, nil)
			require.NoError(t, err)
			result := make(chan error, 1)
			go func() {
				response, err := (&http.Client{Transport: transport}).Do(req)
				if response != nil {
					response.Body.Close()
				}
				result <- err
			}()
			var browserConn net.Conn
			select {
			case browserConn = <-connection:
			case <-ctx.Done():
				t.Fatal("browser did not connect")
			}
			tlsConn, ok := browserConn.(*tls.Conn)
			require.True(t, ok)
			require.Equal(t, "h2", tlsConn.ConnectionState().NegotiatedProtocol)
			select {
			case <-received:
			case <-ctx.Done():
				t.Fatal("origin did not receive request")
			}
			require.True(t, waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetUpstreamActive() == 1 }))
			if mode == "reset_stream" {
				cancelRequest()
			} else {
				require.NoError(t, browserConn.Close())
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("browser cancellation did not reach origin")
			}
			select {
			case err := <-result:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("browser request did not finish")
			}
			require.True(t, waitStats(func(s *ypb.MITMPipelineStats) bool {
				return s.GetActiveTotal() == 0 && s.GetUpstreamActive() == 0 && s.GetOldestUpstreamAgeMs() == 0
			}), "origin was canceled, but MITM still reports a pending upstream request")
		})
	}
}
