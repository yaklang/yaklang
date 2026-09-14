package crep

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minimartian"
	mitmconfig "github.com/yaklang/yaklang/common/minimartian/mitm"
)

// A bypass route has no proxy endpoints by design. MITMServer must preserve
// that empty entry while copying configuration into minimartian; otherwise the
// default downstream proxy silently wins for both HTTP and unknown tunnels.
func TestMITMServer_ApplyProxyConfig_PreservesBypassRoute(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer upstream.Close()
	go func() {
		conn, acceptErr := upstream.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		buffer := make([]byte, 64)
		n, readErr := conn.Read(buffer)
		if readErr == nil && n > 0 {
			_, _ = conn.Write(buffer[:n])
		}
	}()

	proxy := minimartian.NewProxy()
	proxy.SetMITM(&mitmconfig.Config{})
	proxy.SetDisableSystemProxy(true)
	server := &MITMServer{
		proxy:           proxy,
		proxyUrlStrings: []string{"http://127.0.0.1:1"},
		proxyRouteMap: map[string][]string{
			"!127.0.0.1": nil,
		},
	}
	server.applyProxyConfig()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- proxy.Serve(proxyListener, ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = proxyListener.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("timed out stopping minimartian proxy")
		}
	})

	conn, err := net.DialTimeout("tcp", proxyListener.Addr().String(), 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", upstream.Addr(), upstream.Addr())
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	payload := []byte{0x00, 0x00, 0x00, 0x09, 0x51, 0x51}
	_, err = conn.Write(payload)
	require.NoError(t, err)
	echo := make([]byte, len(payload))
	_, err = io.ReadFull(conn, echo)
	require.NoError(t, err)
	require.Equal(t, payload, echo)
}
