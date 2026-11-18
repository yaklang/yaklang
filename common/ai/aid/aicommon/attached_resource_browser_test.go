package aicommon

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type browserAttachmentTestInvoker struct {
	AIInvokeRuntime
	bindings []AttachedBrowserBinding
}

func (i *browserAttachmentTestInvoker) GetBrowserAttachments() []AttachedBrowserBinding {
	return append([]AttachedBrowserBinding(nil), i.bindings...)
}

func (i *browserAttachmentTestInvoker) AddBrowserAttachments(bindings []AttachedBrowserBinding) error {
	seen := make(map[string]struct{}, len(i.bindings))
	for _, binding := range i.bindings {
		seen[binding.DeviceID] = struct{}{}
	}
	for _, binding := range bindings {
		if _, exists := seen[binding.DeviceID]; exists {
			continue
		}
		seen[binding.DeviceID] = struct{}{}
		i.bindings = append(i.bindings, binding)
	}
	return nil
}

func newBrowserAttachmentTestTask(
	t *testing.T,
	taskID string,
	input string,
	bindings ...AttachedBrowserBinding,
) (*AIStatefulTaskBase, *browserAttachmentTestInvoker) {
	t.Helper()
	task := NewStatefulTaskBase(taskID, input, context.Background(), nil, true)
	invoker := &browserAttachmentTestInvoker{bindings: bindings}
	task.SetReActLoop(&attachedBrowserTestLoop{invoker: invoker})
	return task, invoker
}

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
	require.Equal(t, AttachedBrowserBinding{
		DeviceID: "device-1", Name: "Login debugging", Reference: "device-1",
	}, browserResource.Binding())
	require.Contains(t, browserResource.ToAttachData(nil), "`device-1` (Login debugging)")
}

func TestAttachedBrowserResourceUsesBridgeOwnedReference(t *testing.T) {
	cfg := NewConfig(
		context.Background(),
		WithBrowserAttachmentReferenceResolver(func(deviceID string) (string, error) {
			require.Equal(t, "device-1", deviceID)
			return "br_7", nil
		}),
	)
	invoker := &browserAttachmentTestInvoker{}
	loop := &attachedBrowserTestLoop{config: cfg, invoker: invoker}
	resource, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeBrowser,
		AttachedResourceKeyBrowserDevice,
		`{"deviceId":"device-1","name":"Login debugging","reference":"A"}`,
	))
	require.NoError(t, err)
	require.NoError(t, resource.BindLoopData(loop))
	require.Equal(t, []AttachedBrowserBinding{{
		DeviceID: "device-1", Name: "Login debugging", Reference: "br_7",
	}}, invoker.bindings)
	require.Contains(t, resource.ToAttachData(loop), "`br_7` (Login debugging)")

	task := NewStatefulTaskBase("browser-resolved-ref", "inspect", context.Background(), nil, true)
	task.SetReActLoop(loop)
	params, err := BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "br_7",
		"method":      "browser.tabs",
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))

	child := NewConfig(context.Background(), ConvertConfigToOptions(cfg)...)
	require.NotNil(t, child.GetBrowserAttachmentReferenceResolver())
}

func TestAttachedBrowserResourceAddsSessionAndPromotesTools(t *testing.T) {
	toolNames := AttachedBrowserBridgeToolNames()
	tools := make([]*aitool.Tool, 0, len(toolNames))
	for _, name := range toolNames {
		tools = append(tools, aitool.NewWithoutCallback(name))
	}
	cfg := NewConfig(context.Background(), WithTools(tools...))
	for _, name := range toolNames {
		cfg.GetAiToolManager().DisableTool(name)
	}
	invoker := &browserAttachmentTestInvoker{}
	loop := &attachedBrowserTestLoop{config: cfg, invoker: invoker}

	resources := []AttachedResourceData{
		&AttachedBrowserResourceData{DeviceID: "device-a", Reference: "br_1"},
		&AttachedBrowserResourceData{DeviceID: "device-b", Reference: "br_2"},
	}
	require.Equal(t,
		(&AttachedBrowserResourceData{DeviceID: "device-a", Reference: "br_1"}).DedupKey(),
		(&AttachedBrowserResourceData{DeviceID: "device-a", Name: "same device, changed name"}).DedupKey(),
	)
	for _, resource := range resources {
		require.NoError(t, resource.BindLoopData(loop))
	}
	require.Equal(t, []AttachedBrowserBinding{
		{DeviceID: "device-a", Reference: "br_1"},
		{DeviceID: "device-b", Reference: "br_2"},
	}, invoker.bindings)
	for _, name := range toolNames {
		require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(name), name)
	}
	var rendered []string
	for _, resource := range resources {
		rendered = append(rendered, resource.ToAttachData(loop))
	}
	require.Contains(t, strings.Join(rendered, "\n"), "`br_1`")
	require.Contains(t, strings.Join(rendered, "\n"), "`br_2`")
	require.NotContains(t, strings.Join(rendered, "\n"), "device-a")
}

func TestAttachedBrowserResourceBlocksRodIdentitySwitch(t *testing.T) {
	task, invoker := newBrowserAttachmentTestTask(
		t,
		"browser-route",
		"inspect current browser",
		AttachedBrowserBinding{DeviceID: "device-1", Name: "Chrome Browser", Reference: "br_1"},
	)

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
		"browser_ref": "br_1",
		"reason":      "qr_code",
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	params, err = BindAttachedBrowserToolParams(task, attachedBrowserCryptoToolName, aitool.InvokeParams{
		"browser_ref": "br_1",
		"captureId":   "capture-1",
		"nodeId":      "n1",
	})
	require.NoError(t, err)
	require.Equal(t, "device-1", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	invoker.bindings = nil
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
	task, _ := newBrowserAttachmentTestTask(
		t,
		"browser-route",
		"compare browsers",
		AttachedBrowserBinding{DeviceID: "device-a", Name: "A · Chrome Browser", Reference: "br_1"},
		AttachedBrowserBinding{DeviceID: "device-b", Name: "B · Chrome Browser", Reference: "br_2"},
	)

	params, err := BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "@br_2",
		"method":      "browser.tabs",
	})
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))
	require.NotContains(t, params, "browser_ref")

	_, err = BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "B",
		"method":      "browser.tabs",
	})
	require.ErrorContains(t, err, "does not match")

	_, err = BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"method": "browser.tabs",
	})
	require.ErrorContains(t, err, "browser_ref is required")

	_, err = BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "br_3",
		"method":      "browser.tabs",
	})
	require.ErrorContains(t, err, "does not match an attached browser")
}

func TestAttachedBrowserResourceRejectsAmbiguousReference(t *testing.T) {
	task, _ := newBrowserAttachmentTestTask(
		t,
		"browser-ambiguous",
		"compare browsers",
		AttachedBrowserBinding{DeviceID: "device-a", Reference: "br_9"},
		AttachedBrowserBinding{DeviceID: "device-b", Reference: "br_9"},
	)

	_, err := BindAttachedBrowserToolParams(task, attachedBrowserCallToolName, aitool.InvokeParams{
		"browser_ref": "br_9",
		"method":      "browser.tabs",
	})
	require.ErrorContains(t, err, "is ambiguous between attached devices")
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
	invoker := &browserAttachmentTestInvoker{bindings: []AttachedBrowserBinding{{
		DeviceID: "device-1", Reference: "br_1",
	}}}
	caller, err := NewToolCaller(
		ctx,
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_Task(task),
		WithToolCaller_InvokeRuntime(invoker),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_RuntimeId("browser-call"),
	)
	require.NoError(t, err)

	result, _, err := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{
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

		WithToolCaller_Task(task),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_RuntimeId("browser-call"),
	)
	require.NoError(t, err)

	_, _, err = caller.CallToolWithExistedParams(tool, aitool.InvokeParams{})
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
