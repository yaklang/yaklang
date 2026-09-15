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

const managedProtocolVersionForTest = 3

func TestBrowserExtensionYakURLUnavailableWithoutManager(t *testing.T) {
	browser.SetActiveExtensionBridgeManager(nil)
	_, err := newBrowserExtensionAction().Get(&ypb.RequestYakURLParams{
		Method: "GET", Url: &ypb.YakURL{Schema: "browser-extension", Location: "local", Path: "/snapshot"},
	})
	require.ErrorContains(t, err, "not available")
}
