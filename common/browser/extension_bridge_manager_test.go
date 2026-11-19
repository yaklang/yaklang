package browser

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/minimartian"
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

func TestManagedClientAuthPayloadBindsBrowserInstanceIdentity(t *testing.T) {
	hello := ExtensionBridgeEnvelope{
		Type: "auth", InstallationID: "install-1", Client: "client-1", Version: "1.0.0",
		ManagedInstance: &ExtensionBridgeManagedInstance{
			Manager: "ytray", InstanceID: "instance-1", Badge: "B",
		},
	}
	require.Equal(
		t,
		"yak-browser-bridge-v3\nclient-auth\nchrome-extension://abc\nidentity-1\nengine-1\nnonce-1\ninstall-1\nclient-1\n1.0.0\n\n\n\n\n\n\nytray\ninstance-1\nB",
		managedClientAuthPayload("chrome-extension://abc", "identity-1", "engine-1", "nonce-1", hello),
	)
}

func TestManagedExtensionBridgePairsAuthenticatesAndRevokesDevice(t *testing.T) {
	t.Run("websocket", func(t *testing.T) { testManagedExtensionBridge(t, false) })
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Run("unix", func(t *testing.T) { testManagedExtensionBridge(t, true) })
	}
}

func testManagedExtensionBridge(t *testing.T, unix bool) {
	t.Helper()
	store := NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	manager, err := NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	if unix {
		require.NoError(t, manager.StartNative())
	} else {
		require.NoError(t, manager.Start(0))
	}
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
	if unix {
		origin = extensionBridgePipeOrigin
		require.True(t, strings.HasPrefix(snapshot.URL, "unix:"))
		socket := strings.TrimPrefix(snapshot.URL, "unix:")
		discovery, err := net.DialTimeout("unix", socket, time.Second)
		require.NoError(t, err)
		defer discovery.Close()
		require.NoError(t, discovery.SetDeadline(time.Now().Add(time.Second)))
		require.NoError(t, WriteNativeMessagingMessage(discovery, map[string]string{
			"type": "discover", "engineInstanceId": snapshot.EngineInstanceID,
			"engineIdentityId": snapshot.EngineIdentityID,
		}))
		reply, err := ReadNativeMessagingMessage(discovery)
		require.NoError(t, err)
		var descriptor extensionBridgeNativeDescriptor
		require.NoError(t, json.Unmarshal(reply, &descriptor))
		require.Equal(t, snapshot.EngineIdentityID, descriptor.EngineIdentityID)
		require.Equal(t, snapshot.EngineInstanceID, descriptor.EngineInstanceID)
		require.Equal(t, filepath.Base(socket), descriptor.SocketName)
		require.Equal(t, os.Getpid(), descriptor.PID)
	}
	dial := func(purpose string) extensionBridgeWire {
		t.Helper()
		if unix {
			conn, err := net.DialTimeout("unix", strings.TrimPrefix(snapshot.URL, "unix:"), time.Second)
			require.NoError(t, err)
			wire := &extensionBridgeNativeWire{Conn: conn, readLimit: MaxNativeMessagingMessageSize}
			t.Cleanup(func() { _ = wire.Close() })
			require.NoError(t, wire.WriteJSON(map[string]string{
				"type": "open", "purpose": purpose, "origin": origin,
				"engineInstanceId": snapshot.EngineInstanceID, "engineIdentityId": snapshot.EngineIdentityID,
			}))
			return wire
		}
		url := snapshot.URL
		if purpose == "pair" {
			url = strings.Replace(url, extensionBridgePath, extensionBridgePairingPath, 1)
		}
		conn, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{origin}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	pairingConnection := dial("pair")

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	clientNonce := make([]byte, 32)
	_, err = rand.Read(clientNonce)
	require.NoError(t, err)
	require.NoError(t, pairingConnection.WriteJSON(extensionBridgePairingEnvelope{
		Type: "pair_request", ProtocolVersion: managedExtensionBridgeProtocolVersion,
		InstallationID: "managed-installation", Client: "managed-test", Version: "1.0.0",
		ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"},
		Nonce:           base64.RawURLEncoding.EncodeToString(clientNonce), PublicKey: ptrExtensionBridgeJWK(publicKeyToJWK(&clientKey.PublicKey)),
	}))
	var pending extensionBridgePairingEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(pairingConnection, &pending))
	require.Equal(t, "pair_pending", pending.Type)
	require.Regexp(t, `^\d{6}$`, pending.Code)
	require.NotEmpty(t, pending.RequestID)
	require.NotNil(t, pending.PublicKey)
	require.Equal(t, "A", manager.Snapshot().Pending[0].ManagedInstance.Badge)

	device, err := manager.ApprovePairing(pending.RequestID, "Test Chrome", "")
	require.NoError(t, err)
	require.Equal(t, "managed-installation", device.InstallationID)
	require.EqualValues(t, 1, device.Reference)
	require.EqualValues(t, 1, manager.Snapshot().Devices[0].Reference)
	reference, err := manager.BrowserReference(device.ID)
	require.NoError(t, err)
	require.Equal(t, "br_1", reference)
	var approved extensionBridgePairingEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(pairingConnection, &approved))
	require.Equal(t, "pair_approved", approved.Type)
	require.Equal(t, device.ID, approved.DeviceID)

	bridgeConnection := dial("connect")
	var challenge ExtensionBridgeEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(bridgeConnection, &challenge))
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
		ManagedInstance: &ExtensionBridgeManagedInstance{
			Manager: "ytray", InstanceID: "instance-a", Badge: "A",
		},
		Capabilities:      []string{"browser.context", "browser.tabs"},
		CapabilityCatalog: testExtensionBridgeCapabilityCatalog(t, "browser.context", "browser.tabs"),
	}
	auth.Signature = signManagedTestPayload(t, clientKey, managedClientAuthPayload(
		origin, challenge.EngineIdentityID, challenge.EngineInstanceID, challenge.Challenge, auth,
	))
	require.NoError(t, bridgeConnection.WriteJSON(auth))
	var acknowledged ExtensionBridgeEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(bridgeConnection, &acknowledged))
	require.Equal(t, "hello_ack", acknowledged.Type)
	require.Equal(t, managedExtensionBridgeProtocolVersion, acknowledged.ProtocolVersion)
	require.Equal(t, snapshot.EngineIdentityID, acknowledged.EngineIdentityID)
	require.Eventually(t, func() bool { return manager.Snapshot().Connected }, time.Second, 10*time.Millisecond)
	require.Equal(t, managedExtensionBridgeProtocolVersion, manager.Snapshot().ProtocolVersion)
	require.Len(t, manager.Snapshot().Connections, 1)
	require.Equal(t, device.ID, manager.Snapshot().Connections[0].DeviceID)
	require.Equal(t, "br_1", manager.Snapshot().Connections[0].Reference)
	require.Equal(t, auth.ManagedInstance, manager.Snapshot().Connections[0].ManagedInstance)
	require.Equal(t, auth.CapabilityCatalog.Hash, manager.Snapshot().Connections[0].CapabilityCatalog.Hash)

	stopIngress := manager.RegisterMITMIngress(context.Background(), "127.0.0.1:8083", false, make(chan *minimartian.WrapperedConn, 1))
	t.Cleanup(stopIngress)
	require.NoError(t, bridgeConnection.WriteJSON(ExtensionBridgeEnvelope{
		Type: "request", ID: "ingress-request", Method: "yakit.mitm.ingress",
		Params: json.RawMessage(`{"proxyHost":"127.0.0.1","proxyPort":8083}`),
	}))
	var ingressReply ExtensionBridgeEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(bridgeConnection, &ingressReply))
	require.Nil(t, ingressReply.Error)
	var ingress extensionMITMIngress
	require.NoError(t, json.Unmarshal(ingressReply.Result, &ingress))
	require.True(t, ingress.Available)
	require.Positive(t, ingress.Port)
	require.Equal(t, managedExtensionBridgeProtocolVersion, manager.currentServer().Status()["protocolVersion"])
	responseDone := make(chan error, 1)
	go func() {
		var request ExtensionBridgeEnvelope
		if readErr := readStrictExtensionBridgeJSON(bridgeConnection, &request); readErr != nil {
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
		if readErr := readStrictExtensionBridgeJSON(bridgeConnection, &request); readErr != nil {
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
	reclaimed, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", ingress.Port))
	require.NoError(t, err, "revoking a paired device must reclaim its ingress immediately")
	_ = reclaimed.Close()
	require.Eventually(t, func() bool { return !manager.Snapshot().Connected }, time.Second, 10*time.Millisecond)
	_ = bridgeConnection.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = bridgeConnection.ReadMessage()
	require.Error(t, err)
	revokedConnection := dial("connect")
	require.NoError(t, readStrictExtensionBridgeJSON(revokedConnection, &challenge))
	auth.Challenge = challenge.Challenge
	auth.Signature = signManagedTestPayload(t, clientKey, managedClientAuthPayload(
		origin, challenge.EngineIdentityID, challenge.EngineInstanceID, challenge.Challenge, auth,
	))
	require.NoError(t, revokedConnection.WriteJSON(auth))
	var rejection ExtensionBridgeEnvelope
	require.NoError(t, readStrictExtensionBridgeJSON(revokedConnection, &rejection))
	require.NotNil(t, rejection.Error)
	require.Equal(t, "unauthorized", rejection.Error.Code)
}

func TestManagedDeviceRuntimeReferencesPersistAndBackfill(t *testing.T) {
	store := NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	_, err := NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)

	state, err := store.Load()
	require.NoError(t, err)
	state.Devices = []ExtensionBridgeDevice{
		{ID: "browser-newer", CreatedAt: 20, Reference: 7},
		{ID: "browser-older", CreatedAt: 10},
	}
	state.NextReference = 0
	require.NoError(t, store.Save(state))

	reloaded, err := NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	snapshot := reloaded.Snapshot()
	require.Len(t, snapshot.Devices, 2)
	references := map[string]string{}
	for _, device := range snapshot.Devices {
		references[device.ID] = formatExtensionBridgeReference(device.Reference)
	}
	require.Equal(t, "br_8", references["browser-older"])
	require.Equal(t, "br_7", references["browser-newer"])

	persisted, err := store.Load()
	require.NoError(t, err)
	require.EqualValues(t, 8, persisted.Devices[1].Reference)
	require.EqualValues(t, 9, persisted.NextReference)
}

func TestManagedExtensionBridgeDoesNotReuseRevokedReference(t *testing.T) {
	manager, err := NewExtensionBridgeManager(
		NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	origin := "chrome-extension://managed-test-extension"

	firstPending, err := manager.beginPairing(origin, managedTestPairingInput(t, "first-installation"))
	require.NoError(t, err)
	first, err := manager.ApprovePairing(firstPending.request.ID, "First Browser", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Reference)
	require.NoError(t, manager.RevokeDevice(first.ID))

	secondPending, err := manager.beginPairing(origin, managedTestPairingInput(t, "second-installation"))
	require.NoError(t, err)
	second, err := manager.ApprovePairing(secondPending.request.ID, "Second Browser", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, second.Reference)
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
	require.Equal(t, first.Reference, second.Reference)
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
	require.Equal(t, second.Reference, replaced.Reference)
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

func TestManagedExtensionBridgeRefreshesBrowserProductOnConnect(t *testing.T) {
	manager, err := NewExtensionBridgeManager(
		NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	pending, err := manager.beginPairing(
		"chrome-extension://managed-test-extension",
		managedTestPairingInput(t, "browser-product-installation"),
	)
	require.NoError(t, err)
	device, err := manager.ApprovePairing(pending.request.ID, "Browser", "")
	require.NoError(t, err)

	manager.markDeviceSeen(device.ID, "Microsoft Edge", "152.0.1234.5")
	refreshed := manager.Snapshot().Devices[0]
	require.Equal(t, "Microsoft Edge", refreshed.Client)
	require.Equal(t, "152.0.1234.5", refreshed.ClientVersion)
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

func TestExtensionBridgeFileIdentityStoreReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	store := NewExtensionBridgeFileIdentityStore(path)
	first := &extensionBridgeIdentityState{Version: 1, EngineIdentityID: "first"}
	second := &extensionBridgeIdentityState{Version: 1, EngineIdentityID: "second"}

	require.NoError(t, store.Save(first))
	require.NoError(t, store.Save(second))
	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, second, loaded)
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".identity.json.tmp-*"))
	require.NoError(t, err)
	require.Empty(t, matches)
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

func TestPairingVerificationCodeBindsManagedInstance(t *testing.T) {
	input := extensionBridgePairingInput{
		InstallationID: "install-1", Nonce: "client-nonce-value",
		PublicKey:       ExtensionBridgeJWK{KTY: "EC", CRV: "P-256", X: "x-coordinate", Y: "y-coordinate"},
		ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-1", Badge: "B"},
	}
	require.Equal(t, "005427", pairingVerificationCode(
		"engine-id", "request-1", "chrome-extension://abc", input, "server-nonce-value",
	))
}
