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
	if method == "browser.handoff.request" {
		return json.RawMessage(`{
			"id":"handoff-1","reason":"qr_code","state":"waiting_for_user","requestedAt":1,
			"target":{"tabId":7,"frameId":0,"title":"Sign in","grantedUrl":"https://example.test/login","origin":"https://example.test"}
		}`), nil
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func testDynamicCapabilityCatalog() *browser.ExtensionBridgeCapabilityCatalog {
	catalog := testCapabilityCatalog()
	catalog.Capabilities = append(catalog.Capabilities, browser.ExtensionBridgeCapabilityDescriptor{
		Method: "browser.handoff.request", Domain: "handoff", Access: "write",
		Summary: "Wait for local user verification", Scopes: []string{"browser.human.takeover"},
		TargetMode: "document", DefaultTimeoutMS: 20_000,
		ParamsSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"tabId":{"type":"integer"},"frameId":{"type":"integer"},"documentId":{"type":"string"},
				"reason":{"enum":["qr_code","mfa","captcha","device_confirmation","other"]},"message":{"type":"string"}
			},
			"required":["reason"],"additionalProperties":false
		}`),
	})
	return catalog
}

func TestDynamicCapabilityToolsSelectAndValidateDeviceAtInvocationTime(t *testing.T) {
	bridge := &fakeDynamicBridge{catalogs: map[string]*browser.ExtensionBridgeCapabilityCatalog{
		"device-1": testDynamicCapabilityCatalog(),
		"device-2": testDynamicCapabilityCatalog(),
	}}
	tools, err := BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	require.Len(t, tools, 3)

	catalogTool := toolByName(t, tools, "browser.capability.catalog")
	callTool := toolByName(t, tools, "browser.capability.call")
	handoffTool := toolByName(t, tools, "browser.handoff.request")
	require.True(t, catalogTool.NoNeedUserReview)
	require.False(t, callTool.NoNeedUserReview)
	require.NotContains(t, catalogTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, callTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, handoffTool.ToJSONSchemaString(), "device_id")
	require.Contains(t, catalogTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, callTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, handoffTool.ToJSONSchemaString(), "browser_ref")

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

	result, err = handoffTool.InvokeWithParams(map[string]interface{}{
		"device_id": "device-2",
		"reason":    "qr_code",
		"message":   "Scan to sign in",
		"tabId":     7,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "device-2", bridge.deviceID)
	require.Equal(t, "browser.handoff.request", bridge.method)
	require.Equal(t, "qr_code", bridge.params["reason"])
	require.EqualValues(t, 7, bridge.params["tabId"])

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"device_id": "device-offline",
		"method":    "browser.context",
		"params":    map[string]interface{}{},
	})
	require.ErrorContains(t, err, "offline")
	require.False(t, result.Success)
}
