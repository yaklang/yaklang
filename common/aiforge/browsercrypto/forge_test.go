package browsercrypto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestEmbeddedPromptsRenderAndKeepCapabilityDiscoveryDynamic(t *testing.T) {
	require.NotEmpty(t, strings.TrimSpace(initializePrompt))
	require.NotEmpty(t, strings.TrimSpace(persistentPrompt))
	require.Contains(t, initializePrompt, "browser.capability.catalog")
	require.Contains(t, initializePrompt, "Reply in the user's language")
	require.Contains(t, persistentPrompt, "manual`, `ai`, or `yolo")
	require.Contains(t, persistentPrompt, "untrusted evidence")
	require.NotContains(t, initializePrompt, "browser.cookies")
	require.NotContains(t, initializePrompt, "browser.deep_capture.start")

	parsed, err := template.New("browser-crypto-init").Parse(initializePrompt)
	require.NoError(t, err)
	var rendered bytes.Buffer
	require.NoError(t, parsed.Execute(&rendered, map[string]interface{}{
		"Forge": map[string]interface{}{
			"UserParams": "query: inspect the current trace",
		},
	}))
	require.Contains(t, rendered.String(), "query: inspect the current trace")
}

type fakeBrowserCryptoAgentCaller struct {
	method    string
	params    map[string]interface{}
	available bool
	connected bool
	catalog   *browser.ExtensionBridgeCapabilityCatalog
}

type browserCryptoAgentContractCall struct {
	Method string
	Params map[string]interface{}
}

type browserCryptoAgentContractCaller struct {
	calls []browserCryptoAgentContractCall
}

func (c *browserCryptoAgentContractCaller) CallDevice(
	_ context.Context,
	_ string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	input, _ := params.(map[string]interface{})
	c.calls = append(c.calls, browserCryptoAgentContractCall{Method: method, Params: input})
	responses := map[string]string{
		"browser.recording.trace.list": `{
			"traces":[{"id":"trace-1","candidateIds":["candidate-1"]}]
		}`,
		"browser.recording.evidence.inspect": `{
			"trace":{"id":"trace-1"},
			"candidates":[{"id":"candidate-1","status":"ready"}],
			"callables":[{"id":"callable-1","kind":"request-transaction"}]
		}`,
		"browser.callable.inspect": `{
			"id":"callable-1",
			"kind":"request-transaction",
			"inputSlots":[{"name":"body","index":0}],
			"output":{"shape":"envelope","format":"json"}
		}`,
		"browser.profile.propose": `{
			"profile":{"name":"AES + RSA gateway","failMode":"closed"},
			"proposal":{"candidateId":"candidate-1","callableId":"callable-1","compiler":"browser-transform-guided-v1"}
		}`,
		"browser.profile.validate": `{
			"valid":true,
			"saveEligible":true,
			"proofLevel":"structure",
			"validationDraft":{"contractVersion":1,"id":"validation-1","createdAt":1000,"expiresAt":2000}
		}`,
	}
	response, ok := responses[method]
	if !ok {
		return nil, errors.New("unexpected Agent contract method: " + method)
	}
	return json.RawMessage(response), nil
}

func (f *fakeBrowserCryptoAgentCaller) CallDevice(
	_ context.Context,
	_ string,
	method string,
	params interface{},
) (json.RawMessage, error) {
	f.method = method
	f.params, _ = params.(map[string]interface{})
	return json.RawMessage(`{"ok":true}`), nil
}

func browserAgentToolResultMap(t *testing.T, result *aitool.ToolResult) map[string]interface{} {
	t.Helper()
	require.True(t, result.Success)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	value, ok := execution.Result.(map[string]interface{})
	require.True(t, ok)
	return value
}

func (f *fakeBrowserCryptoAgentCaller) Available() bool {
	return f.available
}

func (f *fakeBrowserCryptoAgentCaller) CapabilityCatalog(
	string,
) (*browser.ExtensionBridgeCapabilityCatalog, bool) {
	return f.catalog, f.connected && f.catalog != nil
}

func browserCryptoTestCapabilityCatalog(methods []string) *browser.ExtensionBridgeCapabilityCatalog {
	descriptors := make([]browser.ExtensionBridgeCapabilityDescriptor, 0, len(methods))
	for _, method := range methods {
		summary := "Browser capability"
		if method == "browser.eval" {
			summary = "Execute an async expression or program"
		}
		descriptors = append(descriptors, browser.ExtensionBridgeCapabilityDescriptor{
			Method: method, Domain: "page", Access: "read", Summary: summary,
			Scopes: []string{"browser.tabs.read"}, TargetMode: "document", DefaultTimeoutMS: 20_000,
			ParamsSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`),
		})
	}
	return &browser.ExtensionBridgeCapabilityCatalog{
		Version: 1, SchemaDialect: "http://json-schema.org/draft-07/schema#",
		Hash: "test-schema-hash", Capabilities: descriptors,
	}
}

func browserAgentToolByName(t *testing.T, tools []*aitool.Tool, name string) *aitool.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s not found", name)
	return nil
}

func TestParseBrowserCryptoAgentConfig(t *testing.T) {
	config, err := ParseConfig([]*ypb.ExecParamItem{
		{Key: "deviceId", Value: "device-1"},
		{Key: "tab_id", Value: "7"},
		{Key: "frame-id", Value: "2"},
		{Key: "document_id", Value: "document-1"},
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", config.DeviceID)
	require.Equal(t, 7, config.Target.TabID)
	require.Equal(t, 2, config.Target.FrameID)
	require.NotEmpty(t, config.Query)

	_, err = ParseConfig([]*ypb.ExecParamItem{
		{Key: "device_id", Value: "device-1"},
		{Key: "tab_id", Value: "0"},
	})
	require.ErrorContains(t, err, "positive tab_id")
}

func TestRunnerChecksRuntimeBridgeAndRequiredCapabilitiesBeforeStartingAI(t *testing.T) {
	params := []*ypb.ExecParamItem{
		{Key: "device_id", Value: "device-1"},
		{Key: "tab_id", Value: "7"},
	}

	runner := &Runner{bridge: &fakeBrowserCryptoAgentCaller{}}
	result, err := runner.Execute(context.Background(), params)
	require.ErrorContains(t, err, "bridge is not running")
	require.Nil(t, result)

	runner = &Runner{bridge: &fakeBrowserCryptoAgentCaller{
		available: true,
		connected: true,
		catalog: browserCryptoTestCapabilityCatalog(
			requiredCapabilities[:len(requiredCapabilities)-1],
		),
	}}
	result, err = runner.Execute(context.Background(), params)
	require.ErrorContains(t, err, "missing AI analysis capabilities")
	require.Nil(t, result)
}

func TestBrowserCryptoRuntimeReActBindsPageAndOnlyBrowserTools(t *testing.T) {
	bridge := &fakeBrowserCryptoAgentCaller{
		available: true,
		connected: true,
		catalog: browserCryptoTestCapabilityCatalog(
			append(append([]string{}, requiredCapabilities...), "browser.eval"),
		),
	}
	preparation, err := NewRunner(bridge).PrepareReAct(
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "device_id", Value: "device-1"},
			{Key: "tab_id", Value: "7"},
			{Key: "document_id", Value: "document-1"},
			{Key: "query", Value: "inspect the recorded encryption flow"},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, preparation)

	options := []aicommon.ConfigOption{aicommon.WithSystemFileOperator()}
	options = append(options, preparation.Options...)
	config := aicommon.NewConfig(context.Background(), options...)
	require.Equal(t, ForgeName, config.GetForgeName())
	require.Contains(t, config.PlanPrompt, `"page_target"`)
	require.Contains(t, config.PlanPrompt, "tabId=7")
	require.Contains(t, config.PlanPrompt, "documentId=document-1")
	require.True(t, config.DisallowMCPServers)
	require.True(t, config.IsAutoSkillsDisabled())
	require.True(t, config.DisableIntentRecognition)
	require.True(t, config.DisablePerception)
	require.False(t, config.EnableDispatchSubReactAgents)

	enabled, err := config.GetAiToolManager().GetEnableTools()
	require.NoError(t, err)
	names := make([]string, 0, len(enabled))
	for _, tool := range enabled {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 9)
	require.Contains(t, names, "recording.evidence.inspect")
	require.Contains(t, names, "browser.capability.call")
	require.NotContains(t, names, "ls")
	require.NotContains(t, names, "read_file")
	require.NotContains(t, names, "tools_search")
}

func TestBrowserCryptoAgentToolsBindTargetAndKeepProfileValidateDeterministic(t *testing.T) {
	caller := &fakeBrowserCryptoAgentCaller{}
	capabilities := append(
		append([]string{}, requiredCapabilities...),
		"browser.context",
		"browser.eval",
	)
	tools, err := buildTools(caller, "device-1", browsertools.Target{
		TabID: 7, FrameID: 2, DocumentID: "document-1",
	}, browserCryptoTestCapabilityCatalog(capabilities))
	require.NoError(t, err)
	require.Len(t, tools, 9)

	for _, name := range []string{
		"recording.trace.list",
		"recording.evidence.inspect",
		"callable.inspect",
		"callable.replay",
		"packet.compare",
		"profile.propose",
		"profile.validate",
		"browser.capability.call",
	} {
		require.False(t, browserAgentToolByName(t, tools, name).NoNeedUserReview, name)
	}

	traceTool := browserAgentToolByName(t, tools, "recording.trace.list")
	result, err := traceTool.InvokeWithParams(map[string]interface{}{"limit": 10})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "browser.recording.trace.list", caller.method)
	require.EqualValues(t, 7, caller.params["tabId"])
	require.EqualValues(t, 2, caller.params["frameId"])
	require.Equal(t, "document-1", caller.params["documentId"])

	validateTool := browserAgentToolByName(t, tools, "profile.validate")
	result, err = validateTool.InvokeWithParams(map[string]interface{}{
		"candidateId": "candidate-1",
		"callableId":  "callable-1",
		"inputPaths":  []interface{}{"body"},
		"packet": map[string]interface{}{
			"url":        "https://example.test/login",
			"headers":    []interface{}{},
			"bodyBase64": "",
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "browser.profile.validate", caller.method)
	require.EqualValues(t, 7, caller.params["tabId"])
	require.EqualValues(t, 2, caller.params["frameId"])
	require.NotContains(t, caller.params, "profile")

	catalogTool := browserAgentToolByName(t, tools, "browser.capability.catalog")
	require.True(t, catalogTool.NoNeedUserReview)
	result, err = catalogTool.InvokeWithParams(map[string]interface{}{
		"domain": "page",
		"query":  "async",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	executionResult, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	catalogResult, ok := executionResult.Result.(map[string]interface{})
	require.True(t, ok)
	catalogEntries, err := json.Marshal(catalogResult["capabilities"])
	require.NoError(t, err)
	require.Contains(t, string(catalogEntries), `"method":"browser.eval"`)

	capabilityTool := browserAgentToolByName(t, tools, "browser.capability.call")
	result, err = capabilityTool.InvokeWithParams(map[string]interface{}{
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

	result, err = capabilityTool.InvokeWithParams(map[string]interface{}{
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
}

func TestBrowserCryptoDeterministicAgentContract(t *testing.T) {
	caller := &browserCryptoAgentContractCaller{}
	tools, err := buildTools(
		caller,
		"browser-1",
		browsertools.Target{TabID: 7, FrameID: 0, DocumentID: "document-1"},
		browserCryptoTestCapabilityCatalog(requiredCapabilities),
	)
	require.NoError(t, err)

	invoke := func(name string, params map[string]interface{}) map[string]interface{} {
		t.Helper()
		result, invokeErr := browserAgentToolByName(t, tools, name).InvokeWithParams(params)
		require.NoError(t, invokeErr)
		return browserAgentToolResultMap(t, result)
	}

	traces := invoke("recording.trace.list", map[string]interface{}{"limit": 10})
	require.NotEmpty(t, traces["traces"])
	evidence := invoke("recording.evidence.inspect", map[string]interface{}{
		"traceId": "trace-1",
	})
	require.NotEmpty(t, evidence["candidates"])
	callable := invoke("callable.inspect", map[string]interface{}{
		"callableId": "callable-1",
	})
	require.Equal(t, "request-transaction", callable["kind"])
	proposal := invoke("profile.propose", map[string]interface{}{
		"candidateId": "candidate-1",
		"callableId":  "callable-1",
		"inputPaths":  []interface{}{"body"},
	})
	require.Equal(
		t,
		"browser-transform-guided-v1",
		proposal["proposal"].(map[string]interface{})["compiler"],
	)
	validation := invoke("profile.validate", map[string]interface{}{
		"candidateId": "candidate-1",
		"callableId":  "callable-1",
		"inputPaths":  []interface{}{"body"},
		"packet": map[string]interface{}{
			"method":     "POST",
			"url":        "https://example.test/encrypt/aesrsa.php",
			"headers":    []interface{}{},
			"bodyBase64": "e30=",
		},
	})
	require.Equal(t, true, validation["valid"])
	validationDraft := validation["validationDraft"].(map[string]interface{})
	require.Equal(
		t,
		json.Number(strconv.Itoa(BrowserTransformAgentContractVersion)),
		validationDraft["contractVersion"],
	)

	methods := make([]string, 0, len(caller.calls))
	for _, call := range caller.calls {
		methods = append(methods, call.Method)
	}
	require.Equal(t, []string{
		"browser.recording.trace.list",
		"browser.recording.evidence.inspect",
		"browser.callable.inspect",
		"browser.profile.propose",
		"browser.profile.validate",
	}, methods)
	for _, call := range caller.calls {
		require.EqualValues(t, 7, call.Params["tabId"], call.Method)
		require.EqualValues(t, 0, call.Params["frameId"], call.Method)
		require.Equal(t, "document-1", call.Params["documentId"], call.Method)
		require.NotContains(t, call.Params, "profile", call.Method)
	}
}
