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

func TestExtensionAuthorizationClientWorkspaceRequiresCallerInstance(t *testing.T) {
	var input extensionAuthorizationClientWorkspaceInput
	err := decodeExtensionAuthorizationClientJSON(
		json.RawMessage(`{"mode":"horizontal","left":{"deviceId":"device-a","tabId":11,"frameId":0,"accountLabel":"A"},"right":{"deviceId":"device-b","tabId":12,"frameId":0,"accountLabel":"B"}}`),
		&input,
	)
	require.NoError(t, err)
	prepared, err := extensionAuthorizationWorkspaceInputForDevice("device-a", input)
	require.NoError(t, err)
	require.Equal(t, "device-a", prepared.Left.DeviceID)
	require.Equal(t, "device-b", prepared.Right.DeviceID)

	_, err = extensionAuthorizationWorkspaceInputForDevice(
		"device-b",
		input,
	)
	require.ErrorContains(t, err, "left identity")
}

func TestExtensionAuthorizationInstances(t *testing.T) {
	connections := []ExtensionBridgeConnection{
		{DeviceID: "device-a", ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", Badge: "A"}},
		{DeviceID: "device-b", ManagedInstance: &ExtensionBridgeManagedInstance{Manager: "ytray", Badge: "B"}},
		{DeviceID: "device-other"},
	}
	require.Equal(t, []extensionAuthorizationInstance{
		{DeviceID: "device-a", Badge: "A", Current: true, Tabs: []extensionAuthorizationInstanceTab{}},
		{DeviceID: "device-b", Badge: "B", Current: false, Tabs: []extensionAuthorizationInstanceTab{}},
	}, extensionAuthorizationInstances(connections, "device-a"))
}

func TestExtensionAuthorizationClientSlotKeepsRemoteDevice(t *testing.T) {
	workspace := ExtensionAuthorizationWorkspace{
		Left:  ExtensionAuthorizationIdentitySlot{DeviceID: "device-a"},
		Right: ExtensionAuthorizationIdentitySlot{DeviceID: "device-b"},
	}
	right, err := extensionAuthorizationClientSlot(workspace, "right")
	require.NoError(t, err)
	require.Equal(t, "device-b", right.DeviceID)
	_, err = extensionAuthorizationClientSlot(workspace, "unknown")
	require.Error(t, err)
}
