package browsertools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/browser"
)

type fakeDynamicBridge struct {
	catalogs map[string]*browser.ExtensionBridgeCapabilityCatalog
	deviceID string
	method   string
	params   map[string]interface{}
}

func (f *fakeDynamicBridge) Available() bool { return true }

func (f *fakeDynamicBridge) CapabilityCatalog(deviceID string) (*browser.ExtensionBridgeCapabilityCatalog, bool) {
	catalog, ok := f.catalogs[deviceID]
	return catalog, ok
}

func (f *fakeDynamicBridge) CallDevice(
	_ context.Context,
	deviceID string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	f.deviceID = deviceID
	f.method = method
	f.params, _ = params.(map[string]interface{})
	return json.RawMessage(`{"ok":true}`), nil
}

func TestDynamicCapabilityToolsSelectAndValidateDeviceAtInvocationTime(t *testing.T) {
	bridge := &fakeDynamicBridge{catalogs: map[string]*browser.ExtensionBridgeCapabilityCatalog{
		"device-1": testCapabilityCatalog(),
		"device-2": testCapabilityCatalog(),
	}}
	tools, err := BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	require.Len(t, tools, 2)

	catalogTool := toolByName(t, tools, "browser.capability.catalog")
	callTool := toolByName(t, tools, "browser.capability.call")
	require.True(t, catalogTool.NoNeedUserReview)
	require.False(t, callTool.NoNeedUserReview)
	require.NotContains(t, catalogTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, callTool.ToJSONSchemaString(), "device_id")
	require.Contains(t, catalogTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, callTool.ToJSONSchemaString(), "browser_ref")

	result, err := catalogTool.InvokeWithParams(map[string]interface{}{
		"device_id": "device-2",
		"query":     "async",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"device_id": "device-2",
		"method":    "browser.eval",
		"params": map[string]interface{}{
			"tabId":      9,
			"frameId":    0,
			"documentId": "document-2",
			"mode":       "expression",
			"code":       "document.title",
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "device-2", bridge.deviceID)
	require.Equal(t, "browser.eval", bridge.method)
	require.EqualValues(t, 9, bridge.params["tabId"])

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"device_id": "device-offline",
		"method":    "browser.context",
		"params":    map[string]interface{}{},
	})
	require.ErrorContains(t, err, "offline")
	require.False(t, result.Success)
}
