package vulinbox

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func startWebsocketScenarioServer(t *testing.T) string {
	t.Helper()
	t.Setenv("YAKIT_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr, err := NewVulinServerEx(ctx, true, false, "127.0.0.1")
	require.NoError(t, err)
	return "ws" + strings.TrimPrefix(addr, "http")
}

func dialWebsocketScenario(t *testing.T, baseURL, path string, configure ...func(*websocket.Dialer)) (*websocket.Conn, *websocket.Dialer) {
	t.Helper()
	dialer := *websocket.DefaultDialer
	for _, configureDialer := range configure {
		configureDialer(&dialer)
	}
	conn, _, err := dialer.Dial(baseURL+path, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, &dialer
}

func TestWebsocketScenarioEndpoints(t *testing.T) {
	baseURL := startWebsocketScenarioServer(t)

	t.Run("echo preserves text and binary", func(t *testing.T) {
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/echo")
		for _, messageType := range []int{websocket.TextMessage, websocket.BinaryMessage} {
			payload := []byte{0x00, 0x01, 0x7f, 0x80, 0xff}
			if messageType == websocket.TextMessage {
				payload = []byte("yak websocket text")
			}
			require.NoError(t, conn.WriteMessage(messageType, payload))
			gotType, got, err := conn.ReadMessage()
			require.NoError(t, err)
			require.Equal(t, messageType, gotType)
			require.Equal(t, payload, got)
		}
	})

	t.Run("server sends first frame immediately", func(t *testing.T) {
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/first-frame")
		messageType, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, messageType)
		require.Equal(t, websocketServerFirstMessage, string(got))
	})

	t.Run("compression is negotiated", func(t *testing.T) {
		dialer := *websocket.DefaultDialer
		dialer.EnableCompression = true
		conn, response, err := dialer.Dial(baseURL+"/websocket/ws/echo/compression", nil)
		require.NoError(t, err)
		defer conn.Close()
		require.Contains(t, response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
		payload := []byte(strings.Repeat("compressible-yak-ws-", 128))
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, payload, got)
	})

	t.Run("ping receives pong through the client", func(t *testing.T) {
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/ping")
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocketPongMessage, string(got))
	})

	t.Run("close code and reason are preserved", func(t *testing.T) {
		path := "/websocket/ws/close?code=1008&reason=" + url.QueryEscape("yak policy")
		conn, _ := dialWebsocketScenario(t, baseURL, path)
		_, _, err := conn.ReadMessage()
		var closeErr *websocket.CloseError
		require.ErrorAs(t, err, &closeErr)
		require.Equal(t, websocket.ClosePolicyViolation, closeErr.Code)
		require.Equal(t, "yak policy", closeErr.Text)
	})

	t.Run("subprotocol is negotiated", func(t *testing.T) {
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/subprotocol", func(dialer *websocket.Dialer) {
			dialer.Subprotocols = []string{"unsupported", "yak-ws-v2"}
		})
		require.Equal(t, "yak-ws-v2", conn.Subprotocol())
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "yak-ws-v2", string(got))
	})

	t.Run("idle connection sends no data", func(t *testing.T) {
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/idle")
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(150*time.Millisecond)))
		_, _, err := conn.ReadMessage()
		var netErr net.Error
		require.True(t, errors.As(err, &netErr) && netErr.Timeout(), "expected timeout, got %v", err)
	})

	t.Run("handshake delay is observable", func(t *testing.T) {
		started := time.Now()
		conn, _ := dialWebsocketScenario(t, baseURL, "/websocket/ws/delayed-handshake?delay_ms=120")
		require.GreaterOrEqual(t, time.Since(started), 100*time.Millisecond)
		_, got, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocketServerFirstMessage, string(got))
	})
}
