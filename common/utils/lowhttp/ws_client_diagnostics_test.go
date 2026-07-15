package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWebsocketClientReportsHandshakeTimings(t *testing.T) {
	var listener net.Listener
	var err error
	for range 10 {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
		if err == nil {
			break
		}
	}
	require.NoError(t, err)
	defer listener.Close()
	responseRaw := []byte(fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\n\r\n", ComputeWebsocketAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")))

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil || line == "\r\n" {
				break
			}
		}
		wire := append(bytes.Clone(responseRaw), 0x81, 0x02, 'o', 'k')
		_, _ = conn.Write(wire)
		<-time.After(100 * time.Millisecond)
	}()

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := net.LookupPort("tcp", portText)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/ws", nil)
	require.NoError(t, err)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	var timings WebsocketHandshakeTimings
	var capturedUpgradeResponse []byte
	frames := make(chan []byte, 2)
	client, err := NewWebsocketClientByUpgradeRequest(req,
		WithWebsocketHost(host),
		WithWebsocketPort(port),
		WithWebsocketStrictMode(true),
		WithWebsocketHandshakeTimingsHandler(func(got WebsocketHandshakeTimings) {
			timings = got
		}),
		WithWebsocketUpgradeResponseHandler(func(_ *http.Response, raw []byte, _ *WebsocketExtensions, _ error) []byte {
			capturedUpgradeResponse = bytes.Clone(raw)
			return raw
		}),
		WithWebsocketAllFrameHandler(func(_ *WebsocketClient, frame *Frame, data []byte, _ func()) {
			if frame.Type() == TextMessage {
				frames <- bytes.Clone(data)
			}
		}),
	)
	require.NoError(t, err)
	require.NotZero(t, timings.DialDuration)
	require.NotZero(t, timings.RequestWriteDuration)
	require.NotZero(t, timings.ResponseReadDuration)
	require.Equal(t, responseRaw, capturedUpgradeResponse)
	require.Equal(t, 4, client.BufferedAfterHandshake)
	client.Start()
	select {
	case frame := <-frames:
		require.Equal(t, []byte("ok"), frame)
	case <-time.After(time.Second):
		t.Fatal("buffered first frame was not delivered")
	}
	select {
	case duplicate := <-frames:
		t.Fatalf("buffered first frame was delivered twice: %q", duplicate)
	case <-time.After(30 * time.Millisecond):
	}
	require.NoError(t, client.Close())
	<-serverDone
}

func TestWebsocketClientReportsTerminalReadError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := NewWebsocketClientIns(
		clientConn,
		NewFrameReader(clientConn, false),
		NewFrameWriter(clientConn, false),
		&WebsocketExtensions{},
	)
	client.Start()
	require.NoError(t, serverConn.Close())
	select {
	case <-client.WaitChannel():
	case <-time.After(time.Second):
		t.Fatal("websocket client did not stop after peer close")
	}
	require.ErrorIs(t, client.TerminalError(), io.EOF)
}
