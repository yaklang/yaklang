package yakgrpc

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func startWebsocketMITMProxy(t *testing.T, ctx context.Context, cancel context.CancelFunc, filterWebsocket ...bool) *url.URL {
	t.Helper()
	client, err := NewLocalClient()
	require.NoError(t, err)

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream, err := client.MITM(ctx)
	require.NoError(t, err)
	request := &ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(mitmPort),
		SetAutoForward:   true,
		AutoForwardValue: true,
	}
	if len(filterWebsocket) > 0 && filterWebsocket[0] {
		request.FilterWebsocket = true
		request.UpdateFilterWebsocket = true
	}
	require.NoError(t, stream.Send(request))

	ready := make(chan struct{})
	recvDone := make(chan struct{})
	var readyOnce sync.Once
	go func() {
		defer close(recvDone)
		for {
			response, err := stream.Recv()
			if err != nil {
				return
			}
			if message := response.GetMessage(); message != nil && strings.Contains(string(message.GetMessage()), "starting mitm serve") {
				readyOnce.Do(func() { close(ready) })
			}
		}
	}()

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("MITM proxy did not become ready")
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-recvDone:
		case <-time.After(5 * time.Second):
			t.Error("MITM receive loop did not stop")
		}
		GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
	})

	proxyURL, err := url.Parse("http://" + utils.HostPort("127.0.0.1", mitmPort))
	require.NoError(t, err)
	return proxyURL
}

func dialWebsocketThroughMITM(t *testing.T, target string, proxyURL *url.URL, configure ...func(*websocket.Dialer)) (*websocket.Conn, *http.Response) {
	t.Helper()
	dialer := *websocket.DefaultDialer
	dialer.Proxy = http.ProxyURL(proxyURL)
	dialer.HandshakeTimeout = 8 * time.Second
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // test-only certificates from Vulinbox and MITM
	for _, configureDialer := range configure {
		configureDialer(&dialer)
	}
	conn, response, err := dialer.Dial(target, nil)
	require.NoError(t, err)
	return conn, response
}

func closeWebsocketTestConnection(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	_ = conn.Close()
}

func runWebsocketMITMScenarioMatrix(t *testing.T, baseURL string, proxyURL *url.URL) {
	t.Helper()

	t.Run("echo text and binary", func(t *testing.T) {
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/echo", proxyURL)
		defer closeWebsocketTestConnection(conn)
		for _, messageType := range []int{websocket.TextMessage, websocket.BinaryMessage} {
			payload := []byte{0x00, 0x01, 0x7f, 0x80, 0xff}
			if messageType == websocket.TextMessage {
				payload = []byte("yak websocket through mitm")
			}
			require.NoError(t, conn.WriteMessage(messageType, payload))
			gotType, got, err := conn.ReadMessage()
			require.NoError(t, err)
			require.Equal(t, messageType, gotType)
			require.Equal(t, payload, got)
		}
	})

	t.Run("server first frame", func(t *testing.T) {
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/first-frame", proxyURL)
		defer closeWebsocketTestConnection(conn)
		messageType, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, messageType)
		require.Equal(t, "yak-ws-server-first", string(got))
	})

	t.Run("compression", func(t *testing.T) {
		conn, response := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/echo/compression", proxyURL, func(dialer *websocket.Dialer) {
			dialer.EnableCompression = true
		})
		defer closeWebsocketTestConnection(conn)
		require.Contains(t, response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
		payload := []byte(strings.Repeat("yak-compressed-mitm-", 256))
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, payload, got)
	})

	t.Run("ping pong", func(t *testing.T) {
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/ping", proxyURL)
		defer closeWebsocketTestConnection(conn)
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "yak-ws-pong-ok", string(got))
	})

	t.Run("close code and reason", func(t *testing.T) {
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/close?code=1008&reason=yak+policy", proxyURL)
		defer conn.Close()
		_, _, err := conn.ReadMessage()
		var closeErr *websocket.CloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, websocket.ClosePolicyViolation, closeErr.Code)
		require.Equal(t, "yak policy", closeErr.Text)
	})

	t.Run("subprotocol", func(t *testing.T) {
		conn, response := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/subprotocol", proxyURL, func(dialer *websocket.Dialer) {
			dialer.Subprotocols = []string{"unsupported", "yak-ws-v2"}
		})
		defer closeWebsocketTestConnection(conn)
		require.Equal(t, "yak-ws-v2", response.Header.Get("Sec-WebSocket-Protocol"))
		require.Equal(t, "yak-ws-v2", conn.Subprotocol())
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "yak-ws-v2", string(got))
	})

	t.Run("idle", func(t *testing.T) {
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/idle", proxyURL)
		defer conn.Close()
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(150*time.Millisecond)))
		_, _, err := conn.ReadMessage()
		var netErr net.Error
		require.True(t, errors.As(err, &netErr) && netErr.Timeout(), "expected timeout, got %v", err)
	})

	t.Run("delayed handshake", func(t *testing.T) {
		started := time.Now()
		conn, _ := dialWebsocketThroughMITM(t, baseURL+"/websocket/ws/delayed-handshake?delay_ms=120", proxyURL)
		defer closeWebsocketTestConnection(conn)
		require.GreaterOrEqual(t, time.Since(started), 100*time.Millisecond)
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "yak-ws-server-first", string(got))
	})
}

func TestGRPCMUSTPASS_MITM_WebSocketVulinboxScenarios(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	for _, test := range []struct {
		name    string
		noHTTPS bool
		scheme  string
	}{
		{name: "ws", noHTTPS: true, scheme: "ws"},
		{name: "wss", noHTTPS: false, scheme: "wss"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			vulinboxAddr, err := vulinbox.NewVulinServerEx(ctx, test.noHTTPS, false, "127.0.0.1")
			require.NoError(t, err)
			targetURL, err := url.Parse(vulinboxAddr)
			require.NoError(t, err)
			targetURL.Scheme = test.scheme
			baseURL := targetURL.String()
			proxyURL := startWebsocketMITMProxy(t, ctx, cancel)
			runWebsocketMITMScenarioMatrix(t, baseURL, proxyURL)
		})
	}
}
