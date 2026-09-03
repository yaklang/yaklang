package browser

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionAuthorizationClientTaskRequiresManagedBridge(t *testing.T) {
	server := &ExtensionBridgeServer{}
	result, bridgeErr := server.handleExtensionAuthorizationClientTask(
		context.Background(),
		"device-a",
		json.RawMessage(`{"schema":"authorization.workspace.inspect","payload":{"workspaceId":"workspace-a"}}`),
	)
	require.Nil(t, result)
	require.NotNil(t, bridgeErr)
	require.Equal(t, "bridge_unavailable", bridgeErr.Code)
}

func TestExtensionAuthorizationClientTaskRejectsUnknownEnvelopeFields(t *testing.T) {
	server := &ExtensionBridgeServer{manager: &ExtensionBridgeManager{}}
	result, bridgeErr := server.handleExtensionAuthorizationClientTask(
		context.Background(),
		"device-a",
		json.RawMessage(`{"schema":"authorization.workspace.inspect","payload":{"workspaceId":"workspace-a"},"deviceId":"forged-device"}`),
	)
	require.Nil(t, result)
	require.NotNil(t, bridgeErr)
	require.Equal(t, "invalid_params", bridgeErr.Code)
	require.Contains(t, bridgeErr.Message, "unknown field")
}

func TestExtensionAuthorizationClientWorkspaceIgnoresCallerDeviceSelection(t *testing.T) {
	var input extensionAuthorizationClientWorkspaceInput
	err := decodeExtensionAuthorizationClientJSON(
		json.RawMessage(`{"mode":"horizontal","left":{"tabId":11,"frameId":0,"accountLabel":"A"},"right":{"tabId":12,"frameId":0,"accountLabel":"B"}}`),
		&input,
	)
	require.NoError(t, err)
	require.Equal(t, 11, input.Left.TabID)

	err = decodeExtensionAuthorizationClientJSON(
		json.RawMessage(`{"mode":"horizontal","left":{"deviceId":"other","tabId":11,"frameId":0},"right":{"tabId":12,"frameId":0}}`),
		&input,
	)
	require.ErrorContains(t, err, "unknown field")
}

func TestDecodeExtensionAuthorizationYakitOpen(t *testing.T) {
	prepared, err := decodeExtensionAuthorizationYakitOpen(
		json.RawMessage(`{"tabId":11,"mode":"vertical","targetDeviceId":"device-b"}`),
	)
	require.NoError(t, err)
	require.Equal(t, 11, prepared.TabID)
	require.Equal(t, "vertical", prepared.Mode)
	require.Equal(t, "device-b", prepared.TargetDeviceID)

	existing, err := decodeExtensionAuthorizationYakitOpen(
		json.RawMessage(`{"workspaceId":"workspace-a"}`),
	)
	require.NoError(t, err)
	require.Equal(t, "workspace-a", existing.WorkspaceID)

	_, err = decodeExtensionAuthorizationYakitOpen(
		json.RawMessage(`{"workspaceId":"workspace-a","tabId":11}`),
	)
	require.ErrorContains(t, err, "cannot be combined")
}

func TestExtensionAuthorizationInstances(t *testing.T) {
	connections := []ExtensionBridgeConnection{
		{DeviceID: "device-a", ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", Badge: "A"}},
		{DeviceID: "device-b", ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", Badge: "B"}},
		{DeviceID: "device-other"},
	}
	require.Equal(t, []extensionAuthorizationInstance{
		{DeviceID: "device-a", Badge: "A", Current: true},
		{DeviceID: "device-b", Badge: "B", Current: false},
	}, extensionAuthorizationInstances(connections, "device-a"))
}
