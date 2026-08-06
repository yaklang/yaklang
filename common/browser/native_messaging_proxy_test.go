package browser

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestValidateNativeMessagingProxyOptions(t *testing.T) {
	require.NoError(t, ValidateNativeMessagingProxyOptions(NativeMessagingProxyOptions{
		Endpoint: "ws://127.0.0.1:64333/extension", Origin: "chrome-extension://extension-id",
	}))
	require.ErrorContains(t, ValidateNativeMessagingProxyOptions(NativeMessagingProxyOptions{
		Endpoint: "ws://example.com/extension", Origin: "chrome-extension://extension-id",
	}), "loopback")
	require.ErrorContains(t, ValidateNativeMessagingProxyOptions(NativeMessagingProxyOptions{
		Endpoint: "ws://127.0.0.1:64333/extension", Origin: "https://attacker.example",
	}), "extension origin")
	require.ErrorContains(t, ValidateNativeMessagingProxyOptions(NativeMessagingProxyOptions{
		Endpoint: "ws://127.0.0.1:64333/extension", Origin: "chrome-extension://trusted@attacker.example",
	}), "extension origin")
	require.ErrorContains(t, ValidateNativeMessagingProxyOptions(NativeMessagingProxyOptions{
		Endpoint: "ws://127.0.0.1:64333/extension", Origin: "moz-extension://extension-id/untrusted-path",
	}), "extension origin")
}

func TestNativeMessagingProxyRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "chrome-extension://extension-id", request.Header.Get("Origin"))
		connection, err := (&websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}).Upgrade(writer, request, nil)
		require.NoError(t, err)
		defer connection.Close()
		_, message, err := connection.ReadMessage()
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"hello","protocolVersion":2}`, string(message))
		require.NoError(t, connection.WriteJSON(map[string]interface{}{
			"type": "hello_ack", "protocolVersion": 2, "sessionId": "native-session",
		}))
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- RunNativeMessagingProxy(ctx, inputReader, outputWriter, NativeMessagingProxyOptions{
			Endpoint: strings.Replace(server.URL, "http://", "ws://", 1),
			Origin:   "chrome-extension://extension-id",
		})
	}()
	require.NoError(t, WriteNativeMessagingMessage(inputWriter, json.RawMessage(`{"type":"hello","protocolVersion":2}`)))
	response, err := ReadNativeMessagingMessage(outputReader)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"hello_ack","protocolVersion":2,"sessionId":"native-session"}`, string(response))
	cancel()
	_ = inputWriter.Close()
	select {
	case <-proxyDone:
	case <-time.After(time.Second):
		t.Fatal("native messaging proxy did not stop")
	}
}
