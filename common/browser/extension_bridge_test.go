package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestDecodeStrictExtensionBridgeJSONRejectsUnknownAndTrailingFields(t *testing.T) {
	var envelope ExtensionBridgeEnvelope
	require.NoError(t, decodeStrictExtensionBridgeJSON(
		[]byte(`{"type":"pong","id":"heartbeat-1","sequence":1,"timestamp":1}`),
		&envelope,
	))
	require.ErrorContains(t, decodeStrictExtensionBridgeJSON(
		[]byte(`{"type":"pong","legacy":true}`),
		&envelope,
	), "unknown field")
	require.ErrorContains(t, decodeStrictExtensionBridgeJSON(
		[]byte(`{"type":"pong"} {"type":"pong"}`),
		&envelope,
	), "trailing data")
}

func TestExtensionBridgeRequiresToken(t *testing.T) {
	_, err := newLegacyExtensionBridgeServer(0, "")
	require.ErrorContains(t, err, "token is required")
}

func TestExtensionBridgeResponseIsBoundToTargetConnection(t *testing.T) {
	target := &websocket.Conn{}
	other := &websocket.Conn{}
	response := make(chan ExtensionBridgeEnvelope, 1)
	server := &ExtensionBridgeServer{pending: map[string]extensionBridgePendingCall{
		"request-1": {connection: target, response: response},
	}}
	message := ExtensionBridgeEnvelope{ID: "request-1", Type: "response", Result: json.RawMessage(`{"ok":true}`)}

	server.deliverResponse(other, message)
	select {
	case <-response:
		t.Fatal("response from another browser connection completed the request")
	default:
	}
	server.deliverResponse(target, message)
	require.Equal(t, message, <-response)
}

func TestExtensionBridgeRejectsWebOrigin(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"https://attacker.example"}}
	connection, response, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.Error(t, err)
	require.Nil(t, connection)
	require.NotNil(t, response)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestExtensionBridgeCallRoundTrip(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", Version: "1.0.0",
		ProtocolVersion: extensionBridgeProtocolVersion, Capabilities: []string{"browser.context"}, InstallationID: "install-test",
	}))
	var helloAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&helloAck))
	require.Equal(t, "hello_ack", helloAck.Type)
	require.Equal(t, extensionBridgeProtocolVersion, helloAck.ProtocolVersion)
	require.NotEmpty(t, helloAck.SessionID)
	require.NotEmpty(t, helloAck.EngineInstanceID)
	require.NotEmpty(t, helloAck.ConnectionID)
	require.Contains(t, helloAck.Capabilities, "yakit.web_fuzzer.open")
	require.Contains(t, helloAck.Capabilities, "yakit.poc.generate")
	require.Contains(t, helloAck.Capabilities, "yakit.browser_request.prepare_analysis")
	require.Contains(t, helloAck.Capabilities, "yakit.browser_authorization.task")
	require.Contains(t, helloAck.Capabilities, "yakit.browser_authorization.open")
	require.Eventually(t, func() bool {
		return server.Status()["connected"] == true
	}, time.Second, 10*time.Millisecond)

	clientDone := make(chan error, 1)
	go func() {
		var request ExtensionBridgeEnvelope
		if readErr := connection.ReadJSON(&request); readErr != nil {
			clientDone <- readErr
			return
		}
		if request.Type != "request" || request.Method != "browser.context" {
			clientDone <- &unexpectedBridgeMessageError{request: request}
			return
		}
		result, _ := json.Marshal(map[string]interface{}{"title": "Authenticated page", "loggedIn": true})
		clientDone <- connection.WriteJSON(ExtensionBridgeEnvelope{ID: request.ID, Type: "response", Result: result})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := server.Call(ctx, "browser.context", map[string]interface{}{"includeCookies": true})
	require.NoError(t, err)
	require.JSONEq(t, `{"title":"Authenticated page","loggedIn":true}`, string(result))
	require.NoError(t, <-clientDone)

	status := server.Status()
	require.Equal(t, true, status["connected"])
	require.Equal(t, "unit-test", status["client"])
}

func TestExtensionBridgeRejectsIncompatibleProtocol(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", Version: "1.0.0", ProtocolVersion: 999,
	}))
	var rejection ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&rejection))
	require.NotNil(t, rejection.Error)
	require.Equal(t, "unauthorized", rejection.Error.Code)
	require.Equal(t, false, server.Status()["connected"])
}

func TestExtensionBridgeSendsCancelWhenContextEnds(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", Version: "1.0.0",
		ProtocolVersion: extensionBridgeProtocolVersion, Capabilities: []string{"browser.eval"}, InstallationID: "install-test",
	}))
	var helloAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&helloAck))

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	callDone := make(chan error, 1)
	go func() {
		_, callErr := server.Call(ctx, "browser.eval", map[string]interface{}{"code": "longRunning()"})
		callDone <- callErr
	}()

	var request ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&request))
	require.Equal(t, "request", request.Type)
	var cancelMessage ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&cancelMessage))
	require.Equal(t, "cancel", cancelMessage.Type)
	require.Equal(t, request.ID, cancelMessage.ID)
	require.ErrorIs(t, <-callDone, context.DeadlineExceeded)
}

func TestExtensionBridgeReceivesEvents(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", Version: "1.0.0",
		ProtocolVersion: extensionBridgeProtocolVersion, Capabilities: []string{"browser.handoff.request"}, InstallationID: "install-test",
	}))
	var helloAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&helloAck))

	params, err := json.Marshal(map[string]interface{}{"id": "handoff-1", "state": "completed"})
	require.NoError(t, err)
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "event", Method: "browser.handoff.changed", Params: params,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	event, err := server.WaitEvent(ctx)
	require.NoError(t, err)
	require.Equal(t, "browser.handoff.changed", event["method"])
	require.Equal(t, map[string]interface{}{"id": "handoff-1", "state": "completed"}, event["params"])
}

func TestExtensionBridgeRejectsInvalidWebFuzzerRequest(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", Version: "1.0.0",
		ProtocolVersion: extensionBridgeProtocolVersion, InstallationID: "install-test",
	}))
	var helloAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&helloAck))

	params, err := json.Marshal(map[string]interface{}{"rawRequestBase64": "not-base64", "isHttps": true})
	require.NoError(t, err)
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		ID: "extension-request-1", Type: "request", Method: "yakit.web_fuzzer.open", Params: params,
	}))
	var response ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&response))
	require.Equal(t, "response", response.Type)
	require.Equal(t, "extension-request-1", response.ID)
	require.NotNil(t, response.Error)
	require.Equal(t, "invalid_params", response.Error.Code)
}

func TestExtensionBridgeRotatesTokenAndResumesSession(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}

	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", ProtocolVersion: extensionBridgeProtocolVersion,
		InstallationID: "install-resume", TaskID: "task-1", GrantID: "grant-1",
	}))
	var firstAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&firstAck))
	require.False(t, firstAck.Resumed)
	params, _ := json.Marshal(map[string]string{"token": strings.Repeat("n", 40)})
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{ID: "rotate-1", Type: "request", Method: "system.pairing.rotate", Params: params}))
	var rotateResponse ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&rotateResponse))
	require.Nil(t, rotateResponse.Error)
	require.NoError(t, connection.Close())

	rejected, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	require.NoError(t, rejected.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", ProtocolVersion: extensionBridgeProtocolVersion, InstallationID: "install-resume",
	}))
	var rejection ExtensionBridgeEnvelope
	require.NoError(t, rejected.ReadJSON(&rejection))
	require.NotNil(t, rejection.Error)
	_ = rejected.Close()

	resumed, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resumed.Close() })
	require.NoError(t, resumed.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: strings.Repeat("n", 40), Client: "unit-test", ProtocolVersion: extensionBridgeProtocolVersion,
		InstallationID: "install-resume", ResumeSessionID: firstAck.SessionID, TaskID: "task-1", GrantID: "grant-1",
	}))
	var resumedAck ExtensionBridgeEnvelope
	require.NoError(t, resumed.ReadJSON(&resumedAck))
	require.True(t, resumedAck.Resumed)
	require.Equal(t, firstAck.SessionID, resumedAck.SessionID)
	require.Equal(t, firstAck.EngineInstanceID, resumedAck.EngineInstanceID)
	require.NotEqual(t, firstAck.ConnectionID, resumedAck.ConnectionID)
}

func TestExtensionBridgeChunksLargeMessages(t *testing.T) {
	server, err := newLegacyExtensionBridgeServer(0, "test-token")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	header := http.Header{"Origin": []string{"chrome-extension://test-extension"}}
	connection, _, err := websocket.DefaultDialer.Dial(server.URL(), header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "hello", Token: "test-token", Client: "unit-test", ProtocolVersion: extensionBridgeProtocolVersion,
		InstallationID: "install-chunk",
	}))
	var helloAck ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&helloAck))

	large := strings.Repeat("x", extensionBridgeChunkThreshold+1024)
	callDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, callErr := server.Call(ctx, "browser.context", map[string]string{"blob": large})
		callDone <- callErr
	}()
	assemblies := make(map[string]*extensionBridgeChunkAssembly)
	var request ExtensionBridgeEnvelope
	require.NoError(t, server.readConnectionJSON(connection, assemblies, &request))
	require.Equal(t, "request", request.Type)
	require.Contains(t, string(request.Params), large[:1024])
	result, _ := json.Marshal(map[string]bool{"ok": true})
	require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{ID: request.ID, Type: "response", Result: result}))
	require.NoError(t, <-callDone)

	largeEvent, _ := json.Marshal(map[string]string{"blob": large})
	envelope, _ := json.Marshal(ExtensionBridgeEnvelope{Type: "event", Method: "large.event", Params: largeEvent})
	total := (len(envelope) + extensionBridgeChunkBytes - 1) / extensionBridgeChunkBytes
	for index := 0; index < total; index++ {
		start := index * extensionBridgeChunkBytes
		end := start + extensionBridgeChunkBytes
		if end > len(envelope) {
			end = len(envelope)
		}
		require.NoError(t, connection.WriteJSON(ExtensionBridgeEnvelope{
			Type: "chunk", TransferID: "incoming-large", Index: index, Total: total, OriginalBytes: len(envelope),
			Data: base64.StdEncoding.EncodeToString(envelope[start:end]),
		}))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	event, err := server.WaitEvent(ctx)
	require.NoError(t, err)
	require.Equal(t, "large.event", event["method"])
}

func TestBuildCapturedWebFuzzerConfig(t *testing.T) {
	packet := []byte("POST /api/session HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{\"ok\":true}")
	params, err := json.Marshal(map[string]interface{}{
		"rawRequestBase64": base64.StdEncoding.EncodeToString(packet),
		"isHttps":          true,
		"tabName":          "Browser request",
	})
	require.NoError(t, err)
	config, tabName, bridgeErr := buildCapturedWebFuzzerConfig(params)
	require.Nil(t, bridgeErr)
	require.NotNil(t, config)
	require.Equal(t, "Browser request", tabName)
	require.Contains(t, config.Config, "POST /api/session HTTP/1.1")
	require.Contains(t, config.Config, "\\\"ok\\\":true")
}

func TestGenerateCapturedRequestYakPoC(t *testing.T) {
	packet := []byte("POST /api/session HTTP/1.1\r\nHost: api.example.test\r\nContent-Type: application/json\r\n\r\n{\"ok\":true}")
	params, err := json.Marshal(map[string]interface{}{
		"rawRequestBase64": base64.StdEncoding.EncodeToString(packet),
		"isHttps":          true,
	})
	require.NoError(t, err)
	result, bridgeErr := generateCapturedRequestYakPoC(params)
	require.Nil(t, bridgeErr)
	generated := result.(map[string]interface{})
	require.Equal(t, "yak", generated["language"])
	require.Equal(t, "api-example-test.yak", generated["fileName"])
	require.Contains(t, generated["code"], "poc.HTTP")
	require.Contains(t, generated["code"], "poc.https(true)")
	require.Contains(t, generated["code"], base64.StdEncoding.EncodeToString(packet))
}

func TestPrepareCapturedRequestAnalysisOmitsValues(t *testing.T) {
	packet := []byte("POST /api/accounts?objectId=42&nonce=query-secret HTTP/1.1\r\n" +
		"Host: api.example.test\r\nAuthorization: Bearer header-secret\r\nX-Signature: signature-secret\r\n" +
		"Cookie: session=cookie-secret\r\nContent-Type: application/json\r\n\r\n" +
		"{\"account\":{\"id\":42},\"csrfToken\":\"body-secret\"}")
	params, err := json.Marshal(map[string]interface{}{
		"rawRequestBase64": base64.StdEncoding.EncodeToString(packet),
		"isHttps":          true,
		"observations": []map[string]interface{}{{
			"kind": "webcrypto", "operation": "sign", "algorithm": "HMAC hash=SHA-256", "timestamp": 123,
		}},
	})
	require.NoError(t, err)
	result, bridgeErr := prepareCapturedRequestAnalysis(params)
	require.Nil(t, bridgeErr)
	analysis := result.(map[string]interface{})
	request := analysis["request"].(map[string]interface{})
	require.ElementsMatch(t, []string{"nonce", "objectId"}, request["queryKeys"])
	require.ElementsMatch(t, []string{"account", "account.id", "csrfToken"}, request["bodyKeys"])
	require.ElementsMatch(t, []string{"session"}, request["cookieNames"])
	require.NotEmpty(t, analysis["signals"])
	require.NotEmpty(t, analysis["observations"])
	serialized, err := json.Marshal(analysis)
	require.NoError(t, err)
	for _, secret := range []string{"query-secret", "header-secret", "signature-secret", "cookie-secret", "body-secret"} {
		require.NotContains(t, string(serialized), secret)
	}
}

type unexpectedBridgeMessageError struct {
	request ExtensionBridgeEnvelope
}

func (e *unexpectedBridgeMessageError) Error() string {
	raw, _ := json.Marshal(e.request)
	return "unexpected bridge message: " + string(raw)
}
