//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_RequestYakURL_BrowserExtensionV3Snapshot(t *testing.T) {
	const origin = "chrome-extension://extension-a"
	extensionKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	publicKey := browser.ExtensionBridgeJWK{
		KTY: "EC", CRV: "P-256",
		X: base64.RawURLEncoding.EncodeToString(extensionKey.X.FillBytes(make([]byte, 32))),
		Y: base64.RawURLEncoding.EncodeToString(extensionKey.Y.FillBytes(make([]byte, 32))),
	}

	store := browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	_, err = browser.NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	// A pairing written before #5470 has neither a device reference nor a counter.
	state, err := store.Load()
	require.NoError(t, err)
	state.NextReference = 0
	state.Devices = []browser.ExtensionBridgeDevice{{
		ID: "device-1", InstallationID: "installation-1",
		Name: "Browser A", Client: "extension", ClientVersion: "1",
		Origin: origin, PublicKey: publicKey, CreatedAt: 1000, LastSeenAt: 2000,
	}}
	require.NoError(t, store.Save(state))

	manager, err := browser.NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(0))
	previous := browser.ActiveExtensionBridgeManager()
	browser.SetActiveExtensionBridgeManager(manager)
	t.Cleanup(func() {
		browser.SetActiveExtensionBridgeManager(previous)
		require.NoError(t, manager.Close())
	})

	client, err := newInMemoryClient(&Server{browserBridge: manager})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	readSnapshot := func() map[string]string {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// This is the RPC and URL used by v3 desktop clients.
		response, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
			Method: http.MethodGet,
			Url:    &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
		})
		require.NoError(t, err)
		data := make(map[string]string)
		for _, resource := range response.GetResources() {
			require.NotContains(t, data, resource.GetResourceType())
			data[resource.GetResourceType()] = ExtraKVPairsToMap(resource.GetExtra())["data"]
		}
		return data
	}

	snapshot := manager.Snapshot()
	expectedStatus := map[string]any{
		"revision": snapshot.Revision, "running": true, "connected": false, "url": snapshot.URL,
		"protocolVersion": 3, "engineIdentityId": snapshot.EngineIdentityID,
		"engineInstanceId": snapshot.EngineInstanceID, "connections": []any{},
	}
	expectedDevice := map[string]any{
		"id": "device-1", "installationId": "installation-1", "name": "Browser A",
		"client": "extension", "clientVersion": "1", "origin": origin,
		"publicKey": map[string]string{"kty": "EC", "crv": "P-256", "x": publicKey.X, "y": publicKey.Y},
		"createdAt": 1000, "lastSeenAt": 2000,
	}
	offline := readSnapshot()
	require.Len(t, offline, 2)
	assertBrowserV3SnapshotJSON(t, expectedStatus, offline["status"])
	assertBrowserV3SnapshotJSON(t, expectedDevice, offline["paired-device"])

	dial := func(endpoint string) *websocket.Conn {
		conn, _, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": {origin}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		return conn
	}
	connection := dial(snapshot.URL)
	var challenge browser.ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&challenge))
	require.Equal(t, "challenge", challenge.Type)
	require.Equal(t, 3, challenge.ProtocolVersion)

	catalog := map[string]any{
		"version": 1, "schemaDialect": "http://json-schema.org/draft-07/schema#",
		"capabilities": []map[string]any{{
			"method": "browser.tabs", "domain": "page", "access": "read", "summary": "List tabs",
			"agentVisible": true, "scopes": []string{"browser.tabs.read"}, "targetMode": "document",
			"conditionalScopes": []map[string]string{{"scope": "browser.tabs.read", "when": "tab selected"}},
			"defaultTimeoutMs":  1000, "paramsSchema": map[string]any{"type": "object"},
		}},
	}
	catalogJSON, err := json.Marshal(catalog)
	require.NoError(t, err)
	catalogHash := fmt.Sprintf("%x", sha256.Sum256(catalogJSON))
	catalog["hash"] = catalogHash
	managed := map[string]string{"manager": "ytray", "instanceId": "instance-a", "badge": "A"}
	// Sign the v3 transcript independently of the engine's internal model.
	transcript := strings.Join([]string{
		"yak-browser-bridge-v3", "client-auth", origin, challenge.EngineIdentityID,
		challenge.EngineInstanceID, challenge.Challenge, "installation-1", "extension", "",
		"browser.tabs", "1", catalogHash, "task-1", "grant-1", "", "ytray", "instance-a", "A",
	}, "\n")
	digest := sha256.Sum256([]byte(transcript))
	r, s, err := ecdsa.Sign(rand.Reader, extensionKey, digest[:])
	require.NoError(t, err)
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	require.NoError(t, connection.WriteJSON(map[string]any{
		"type": "auth", "protocolVersion": 3, "challenge": challenge.Challenge,
		"installationId": "installation-1", "client": "extension",
		"managedInstance": managed, "capabilities": []string{"browser.tabs"}, "capabilityCatalog": catalog,
		"taskId": "task-1", "grantId": "grant-1", "signature": base64.RawURLEncoding.EncodeToString(signature),
	}))
	var ack browser.ExtensionBridgeEnvelope
	require.NoError(t, connection.ReadJSON(&ack))
	require.Equal(t, "hello_ack", ack.Type)
	require.Eventually(t, func() bool {
		return manager.Snapshot().Devices[0].LastSeenAt > 2000
	}, 3*time.Second, 10*time.Millisecond)

	pairing := dial(strings.TrimSuffix(snapshot.URL, "/extension") + "/pairing")
	require.NoError(t, pairing.WriteJSON(map[string]any{
		"type": "pair_request", "protocolVersion": 3, "installationId": "installation-2",
		"client": "extension", "publicKey": expectedDevice["publicKey"],
		"nonce": base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "managedInstance": managed,
	}))
	var pending struct {
		Type      string `json:"type"`
		RequestID string `json:"requestId"`
		Code      string `json:"code"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	require.NoError(t, pairing.ReadJSON(&pending))
	require.Equal(t, "pair_pending", pending.Type)

	snapshot = manager.Snapshot()
	require.Len(t, snapshot.Connections, 1)
	require.Len(t, snapshot.Pending, 1)
	require.Equal(t, "br_1", snapshot.Connections[0].Reference)
	require.NotNil(t, snapshot.Connections[0].CapabilityCatalog)
	require.Equal(t, catalogHash, snapshot.Connections[0].CapabilityCatalog.Hash)
	expectedStatus["revision"] = snapshot.Revision
	expectedStatus["connected"] = true
	expectedStatus["connections"] = []map[string]any{{
		"deviceId": "device-1", "installationId": "installation-1", "managedInstance": managed,
		"client": "extension", "clientVersion": "unknown", "capabilities": []string{"browser.tabs"},
		"sessionId": ack.SessionID, "connectionId": ack.ConnectionID,
		"taskId": "task-1", "grantId": "grant-1", "connectedAt": snapshot.Connections[0].ConnectedAt,
	}}
	expectedDevice["lastSeenAt"] = snapshot.Devices[0].LastSeenAt
	expectedDevice["clientVersion"] = "unknown"

	// Frozen v3 fields are checked for every resource, including nested online
	// connections, because shipped clients reject additional fields.
	online := readSnapshot()
	require.Len(t, online, 3)
	assertBrowserV3SnapshotJSON(t, expectedStatus, online["status"])
	assertBrowserV3SnapshotJSON(t, expectedDevice, online["paired-device"])
	assertBrowserV3SnapshotJSON(t, map[string]any{
		"id": pending.RequestID, "installationId": "installation-2", "managedInstance": managed,
		"extensionId": "extension-a", "client": "extension", "clientVersion": "unknown", "origin": origin,
		"code": pending.Code, "createdAt": snapshot.Pending[0].CreatedAt, "expiresAt": pending.ExpiresAt,
	}, online["pairing-request"])

	saved, err := store.Load()
	require.NoError(t, err)
	require.Len(t, saved.Devices, 1)
	require.EqualValues(t, 1, saved.Devices[0].Reference)
}

func assertBrowserV3SnapshotJSON(t *testing.T, expected any, received string) {
	t.Helper()
	encoded, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), received)
}
