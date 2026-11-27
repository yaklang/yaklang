package fp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAssetInventoryBannerDetection(t *testing.T) {
	rules, err := ParseNmapServiceProbeToRuleMap([]byte(`Probe TCP NULL q||
rarity 1
match ssh m|^SSH-2\.0-OpenSSH_| p/OpenSSH/
match smtp m|^220 localhost ESMTP| p/Postfix/
Probe TCP GetRequest q|GET / HTTP/1.0\r\n\r\n|
rarity 1
match http m|^HTTP/1\.[01] | p/HTTP fixture/
Probe TCP InventoryFixtureMetadata q|GET /fixture-meta HTTP/1.0\r\n\r\n|
rarity 1
match http m|^HTTP/1\.[01] | p/HTTP fixture/
`))
	require.NoError(t, err)
	require.Len(t, rules, 3)
	cfg := NewConfig(WithNmapRule(rules), WithActiveMode(true), WithPreferVerifiedHTTP(true), WithProbeTimeout(3*time.Second))
	matcher, err := NewDefaultFingerprintMatcher(cfg)
	require.NoError(t, err)

	t.Run("verified_http_releases_silent_banner_probe", func(t *testing.T) {
		var root, metadata atomic.Int64
		site := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				root.Add(1)
			case "/fixture-meta":
				metadata.Add(1)
			}
			_, _ = io.WriteString(w, "inventory test")
		}))
		site.Config.ReadHeaderTimeout = 3 * time.Second
		site.Start()
		defer site.Close()

		// The matcher must return HTTP before the fixture closes a silent connection.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		addr := site.Listener.Addr().(*net.TCPAddr)
		result, err := matcher.matchWithContext(ctx, addr.IP, addr.Port, addr.IP.String(), cfg)
		require.NoError(t, err)
		require.True(t, result.IsOpen())
		require.Equal(t, "http", result.GetServiceName())
		require.True(t, result.Fingerprint.confirmedPlainHTTP)
		require.EqualValues(t, 1, root.Load())
		require.EqualValues(t, 1, metadata.Load())
	})

	for _, service := range []struct{ name, banner string }{
		{name: "ssh", banner: "SSH-2.0-OpenSSH_9.6\r\n"},
		{name: "smtp", banner: "220 localhost ESMTP Postfix\r\n"},
	} {
		for _, timing := range []struct {
			name         string
			delayed      bool
			wantAccepted int64
		}{
			{name: "immediate", wantAccepted: 1},
			{name: "delayed", delayed: true, wantAccepted: 2},
		} {
			t.Run(service.name+"/"+timing.name, func(t *testing.T) {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				require.NoError(t, err)
				var accepted atomic.Int64
				var fixtures sync.WaitGroup
				httpConnected := make(chan struct{})
				stop := make(chan struct{})
				fixtures.Add(1)
				go func() {
					defer fixtures.Done()
					for {
						conn, err := listener.Accept()
						if err != nil {
							return
						}
						index := accepted.Add(1)
						if index == 2 {
							close(httpConnected)
						}
						fixtures.Add(1)
						go func() {
							defer fixtures.Done()
							defer conn.Close()
							if index == 1 && timing.delayed {
								select {
								case <-httpConnected:
								case <-stop:
									return
								}
							}
							_, _ = io.WriteString(conn, service.banner)
						}()
					}
				}()
				t.Cleanup(func() {
					close(stop)
					_ = listener.Close()
					fixtures.Wait()
				})
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				addr := listener.Addr().(*net.TCPAddr)
				result, err := matcher.matchWithContext(ctx, addr.IP, addr.Port, addr.IP.String(), cfg)
				require.NoError(t, err)
				require.True(t, result.IsOpen())
				require.Equal(t, service.name, result.GetServiceName())
				require.Contains(t, result.GetBanner(), service.banner[:len(service.banner)-2])
				require.Equal(t, timing.wantAccepted, accepted.Load())
			})
		}
	}
}
