package aicommon

import (
	"context"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestAttachedBrowserResourceData(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeBrowser,
		AttachedResourceKeyBrowserDevice,
		`{"deviceId":"device-1","name":"Login debugging","reference":"A"}`,
	))
	require.NoError(t, err)
	browserResource, ok := resource.(*AttachedBrowserResourceData)
	require.True(t, ok)
	require.Equal(t, "device-1", browserResource.DeviceID)
	require.Contains(t, browserResource.ToAttachData(nil), "browser.capability.catalog")
	require.NotContains(t, browserResource.ToAttachData(nil), "device-1")
	require.Contains(t, browserResource.ToAttachData(nil), "do not use use_browser")
	require.Contains(t, browserResource.ToAttachData(nil), "method browser.tabs")
	require.Contains(t, browserResource.ToAttachData(nil), `{"browser_ref":"...","method":"browser.*","params":{...}}`)
	require.Contains(t, browserResource.ToAttachData(nil), attachedBrowserCryptoToolName)
	require.Contains(t, browserResource.ToAttachData(nil), "postAction.sameDocument")
	require.Contains(t, browserResource.ToAttachData(nil), "domain=transform")
	require.Contains(t, browserResource.ToAttachData(nil), attachedBrowserHTTPToolName)
	require.Contains(t, browserResource.ToAttachData(nil), "without permanently saving a Profile")
}

func TestAttachedBrowserResourcePromotesBridgeTools(t *testing.T) {
	catalogTool := aitool.NewWithoutCallback(
		attachedBrowserCatalogToolName,
		aitool.WithDescription("catalog"),
		aitool.WithStringParam("device_id"),
	)
	callTool := aitool.NewWithoutCallback(
		attachedBrowserCallToolName,
		aitool.WithDescription("call"),
		aitool.WithStringParam("device_id"),
	)
	cryptoTool := aitool.NewWithoutCallback(
		attachedBrowserCryptoToolName,
		aitool.WithDescription("crypto"),
		aitool.WithStringParam("device_id"),
	)
	prepareTool := aitool.NewWithoutCallback(
		attachedBrowserPrepareToolName,
		aitool.WithDescription("prepare"),
		aitool.WithStringParam("device_id"),
	)
	handoffTool := aitool.NewWithoutCallback(
		attachedBrowserHandoffToolName,
		aitool.WithDescription("handoff"),
		aitool.WithStringParam("device_id"),
	)
	httpTool := aitool.NewWithoutCallback(
		attachedBrowserHTTPToolName,
		aitool.WithDescription("http"),
		aitool.WithStringParam("device_id"),
	)
	cfg := NewConfig(context.Background(), WithTools(catalogTool, callTool, cryptoTool, prepareTool, handoffTool, httpTool))
	cfg.GetAiToolManager().DisableTool(attachedBrowserCatalogToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserCallToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserCryptoToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserPrepareToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserHandoffToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserHTTPToolName)
	loop := &attachedBrowserTestLoop{config: cfg}
	resource := &AttachedBrowserResourceData{DeviceID: "device-1", Name: "Chrome Browser"}

	require.NoError(t, resource.BindLoopData(loop))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserCatalogToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserCallToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserCryptoToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserPrepareToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserHandoffToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserHTTPToolName))
	promptMaterials := BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserCatalogToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserCallToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserCryptoToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserPrepareToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserHandoffToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserHTTPToolName)
	require.Contains(t, resource.ToAttachData(loop), "tools are available and have been promoted")
}

func TestAttachedBrowserResourceDoesNotSuggestRodFallbackWhenBridgeToolsMissing(t *testing.T) {
	cfg := NewConfig(context.Background())
	loop := &attachedBrowserTestLoop{config: cfg}
	resource := &AttachedBrowserResourceData{DeviceID: "device-1", Name: "Chrome Browser"}

	require.NoError(t, resource.BindLoopData(loop))
	rendered := resource.ToAttachData(loop)
	require.Contains(t, rendered, "Bridge routing is unavailable")
	require.Contains(t, rendered, "do not open a replacement browser")
}

func TestAttachedBrowserResourceBlocksRodIdentitySwitch(t *testing.T) {
	task := NewStatefulTaskBase("browser-route", "inspect current browser", context.Background(), nil, true)
	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(
			AttachedResourceTypeBrowser,
			AttachedResourceKeyBrowserDevice,
			`{"deviceId":"device-1","name":"Chrome Browser"}`,
		),
	})

	allow, feedback := CheckAttachedBrowserToolRoute(task, attachedBrowserRodToolName)
	require.False(t, allow)
	require.Contains(t, feedback, attachedBrowserCatalogToolName)
	require.Contains(t, feedback, attachedBrowserCallToolName)
	require.Contains(t, feedback, attachedBrowserCryptoToolName)
	require.Contains(t, feedback, attachedBrowserPrepareToolName)
	require.Contains(t, feedback, attachedBrowserHandoffToolName)
	require.Contains(t, feedback, attachedBrowserHTTPToolName)
	require.Contains(t, feedback, "Do not call op=open")

	allow, feedback = CheckAttachedBrowserToolRoute(task, attachedBrowserCatalogToolName)
	require.True(t, allow)
	require.Empty(t, feedback)

	params, err := BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"device_id": "model-selected-other-device",
		"method":    "browser.tabs",
		"params":    aitool.InvokeParams{},
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))

	params, err = BindAttachedBrowserToolParams(task, attachedBrowserHandoffToolName, aitool.InvokeParams{
		"browser_ref": "device-1",
		"reason":      "qr_code",
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	params, err = BindAttachedBrowserToolParams(task, attachedBrowserCryptoToolName, aitool.InvokeParams{
		"browser_ref": "device-1",
		"captureId":   "capture-1",
		"nodeId":      "n1",
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	task.SetAttachedDatas(nil)
	allow, feedback = CheckAttachedBrowserToolRoute(task, attachedBrowserRodToolName)
	require.True(t, allow)
	require.Empty(t, feedback)

	allow, feedback = CheckAttachedBrowserToolRoute(task, attachedBrowserCallToolName)
	require.True(t, allow)
	require.Empty(t, feedback)
}

func TestBrowserExtensionIntentBlocksRodWithoutMentionAttachment(t *testing.T) {
	task := NewStatefulTaskBase(
		"browser-route",
		"我打开了实例A，请着重测试浏览器插件能力，必须通过浏览器插件",
		context.Background(),
		nil,
		true,
	)

	allow, feedback := CheckAttachedBrowserToolRoute(task, attachedBrowserRodToolName)
	require.False(t, allow)
	require.Contains(t, feedback, attachedBrowserCatalogToolName)
	require.Contains(t, feedback, "instead of creating a replacement Rod browser")
}

func TestBrowserBridgeToolsArePrioritizedInInventory(t *testing.T) {
	tools := make([]*aitool.Tool, 0, 30)
	for index := 0; index < 24; index++ {
		tools = append(tools, aitool.NewWithoutCallback("filler-"+strconv.Itoa(index)))
	}
	browserToolNames := []string{
		"browser.instances.list",
		attachedBrowserCatalogToolName,
		attachedBrowserCallToolName,
		attachedBrowserCryptoToolName,
		attachedBrowserPrepareToolName,
		attachedBrowserHandoffToolName,
		attachedBrowserHTTPToolName,
	}
	for _, name := range browserToolNames {
		tools = append(tools, aitool.NewWithoutCallback(name))
	}

	prioritized := PrioritizeToolsForInventory(tools, ToolInventoryMinCount)
	prioritizedNames := make([]string, 0, len(prioritized))
	for _, tool := range prioritized {
		prioritizedNames = append(prioritizedNames, tool.Name)
	}
	for _, name := range browserToolNames {
		require.Contains(t, prioritizedNames, name)
	}
}

func TestAttachedBrowserResourceRoutesMultipleMentionsByReference(t *testing.T) {
	task := NewStatefulTaskBase("browser-route", "compare browsers", context.Background(), nil, true)
	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice,
			`{"deviceId":"device-a","name":"A · Chrome Browser","reference":"A"}`),
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice,
			`{"deviceId":"device-b","name":"B · Chrome Browser","reference":"B"}`),
	})

	params, err := BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "@B",
		"method":      "browser.tabs",
	})
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	_, err = BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"method": "browser.tabs",
	})
	require.ErrorContains(t, err, "browser_ref is required")

	_, err = BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "C",
		"method":      "browser.tabs",
	})
	require.ErrorContains(t, err, "does not match an attached browser")
}

func TestToolCallerBindsAttachedBrowserDevice(t *testing.T) {
	ctx := context.Background()
	var invokedDeviceID string
	tool, err := aitool.New(
		attachedBrowserCallToolName,
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam("method", aitool.WithParam_Required(true)),
		aitool.WithNoRuntimeCallback(func(_ context.Context, params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			invokedDeviceID = params.GetString("device_id")
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()))
	task := NewStatefulTaskBase("browser-call", "inspect browser", ctx, cfg.Emitter, true)
	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-1"}`),
	})
	caller, err := NewToolCaller(
		ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Task(task),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_RuntimeId("browser-call"),
	)
	require.NoError(t, err)

	result, _, err := caller.CallToolWithExistedParams(tool, true, aitool.InvokeParams{
		"device_id": "model-selected-other-device",
		"method":    "browser.tabs",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "device-1", invokedDeviceID)
}

func TestToolCallerBlocksRodForBrowserExtensionIntent(t *testing.T) {
	ctx := context.Background()
	invoked := false
	tool, err := aitool.New(
		attachedBrowserRodToolName,
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(context.Context, aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			invoked = true
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()))
	task := NewStatefulTaskBase("browser-call", "我用 YTray 打开了实例 A，只能通过浏览器插件测试", ctx, cfg.Emitter, true)
	caller, err := NewToolCaller(
		ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_AICaller(cfg),
		WithToolCaller_Task(task),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_RuntimeId("browser-call"),
	)
	require.NoError(t, err)

	_, _, err = caller.CallToolWithExistedParams(tool, true, nil)
	require.ErrorContains(t, err, "explicitly requested an existing browser-extension/YTray instance")
	require.False(t, invoked)
}

func TestAttachedBrowserResourceRejectsMissingDevice(t *testing.T) {
	_, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeBrowser,
		AttachedResourceKeyBrowserDevice,
		`{"name":"Missing device"}`,
	))
	require.ErrorContains(t, err, "no deviceId")
}
