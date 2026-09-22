package crep

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func startMITMProxyForWebsocketTest(t *testing.T, ctx context.Context, opts ...MITMConfig) string {
	t.Helper()
	opts = append([]MITMConfig{MITM_SetDisableSystemProxy(true)}, opts...)
	proxy, err := NewMITMServer(opts...)
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
	return proxyAddr
}

func websocketUpgradeTestPacket(hostPort, path string) []byte {
	return lowhttp.FixHTTPRequest([]byte(fmt.Sprintf(`GET %s HTTP/1.1
Host: %s
Origin: http://%s
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==

`, path, hostPort, hostPort)))
}

func TestMITMWebsocketHotPatchDropsUpgradeRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, port := utils.DebugMockEchoWs("drop-req")
	proxyAddr := startMITMProxyForWebsocketTest(t, ctx,
		MITM_SetHTTPRequestHijackRaw(func(_ bool, _ *http.Request, _ []byte) []byte {
			return nil
		}),
	)

	_, err := lowhttp.NewWebsocketClient(
		websocketUpgradeTestPacket(utils.HostPort(host, port), "/drop-req"),
		lowhttp.WithWebsocketProxy("http://"+proxyAddr),
		lowhttp.WithWebsocketWithContext(ctx),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upgrade websocket failed")
}

func TestMITMWebsocketHotPatchDropsUpgradeResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, port := utils.DebugMockEchoWs("drop-rsp")
	proxyAddr := startMITMProxyForWebsocketTest(t, ctx,
		MITM_SetHTTPResponseHijackRaw(func(_ bool, _ *http.Request, _ *http.Response, _ []byte, _ string) []byte {
			return nil
		}),
	)

	_, err := lowhttp.NewWebsocketClient(
		websocketUpgradeTestPacket(utils.HostPort(host, port), "/drop-rsp"),
		lowhttp.WithWebsocketProxy("http://"+proxyAddr),
		lowhttp.WithWebsocketWithContext(ctx),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upgrade websocket failed")
}

func TestMITMWebsocketHotPatchPassthroughStillUpgrades(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	host, port := utils.DebugMockEchoWs("passthrough")
	proxyAddr := startMITMProxyForWebsocketTest(t, ctx,
		MITM_SetHTTPRequestHijackRaw(func(_ bool, _ *http.Request, raw []byte) []byte {
			return raw
		}),
	)

	client, err := lowhttp.NewWebsocketClient(
		websocketUpgradeTestPacket(utils.HostPort(host, port), "/passthrough"),
		lowhttp.WithWebsocketProxy("http://"+proxyAddr),
		lowhttp.WithWebsocketWithContext(ctx),
	)
	require.NoError(t, err)
	defer client.Close()
}

func TestMITMWebsocketHotPatchModifiesUpgradeRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	var sawMarker atomic.Bool
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/modify", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Yak-Hotpatch") == "1" {
			sawMarker.Store(true)
		}
		conn, upgradeErr := upgrader.Upgrade(w, r, nil)
		if upgradeErr != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	})
	go func() {
		_ = http.Serve(lis, mux)
	}()

	proxyAddr := startMITMProxyForWebsocketTest(t, ctx,
		MITM_SetHTTPRequestHijackRaw(func(_ bool, _ *http.Request, raw []byte) []byte {
			return lowhttp.ReplaceHTTPPacketHeader(raw, "X-Yak-Hotpatch", "1")
		}),
	)

	client, err := lowhttp.NewWebsocketClient(
		websocketUpgradeTestPacket(lis.Addr().String(), "/modify"),
		lowhttp.WithWebsocketProxy("http://"+proxyAddr),
		lowhttp.WithWebsocketWithContext(ctx),
	)
	require.NoError(t, err)
	defer client.Close()
	require.True(t, sawMarker.Load(), "upstream websocket handshake should see hijacked header")
}
