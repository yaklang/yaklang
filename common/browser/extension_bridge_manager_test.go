package browser

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func signManagedTestPayload(t *testing.T, key *ecdsa.PrivateKey, payload string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(payload))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	require.NoError(t, err)
	return encodeECDSASignature(r, s)
}

func TestManagedClientAuthPayloadMatchesBrowserTranscript(t *testing.T) {
	hello := ExtensionBridgeEnvelope{
		Type: "auth", InstallationID: "install-1", Client: "client-1", Version: "1.0.0",
		Capabilities: []string{"z.capability", "a.capability"},
		CapabilityCatalog: &ExtensionBridgeCapabilityCatalog{
			Version: 1,
			Hash:    "schema-hash-1",
		},
		TaskID: "task-1", GrantID: "grant-1", ResumeSessionID: "session-1",
	}
	require.Equal(
		t,
		"yak-browser-bridge-v3\nclient-auth\nchrome-extension://abc\nidentity-1\ninstance-1\nnonce-1\ninstall-1\nclient-1\n1.0.0\na.capability,z.capability\n1\nschema-hash-1\ntask-1\ngrant-1\nsession-1",
		managedClientAuthPayload(
			"chrome-extension://abc",
			"identity-1",
			"instance-1",
			"nonce-1",
			hello,
		),
	)
}

func TestManagedExtensionBridgePairsAuthenticatesAndRevokesDevice(t *testing.T) {
	store := NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	manager, err := NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(0))
	SetActiveExtensionBridgeManager(manager)
	t.Cleanup(func() {
		SetActiveExtensionBridgeManager(nil)
		require.NoError(t, manager.Close())
	})

	snapshot := manager.Snapshot()
	require.True(t, snapshot.Running)
	require.NotEmpty(t, snapshot.EngineIdentityID)
	require.NotEmpty(t, snapshot.EngineInstanceID)
	require.Empty(t, snapshot.Devices)

	origin := "chrome-extension://managed-test-extension"
	header := http.Header{"Origin": []string{origin}}
	pairingURL := strings.Replace(snapshot.URL, extensionBridgePath, extensionBridgePairingPath, 1)
	pairingConnection, _, err := websocket.DefaultDialer.Dial(pairingURL, header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pairingConnection.Close() })

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	clientNonce := make([]byte, 32)
	_, err = rand.Read(clientNonce)
	require.NoError(t, err)
	require.NoError(t, pairingConnection.WriteJSON(extensionBridgePairingEnvelope{
		Type: "pair_request", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		InstallationID: "managed-installation", Client: "managed-test", Version: "1.0.0",
		Nonce: base64.RawURLEncoding.EncodeToString(clientNonce), PublicKey: ptrExtensionBridgeJWK(publicKeyToJWK(&clientKey.PublicKey)),
	}))
	var pending extensionBridgePairingEnvelope
	require.NoError(t, pairingConnection.ReadJSON(&pending))
	require.Equal(t, "pair_pending", pending.Type)
	require.Regexp(t, `^\d{6}$`, pending.Code)
	require.NotEmpty(t, pending.RequestID)
	require.NotNil(t, pending.PublicKey)

	device, err := manager.ApprovePairing(pending.RequestID, "Test Chrome", "")
	require.NoError(t, err)
	require.Equal(t, "managed-installation", device.InstallationID)
	var approved extensionBridgePairingEnvelope
	require.NoError(t, pairingConnection.ReadJSON(&approved))
	require.Equal(t, "pair_approved", approved.Type)
	require.Equal(t, device.ID, approved.DeviceID)

	bridgeConnection, _, err := websocket.DefaultDialer.Dial(snapshot.URL, header)
	require.NoError(t, err)
	var challenge ExtensionBridgeEnvelope
	require.NoError(t, bridgeConnection.ReadJSON(&challenge))
	require.Equal(t, "challenge", challenge.Type)
	require.Equal(t, managedExtensionBridgeProtocolVersion, challenge.ProtocolVersion)
	require.Equal(t, snapshot.EngineInstanceID, challenge.EngineInstanceID)
	engineKey, err := parseExtensionBridgePublicKey(*challenge.PublicKey)
	require.NoError(t, err)
	require.True(t, verifyECDSASignature(engineKey, managedEngineChallengePayload(
		challenge.EngineIdentityID, challenge.EngineInstanceID, challenge.Challenge, challenge.Timestamp,
	), challenge.Signature))

	auth := ExtensionBridgeEnvelope{
		Type: "auth", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		Challenge: challenge.Challenge, InstallationID: "managed-installation",
		Client: "managed-test", Version: "1.0.0",
		Capabilities:      []string{"browser.context", "browser.tabs"},
		CapabilityCatalog: testExtensionBridgeCapabilityCatalog(t, "browser.context", "browser.tabs"),
	}
	auth.Signature = signManagedTestPayload(t, clientKey, managedClientAuthPayload(
		origin, challenge.EngineIdentityID, challenge.EngineInstanceID, challenge.Challenge, auth,
	))
	require.NoError(t, bridgeConnection.WriteJSON(auth))
	var acknowledged ExtensionBridgeEnvelope
	require.NoError(t, bridgeConnection.ReadJSON(&acknowledged))
	require.Equal(t, "hello_ack", acknowledged.Type)
	require.Equal(t, managedExtensionBridgeProtocolVersion, acknowledged.ProtocolVersion)
	require.Equal(t, snapshot.EngineIdentityID, acknowledged.EngineIdentityID)
	require.Eventually(t, func() bool { return manager.Snapshot().Connected }, time.Second, 10*time.Millisecond)
	require.Equal(t, managedExtensionBridgeProtocolVersion, manager.Snapshot().ProtocolVersion)
	require.Len(t, manager.Snapshot().Connections, 1)
	require.Equal(t, device.ID, manager.Snapshot().Connections[0].DeviceID)
	require.Equal(t, auth.CapabilityCatalog.Hash, manager.Snapshot().Connections[0].CapabilityCatalog.Hash)
	require.Equal(t, managedExtensionBridgeProtocolVersion, manager.currentServer().Status()["protocolVersion"])
	responseDone := make(chan error, 1)
	go func() {
		var request ExtensionBridgeEnvelope
		if readErr := bridgeConnection.ReadJSON(&request); readErr != nil {
			responseDone <- readErr
			return
		}
		result, _ := json.Marshal(map[string]bool{"ok": true})
		responseDone <- bridgeConnection.WriteJSON(ExtensionBridgeEnvelope{ID: request.ID, Type: "response", Result: result})
	}()
	result, err := CallExtensionBridge("browser.context", map[string]interface{}{}, 1)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"ok": true}, result)
	require.NoError(t, <-responseDone)
	_, err = manager.CallDevice(context.Background(), device.ID, "browser.missing", map[string]interface{}{})
	require.ErrorContains(t, err, "not declared")
	_, err = manager.CallDevice(
		context.Background(),
		device.ID,
		"browser.context",
		map[string]interface{}{"includeDom": true},
	)
	require.ErrorContains(t, err, "do not match schema")
	responseDone = make(chan error, 1)
	go func() {
		var request ExtensionBridgeEnvelope
		if readErr := bridgeConnection.ReadJSON(&request); readErr != nil {
			responseDone <- readErr
			return
		}
		result, _ := json.Marshal(map[string]string{"route": device.ID})
		responseDone <- bridgeConnection.WriteJSON(ExtensionBridgeEnvelope{ID: request.ID, Type: "response", Result: result})
	}()
	raw, err := manager.CallDevice(context.Background(), device.ID, "browser.tabs", map[string]interface{}{})
	require.NoError(t, err)
	require.JSONEq(t, `{"route":"`+device.ID+`"}`, string(raw))
	require.NoError(t, <-responseDone)
	require.NoError(t, manager.RevokeDevice(device.ID))
	require.Eventually(t, func() bool { return !manager.Snapshot().Connected }, time.Second, 10*time.Millisecond)
	_ = bridgeConnection.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = bridgeConnection.ReadMessage()
	require.Error(t, err)
	revokedConnection, _, err := websocket.DefaultDialer.Dial(snapshot.URL, header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = revokedConnection.Close() })
	require.NoError(t, revokedConnection.ReadJSON(&challenge))
	auth.Challenge = challenge.Challenge
	auth.Signature = signManagedTestPayload(t, clientKey, managedClientAuthPayload(
		origin, challenge.EngineIdentityID, challenge.EngineInstanceID, challenge.Challenge, auth,
	))
	require.NoError(t, revokedConnection.WriteJSON(auth))
	var rejection ExtensionBridgeEnvelope
	require.NoError(t, revokedConnection.ReadJSON(&rejection))
	require.NotNil(t, rejection.Error)
	require.Equal(t, "unauthorized", rejection.Error.Code)
}

func ptrExtensionBridgeJWK(value ExtensionBridgeJWK) *ExtensionBridgeJWK {
	return &value
}

func managedTestPairingInput(t *testing.T, installationID string) extensionBridgePairingInput {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	nonce := make([]byte, 32)
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	return extensionBridgePairingInput{
		InstallationID: installationID,
		Client:         "managed-test",
		ClientVersion:  "1.0.0",
		Nonce:          base64.RawURLEncoding.EncodeToString(nonce),
		PublicKey:      publicKeyToJWK(&key.PublicKey),
	}
}

func TestManagedExtensionBridgeRePairingReusesTrustedDevice(t *testing.T) {
	manager, err := NewExtensionBridgeManager(
		NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	origin := "chrome-extension://managed-test-extension"

	firstPending, err := manager.beginPairing(origin, managedTestPairingInput(t, "stable-installation"))
	require.NoError(t, err)
	first, err := manager.ApprovePairing(firstPending.request.ID, "Named Browser", "")
	require.NoError(t, err)

	rotatedInput := managedTestPairingInput(t, "stable-installation")
	secondPending, err := manager.beginPairing(origin, rotatedInput)
	require.NoError(t, err)
	second, err := manager.ApprovePairing(secondPending.request.ID, "Chrome Browser", "")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.CreatedAt, second.CreatedAt)
	require.Equal(t, "Named Browser", second.Name)
	require.Equal(t, rotatedInput.PublicKey, second.PublicKey)
	require.Len(t, manager.Snapshot().Devices, 1)

	resetInput := managedTestPairingInput(t, "reset-installation")
	resetPending, err := manager.beginPairing(origin, resetInput)
	require.NoError(t, err)
	replaced, err := manager.ApprovePairing(resetPending.request.ID, "Chrome Browser", second.ID)
	require.NoError(t, err)
	require.Equal(t, second.ID, replaced.ID)
	require.Equal(t, "reset-installation", replaced.InstallationID)
	require.Equal(t, "Named Browser", replaced.Name)
	require.Len(t, manager.Snapshot().Devices, 1)

	foreignPending, err := manager.beginPairing(
		"chrome-extension://another-extension",
		managedTestPairingInput(t, "foreign-installation"),
	)
	require.NoError(t, err)
	_, err = manager.ApprovePairing(foreignPending.request.ID, "Foreign Browser", replaced.ID)
	require.ErrorContains(t, err, "does not match")
	require.Len(t, manager.Snapshot().Devices, 1)
}

func TestManagedExtensionBridgeIdentityPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	first, err := NewExtensionBridgeManager(NewExtensionBridgeFileIdentityStore(path), nil)
	require.NoError(t, err)
	firstID, firstKey := first.EngineIdentity()

	second, err := NewExtensionBridgeManager(NewExtensionBridgeFileIdentityStore(path), nil)
	require.NoError(t, err)
	secondID, secondKey := second.EngineIdentity()
	require.Equal(t, firstID, secondID)
	require.Equal(t, firstKey, secondKey)
}

func TestPairingVerificationCodeSharedVector(t *testing.T) {
	require.Equal(t, "113961", pairingVerificationCode(
		"engine-id", "request-1", "chrome-extension://abc",
		extensionBridgePairingInput{
			InstallationID: "install-1", Nonce: "client-nonce-value",
			PublicKey: ExtensionBridgeJWK{KTY: "EC", CRV: "P-256", X: "x-coordinate", Y: "y-coordinate"},
		},
		"server-nonce-value",
	))
}
