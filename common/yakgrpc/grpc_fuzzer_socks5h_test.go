package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_Socks5hRemoteDNS(t *testing.T) {
	const targetDomain = "fuzzer-socks5h.invalid"
	const responseBody = "fuzzer-socks5h-ok"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, responseBody)
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	socksProxy := newMockSocks5ProxyDNS(t, upstreamURL.Host)
	defer socksProxy.close()
	_, port, err := net.SplitHostPort(upstreamURL.Host)
	require.NoError(t, err)

	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		RequestRaw:               []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s:%s\r\nConnection: close\r\n\r\n", targetDomain, port)),
		Proxy:                    "socks5h://" + socksProxy.addr(),
		NoSystemProxy:            true,
		Concurrent:               1,
		PerRequestTimeoutSeconds: 5,
	})
	require.NoError(t, err)

	var responseCount int
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.True(t, response.GetOk(), response.GetReason())
		require.Contains(t, string(response.GetResponseRaw()), responseBody)
		responseCount++
	}
	require.Equal(t, 1, responseCount)
	select {
	case atyp := <-socksProxy.atypCh:
		require.Equal(t, byte(3), atyp, "target must be sent as a SOCKS5 domain")
	case <-ctx.Done():
		t.Fatal("SOCKS5 proxy did not receive a CONNECT request")
	}
	select {
	case host := <-socksProxy.hostCh:
		require.Equal(t, targetDomain, host)
	case <-ctx.Done():
		t.Fatal("SOCKS5 proxy did not receive the target domain")
	}
}
