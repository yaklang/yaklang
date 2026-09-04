package browsertools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

type fakeCaller struct {
	method string
	params map[string]interface{}
	calls  int
}

func (f *fakeCaller) CallDevice(
	_ context.Context,
	_ string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	f.calls++
	f.method = method
	f.params, _ = params.(map[string]interface{})
	return json.RawMessage(`{"ok":true}`), nil
}

func testCapabilityCatalog() *browser.ExtensionBridgeCapabilityCatalog {
	targetProperties := `"tabId":{"type":"integer"},"frameId":{"type":"integer"},"documentId":{"type":"string"}`
	return &browser.ExtensionBridgeCapabilityCatalog{
		Version:       1,
		SchemaDialect: "http://json-schema.org/draft-07/schema#",
		Hash:          "test-schema-hash",
		Capabilities: []browser.ExtensionBridgeCapabilityDescriptor{
			{
				Method: "browser.context", Domain: "page", Access: "read",
				Summary: "Read the selected page context", Scopes: []string{"browser.tabs.read"},
				TargetMode: "document", DefaultTimeoutMS: 20_000,
				ParamsSchema: json.RawMessage(`{
					"type":"object",
					"properties":{` + targetProperties + `,"includeDom":{"type":"boolean"}},
					"additionalProperties":false
				}`),
			},
			{
				Method: "browser.eval", Domain: "page", Access: "execute",
				Summary: "Execute an async expression or program", Scopes: []string{"browser.page.execute"},
				TargetMode: "document", DefaultTimeoutMS: 20_000,
				ParamsSchema: json.RawMessage(`{
					"type":"object",
					"properties":{` + targetProperties + `,"mode":{"enum":["expression","program"]},"code":{"type":"string"}},
					"required":["mode","code"],
					"additionalProperties":false
				}`),
			},
		},
	}
}

func toolByName(t *testing.T, tools []*aitool.Tool, name string) *aitool.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func TestCapabilityToolsUseAdvertisedMethodsReviewAndTargetRules(t *testing.T) {
	factory := aitool.NewFactory()
	caller := &fakeCaller{}
	target := Target{TabID: 7, FrameID: 2, DocumentID: "document-1"}
	require.NoError(t, RegisterCapabilityTools(
		factory,
		caller,
		"device-1",
		target,
		testCapabilityCatalog(),
	))

	tools := factory.Tools()
	require.Len(t, tools, 2)
	catalogTool := toolByName(t, tools, "browser.capability.catalog")
	callTool := toolByName(t, tools, "browser.capability.call")
	require.True(t, catalogTool.NoNeedUserReview)
	require.False(t, callTool.NoNeedUserReview)

	result, err := catalogTool.InvokeWithParams(map[string]interface{}{
		"domain": "page",
		"query":  "async",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	executionResult, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	catalogResult, ok := executionResult.Result.(map[string]interface{})
	require.True(t, ok)
	entries, ok := catalogResult["capabilities"].([]browser.ExtensionBridgeCapabilityDescriptor)
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "browser.eval", entries[0].Method)
	require.Equal(t, "test-schema-hash", catalogResult["schemaHash"])

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"method": "browser.eval",
		"params": map[string]interface{}{
			"mode": "expression",
			"code": "document.title",
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "browser.eval", caller.method)
	require.EqualValues(t, 7, caller.params["tabId"])
	require.EqualValues(t, 2, caller.params["frameId"])
	require.Equal(t, "document-1", caller.params["documentId"])

	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"method": "browser.context",
		"params": map[string]interface{}{
			"tabId":      9,
			"includeDom": true,
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.EqualValues(t, 9, caller.params["tabId"])
	require.NotContains(t, caller.params, "frameId")
	require.NotContains(t, caller.params, "documentId")

	previousCalls := caller.calls
	result, err = callTool.InvokeWithParams(map[string]interface{}{
		"method": "browser.eval",
		"params": map[string]interface{}{
			"mode": "expression",
		},
	})
	require.ErrorContains(t, err, "missing property 'code'")
	require.False(t, result.Success)
	require.Equal(t, previousCalls, caller.calls)
}

func TestCapabilityToolsExcludeUIOnlyMethods(t *testing.T) {
	hidden := false
	catalog := testCapabilityCatalog()
	catalog.Capabilities = append(catalog.Capabilities, browser.ExtensionBridgeCapabilityDescriptor{
		Method: "browser.handoff.presentation.get", Domain: "handoff", Access: "sensitive-read",
		AgentVisible: &hidden, Summary: "Local UI presentation", Scopes: []string{"browser.human.takeover"},
		TargetMode: "none", DefaultTimeoutMS: 20_000,
		ParamsSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})

	descriptors, methods, err := browserCapabilityDescriptors(catalog)
	require.NoError(t, err)
	require.NotContains(t, descriptors, "browser.handoff.presentation.get")
	require.NotContains(t, methods, "browser.handoff.presentation.get")
	require.NotContains(t, browserCapabilityCatalog(catalog, "all", ""), catalog.Capabilities[2])
}
