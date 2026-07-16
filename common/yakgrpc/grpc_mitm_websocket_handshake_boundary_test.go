package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestGRPCMUSTPASS_MITM_WebSocketChunkedUpgradeServerFirstFrame(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var listener net.Listener
	var err error
	for range 10 {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, err)
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		req, readErr := http.ReadRequest(reader)
		if readErr != nil {
			serverDone <- readErr
			return
		}
		accept := lowhttp.ComputeWebsocketAcceptKey(req.Header.Get("Sec-WebSocket-Key"))

		var firstFrame bytes.Buffer
		firstWriter := lowhttp.NewFrameWriter(&firstFrame, false)
		if writeErr := firstWriter.WriteText([]byte("server-first-through-mitm"), false); writeErr != nil {
			serverDone <- writeErr
			return
		}
		if flushErr := firstWriter.Flush(); flushErr != nil {
			serverDone <- flushErr
			return
		}
		response := []byte(fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\nTransfer-Encoding: chunked\r\nContent-Type: text/plain\r\n\r\n", accept))
		wire := append(response, firstFrame.Bytes()...)
		if _, writeErr := conn.Write(wire); writeErr != nil {
			serverDone <- writeErr
			return
		}

		clientFrame, readErr := lowhttp.NewFrameReaderFromBufio(reader, false).ReadFrame()
		if readErr != nil {
			serverDone <- readErr
			return
		}
		if clientFrame.Type() != lowhttp.TextMessage || string(clientFrame.GetData()) != "client-through-mitm" {
			serverDone <- fmt.Errorf("unexpected client frame type=%d payload=%q", clientFrame.Type(), clientFrame.GetData())
			return
		}

		writer := lowhttp.NewFrameWriter(conn, false)
		if writeErr := writer.WriteText([]byte("server-ack"), false); writeErr != nil {
			serverDone <- writeErr
			return
		}
		if flushErr := writer.Flush(); flushErr != nil {
			serverDone <- flushErr
			return
		}
		serverDone <- nil
	}()

	proxyURL := startWebsocketMITMProxy(t, ctx, cancel)
	conn, response := dialWebsocketThroughMITM(t, "ws://"+listener.Addr().String()+"/ws", proxyURL)
	defer closeWebsocketTestConnection(conn)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	messageType, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, "server-first-through-mitm", string(payload))

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("client-through-mitm")))
	messageType, payload, err = conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, "server-ack", string(payload))

	select {
	case serverErr := <-serverDone:
		require.NoError(t, serverErr)
	case <-time.After(3 * time.Second):
		t.Fatal("raw websocket server did not complete")
	}
}
