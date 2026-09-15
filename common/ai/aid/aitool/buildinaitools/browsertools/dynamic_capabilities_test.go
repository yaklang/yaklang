package browsertools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

type fakeDynamicBridge struct {
	catalogs    map[string]*browser.ExtensionBridgeCapabilityCatalog
	connections []browser.ExtensionBridgeConnection
	deviceID    string
	method      string
	params      map[string]interface{}
}

func (f *fakeDynamicBridge) Available() bool { return true }

func (f *fakeDynamicBridge) Connections() []browser.ExtensionBridgeConnection {
	return append([]browser.ExtensionBridgeConnection(nil), f.connections...)
}

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
		Method: "browser.crypto.inspect", Domain: "recording", Access: "execute",
		Summary: "Inspect one crypto operation", Scopes: []string{"browser.dom.write", "browser.recording.control"},
		TargetMode: "document", DefaultTimeoutMS: 60_000,
		ParamsSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"tabId":{"type":"integer"},"frameId":{"type":"integer"},"documentId":{"type":"string"},
				"captureId":{"type":"string"},"nodeId":{"type":"string"},"settleMs":{"type":"integer","minimum":250,"maximum":5000}
			},
			"required":["captureId","nodeId"],"additionalProperties":false
		}`),
	}, browser.ExtensionBridgeCapabilityDescriptor{
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
	}, connections: []browser.ExtensionBridgeConnection{
		{DeviceID: "device-1", Client: "yakit-browser-extension", ManagedInstance: &browser.ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"}, CapabilityCatalog: testDynamicCapabilityCatalog()},
		{DeviceID: "device-2", Client: "yakit-browser-extension", ManagedInstance: &browser.ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-b", Badge: "B"}, CapabilityCatalog: testDynamicCapabilityCatalog()},
	}}
	tools, err := BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	require.Len(t, tools, 5)

	instancesTool := toolByName(t, tools, "browser.instances.list")
	catalogTool := toolByName(t, tools, "browser.capability.catalog")
	callTool := toolByName(t, tools, "browser.capability.call")
	cryptoTool := toolByName(t, tools, "browser.crypto.inspect")
	handoffTool := toolByName(t, tools, "browser.handoff.request")
	require.True(t, instancesTool.NoNeedUserReview)
	require.True(t, catalogTool.NoNeedUserReview)
	require.False(t, callTool.NoNeedUserReview)
	require.NotContains(t, catalogTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, callTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, cryptoTool.ToJSONSchemaString(), "device_id")
	require.NotContains(t, handoffTool.ToJSONSchemaString(), "device_id")
	require.Contains(t, catalogTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, callTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, cryptoTool.ToJSONSchemaString(), "browser_ref")
	require.Contains(t, handoffTool.ToJSONSchemaString(), "browser_ref")

	result, err := instancesTool.InvokeWithParams(nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	executionResult, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	instances, ok := executionResult.Result.([]map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "A", instances[0]["browserRef"])
	require.NotContains(t, instances[0], "deviceId")

	result, err = catalogTool.InvokeWithParams(map[string]interface{}{
		"browser_ref": "@B",
		"domain":      "page",
		"query":       "async",
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"browser_ref": "B",
		"method":      "browser.eval",
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

	result, err = cryptoTool.InvokeWithParams(map[string]interface{}{
		"browser_ref": "B",
		"captureId":   "capture-2",
		"nodeId":      "n7",
		"tabId":       9,
		"settleMs":    2_000,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "browser.crypto.inspect", bridge.method)
	require.Equal(t, "capture-2", bridge.params["captureId"])
	require.Equal(t, "n7", bridge.params["nodeId"])

	result, err = handoffTool.InvokeWithParams(map[string]interface{}{
		"browser_ref": "B",
		"reason":      "qr_code",
		"message":     "Scan to sign in",
		"tabId":       7,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "device-2", bridge.deviceID)
	require.Equal(t, "browser.handoff.request", bridge.method)
	require.Equal(t, "qr_code", bridge.params["reason"])
	require.EqualValues(t, 7, bridge.params["tabId"])

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"browser_ref": "offline",
		"method":      "browser.context",
		"params":      map[string]interface{}{},
	})
	require.ErrorContains(t, err, "offline")
	require.False(t, result.Success)
}

func TestDynamicCapabilityToolsAutomaticallySelectSoleOnlineBrowser(t *testing.T) {
	catalog := testDynamicCapabilityCatalog()
	bridge := &fakeDynamicBridge{
		catalogs: map[string]*browser.ExtensionBridgeCapabilityCatalog{"device-a": catalog},
		connections: []browser.ExtensionBridgeConnection{{
			DeviceID: "device-a", Client: "extension", CapabilityCatalog: catalog,
			ManagedInstance: &browser.ExtensionBridgeManagedInstance{Manager: "ytray", InstanceID: "instance-a", Badge: "A"},
		}},
	}
	tools, err := BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	result, err := toolByName(t, tools, "browser.capability.call").InvokeWithParams(map[string]interface{}{
		"method": "browser.eval",
		"params": map[string]interface{}{
			"tabId": 1, "frameId": 0, "documentId": "doc-a", "mode": "expression", "code": "document.title",
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "device-a", bridge.deviceID)
	require.Contains(t, RuntimeContext(bridge), "@-mention is optional")
	require.Contains(t, RuntimeContext(bridge), "The use_browser tool is unrelated Rod automation and never routes to these instances")
	require.Contains(t, RuntimeContext(bridge), "browser.capability.call")
	require.NotContains(t, RuntimeContext(bridge), "device-a")
}

func TestDynamicCapabilityToolsRequireReferenceForMultipleOnlineBrowsers(t *testing.T) {
	catalog := testDynamicCapabilityCatalog()
	bridge := &fakeDynamicBridge{
		catalogs: map[string]*browser.ExtensionBridgeCapabilityCatalog{"device-a": catalog, "device-b": catalog},
		connections: []browser.ExtensionBridgeConnection{
			{DeviceID: "device-a", CapabilityCatalog: catalog, ManagedInstance: &browser.ExtensionBridgeManagedInstance{Badge: "A"}},
			{DeviceID: "device-b", CapabilityCatalog: catalog, ManagedInstance: &browser.ExtensionBridgeManagedInstance{Badge: "B"}},
		},
	}
	tools, err := BuildDynamicCapabilityTools(bridge)
	require.NoError(t, err)
	result, err := toolByName(t, tools, "browser.capability.call").InvokeWithParams(map[string]interface{}{
		"method": "browser.eval",
		"params": map[string]interface{}{
			"tabId": 1, "frameId": 0, "documentId": "doc-a", "mode": "expression", "code": "document.title",
		},
	})
	require.ErrorContains(t, err, "multiple browser-extension instances are online (A, B)")
	require.False(t, result.Success)
}
