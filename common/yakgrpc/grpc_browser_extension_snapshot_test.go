//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_RequestYakURL_BrowserExtensionV3Snapshot(t *testing.T) {
	store := browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	legacy, err := browser.NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	_, publicKey := legacy.EngineIdentity()

	// A pairing written before #5470 has neither a device reference nor a counter.
	state, err := store.Load()
	require.NoError(t, err)
	state.NextReference = 0
	state.Devices = []browser.ExtensionBridgeDevice{{
		ID: "device-1", InstallationID: "installation-1",
		Name: "Browser A", Client: "extension", ClientVersion: "1",
		Origin: "chrome-extension://extension-a", PublicKey: publicKey,
		CreatedAt: 1000, LastSeenAt: 2000,
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use the same RPC and URL as the installed v3 desktop clients.
	response, err := client.RequestYakURL(ctx, &ypb.RequestYakURLParams{
		Method: http.MethodGet,
		Url:    &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
	})
	require.NoError(t, err)
	resources := response.GetResources()
	require.Len(t, resources, 2)
	require.Equal(t, "status", resources[0].GetResourceType())
	var status struct {
		Running         bool `json:"running"`
		ProtocolVersion int  `json:"protocolVersion"`
	}
	require.NoError(t, json.Unmarshal([]byte(ExtraKVPairsToMap(resources[0].GetExtra())["data"]), &status))
	require.True(t, status.Running)
	require.Equal(t, 3, status.ProtocolVersion)

	require.Equal(t, "paired-device", resources[1].GetResourceType())
	require.Equal(t, "device-1", resources[1].GetResourceName())
	encodedKey, err := json.Marshal(publicKey)
	require.NoError(t, err)
	// Older strict decoders reject any additional field, including reference.
	require.JSONEq(t, `{
		"id":"device-1", "installationId":"installation-1",
		"name":"Browser A", "client":"extension", "clientVersion":"1",
		"origin":"chrome-extension://extension-a", "publicKey":`+string(encodedKey)+`,
		"createdAt":1000, "lastSeenAt":2000
	}`, ExtraKVPairsToMap(resources[1].GetExtra())["data"])

	reference, err := manager.BrowserReference("device-1")
	require.NoError(t, err)
	require.Equal(t, "br_1", reference)
	saved, err := store.Load()
	require.NoError(t, err)
	require.EqualValues(t, 1, saved.Devices[0].Reference)
}
