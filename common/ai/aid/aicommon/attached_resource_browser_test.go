package aicommon

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestAttachedBrowserResourceData(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeBrowser,
		AttachedResourceKeyBrowserDevice,
		`{"deviceId":"device-1","name":"Login debugging"}`,
	))
	require.NoError(t, err)
	browserResource, ok := resource.(*AttachedBrowserResourceData)
	require.True(t, ok)
	require.Equal(t, "device-1", browserResource.DeviceID)
	require.Contains(t, browserResource.ToAttachData(nil), "browser.capability.catalog")
	require.Contains(t, browserResource.ToAttachData(nil), "device-1")
	require.Contains(t, browserResource.ToAttachData(nil), "do not use use_browser")
	require.Contains(t, browserResource.ToAttachData(nil), "method browser.tabs")
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
	cfg := NewConfig(context.Background(), WithTools(catalogTool, callTool))
	cfg.GetAiToolManager().DisableTool(attachedBrowserCatalogToolName)
	cfg.GetAiToolManager().DisableTool(attachedBrowserCallToolName)
	loop := &attachedBrowserTestLoop{config: cfg}
	resource := &AttachedBrowserResourceData{DeviceID: "device-1", Name: "Chrome Browser"}

	require.NoError(t, resource.BindLoopData(loop))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserCatalogToolName))
	require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(attachedBrowserCallToolName))
	promptMaterials := BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserCatalogToolName)
	require.Contains(t, promptMaterials.PromotedTimelineOpen, "## Tool: "+attachedBrowserCallToolName)
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

	task.SetAttachedDatas(nil)
	allow, feedback = CheckAttachedBrowserToolRoute(task, attachedBrowserRodToolName)
	require.True(t, allow)
	require.Empty(t, feedback)

	allow, feedback = CheckAttachedBrowserToolRoute(task, attachedBrowserCallToolName)
	require.False(t, allow)
	require.Contains(t, feedback, "requires at least one attached")
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
		"browser_ref": "B",
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

func TestAttachedBrowserResourceRejectsMissingDevice(t *testing.T) {
	_, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeBrowser,
		AttachedResourceKeyBrowserDevice,
		`{"name":"Missing device"}`,
	))
	require.ErrorContains(t, err, "no deviceId")
}
