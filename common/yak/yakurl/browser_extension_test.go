//go:build !yakit_exclude

package yakurl

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBrowserExtensionYakURLSnapshotAndPairingWindow(t *testing.T) {
	manager, err := browser.NewExtensionBridgeManager(
		browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json")), nil,
	)
	require.NoError(t, err)
	require.NoError(t, manager.Start(0))
	browser.SetActiveExtensionBridgeManager(manager)
	t.Cleanup(func() {
		browser.SetActiveExtensionBridgeManager(nil)
		require.NoError(t, manager.Close())
	})

	action := newBrowserExtensionAction()
	response, err := action.Get(&ypb.RequestYakURLParams{
		Method: "GET", Url: &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
	})
	require.NoError(t, err)
	require.Len(t, response.Resources, 1)
	require.Equal(t, "status", response.Resources[0].ResourceType)
	var status map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response.Resources[0].Extra[0].Value), &status))
	require.Equal(t, true, status["running"])
	require.EqualValues(t, managedProtocolVersionForTest, status["protocolVersion"])
	require.NotEmpty(t, status["engineInstanceId"])

	response, err = action.Post(&ypb.RequestYakURLParams{
		Method: "POST", Url: &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/pairing-window"},
		Body: []byte(`{"ttlSeconds":120}`),
	})
	require.NoError(t, err)
	require.Len(t, response.Resources, 1)
	require.NoError(t, json.Unmarshal([]byte(response.Resources[0].Extra[0].Value), &status))
	require.NotZero(t, status["pairingOpenUntil"])
}

func TestBrowserExtensionPairedDeviceSnapshotMatchesV3(t *testing.T) {
	store := browser.NewExtensionBridgeFileIdentityStore(filepath.Join(t.TempDir(), "identity.json"))
	initial, err := browser.NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	_, publicKey := initial.EngineIdentity()

	state, err := store.Load()
	require.NoError(t, err)
	state.NextReference = 8
	state.Devices = []browser.ExtensionBridgeDevice{{
		ID: "device-1", Reference: 7, InstallationID: "installation-1",
		Name: "Browser A", Client: "extension", ClientVersion: "1",
		Origin: "chrome-extension://extension-a", PublicKey: publicKey,
		CreatedAt: 1000, LastSeenAt: 2000,
	}}
	require.NoError(t, store.Save(state))

	manager, err := browser.NewExtensionBridgeManager(store, nil)
	require.NoError(t, err)
	browser.SetActiveExtensionBridgeManager(manager)
	t.Cleanup(func() {
		browser.SetActiveExtensionBridgeManager(nil)
		require.NoError(t, manager.Close())
	})

	response, err := newBrowserExtensionAction().Get(&ypb.RequestYakURLParams{
		Method: "GET", Url: &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
	})
	require.NoError(t, err)
	require.Len(t, response.Resources, 2)
	require.Equal(t, "paired-device", response.Resources[1].ResourceType)
	require.Equal(t, "device-1", response.Resources[1].ResourceName)

	encodedKey, err := json.Marshal(publicKey)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"device-1", "installationId":"installation-1",
		"name":"Browser A", "client":"extension", "clientVersion":"1",
		"origin":"chrome-extension://extension-a", "publicKey":`+string(encodedKey)+`,
		"createdAt":1000, "lastSeenAt":2000
	}`, response.Resources[1].Extra[0].Value)

	reference, err := manager.BrowserReference("device-1")
	require.NoError(t, err)
	require.Equal(t, "br_7", reference)
	saved, err := store.Load()
	require.NoError(t, err)
	require.EqualValues(t, 7, saved.Devices[0].Reference)
}

const managedProtocolVersionForTest = 3

func TestBrowserExtensionYakURLUnavailableWithoutManager(t *testing.T) {
	browser.SetActiveExtensionBridgeManager(nil)
	_, err := newBrowserExtensionAction().Get(&ypb.RequestYakURLParams{
		Method: "GET", Url: &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
	})
	require.ErrorContains(t, err, "not available")
}
