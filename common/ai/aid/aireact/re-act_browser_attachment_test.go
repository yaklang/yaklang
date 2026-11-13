package aireact

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func browserAttachment(deviceID, reference string) *ypb.AttachedResourceInfo {
	return &ypb.AttachedResourceInfo{
		Type:  aicommon.AttachedResourceTypeBrowser,
		Key:   aicommon.AttachedResourceKeyBrowserDevice,
		Value: `{"deviceId":"` + deviceID + `","reference":"` + reference + `"}`,
	}
}

func browserTaskBindings(t *testing.T, task aicommon.AIStatefulTask) []aicommon.AttachedBrowserBinding {
	t.Helper()
	return getTaskBrowserBindings(task)
}

func browserReferences(bindings []aicommon.AttachedBrowserBinding) []string {
	references := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		references = append(references, binding.Reference)
	}
	return references
}

func newBrowserAttachmentTestReAct(t *testing.T, options ...aicommon.ConfigOption) *ReAct {
	t.Helper()
	cfg := aicommon.NewConfig(context.Background(), options...)
	return &ReAct{config: cfg, Emitter: cfg.Emitter}
}

type browserRouteTestLoop struct {
	aicommon.ReActLoopIF
	invoker aicommon.AIInvokeRuntime
}

func (l *browserRouteTestLoop) GetInvoker() aicommon.AIInvokeRuntime {
	return l.invoker
}

func bindBrowserRouteTestRuntime(task aicommon.AIStatefulTask, runtime aicommon.AIInvokeRuntime) {
	task.SetReActLoop(&browserRouteTestLoop{invoker: runtime})
}

func TestReActBrowserAttachmentsUseTaskSnapshots(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)

	firstEvent := &ypb.AIInputEvent{
		FreeInput: "open bilibili",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
			{Type: aicommon.AttachedResourceTypeSelected, Key: aicommon.AttachedResourceKeyContent, Value: "first-turn-only"},
		},
	}
	firstResources, firstControl := normalizeInputAttachments(firstEvent)
	first := react.buildReTaskFromEvent(firstEvent, firstResources, firstControl)
	require.Equal(t, []string{"A"}, browserReferences(browserTaskBindings(t, first)))
	require.Len(t, first.GetAttachedDatas(), 1, "browser resources must not enter generic task attachments")
	require.Empty(t, react.GetActiveBrowserAttachments(), "a queued task must not change the active browser yet")

	secondEvent := &ypb.AIInputEvent{
		FreeInput: "compare B",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-b", "B"),
		},
	}
	secondResources, secondControl := normalizeInputAttachments(secondEvent)
	second := react.buildReTaskFromEvent(secondEvent, secondResources, secondControl)
	require.Equal(t, []string{"B"}, browserReferences(browserTaskBindings(t, second)))

	react.prepareBrowserAttachmentForTask(first)
	require.Equal(t, []string{"A"}, browserReferences(react.GetActiveBrowserAttachments()))
	bindBrowserRouteTestRuntime(first, react)
	params, err := aicommon.BindAttachedBrowserToolParams(first, "browser.capability.call", nil)
	require.NoError(t, err)
	require.Equal(t, "device-a", params.GetString("device_id"))

	react.prepareBrowserAttachmentForTask(second)
	require.Equal(t, []string{"B"}, browserReferences(react.GetActiveBrowserAttachments()))
	bindBrowserRouteTestRuntime(second, react)
	params, err = aicommon.BindAttachedBrowserToolParams(second, "browser.capability.call", nil)
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))
}

func TestReActBrowserSelectionPersistsAndCanBeReplaced(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)

	firstEvent := &ypb.AIInputEvent{
		FreeInput: "inspect A",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
		},
	}
	firstResources, firstControl := normalizeInputAttachments(firstEvent)
	first := react.buildReTaskFromEvent(firstEvent, firstResources, firstControl)
	react.prepareBrowserAttachmentForTask(first)

	followUpEvent := &ypb.AIInputEvent{FreeInput: "tell me my profile"}
	followUpResources, followUpControl := normalizeInputAttachments(followUpEvent)
	followUp := react.buildReTaskFromEvent(followUpEvent, followUpResources, followUpControl)
	require.Equal(t, []string{"A"}, browserReferences(browserTaskBindings(t, followUp)))
	react.prepareBrowserAttachmentForTask(followUp)

	switchedEvent := &ypb.AIInputEvent{
		FreeInput: "compare both accounts",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
			browserAttachment("device-b", "B"),
		},
	}
	switchedResources, switchedControl := normalizeInputAttachments(switchedEvent)
	switched := react.buildReTaskFromEvent(switchedEvent, switchedResources, switchedControl)
	require.Equal(t, []string{"A", "B"}, browserReferences(browserTaskBindings(t, switched)))
	react.prepareBrowserAttachmentForTask(switched)
	require.Equal(t, []string{"A", "B"}, browserReferences(react.GetActiveBrowserAttachments()))
	bindBrowserRouteTestRuntime(switched, react)

	allowed, feedback := aicommon.CheckAttachedBrowserToolRoute(switched, "browser.capability.call")
	require.True(t, allowed, feedback)
	params, err := aicommon.BindAttachedBrowserToolParams(switched, "browser.capability.call", aitool.InvokeParams{"browser_ref": "@B"})
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))
}

func TestPrepareBrowserAttachmentPromotesToolsAndAddsTimeline(t *testing.T) {
	browserToolNames := aicommon.AttachedBrowserBridgeToolNames()
	tools := make([]*aitool.Tool, 0, len(browserToolNames))
	for _, name := range browserToolNames {
		tools = append(tools, aitool.NewWithoutCallback(name, aitool.WithDescription(name)))
	}
	react := newBrowserAttachmentTestReAct(t, aicommon.WithTools(tools...))
	for _, name := range browserToolNames {
		react.config.GetAiToolManager().DisableTool(name)
	}

	task := aicommon.NewStatefulTaskBase("browser-init", "inspect A", context.Background(), react.Emitter, true)
	task.SetAttachedBrowserBindings([]aicommon.AttachedBrowserBinding{{
		DeviceID: "device-a", Name: "Chrome A", Reference: "A",
	}})
	react.SetCurrentTask(task)
	react.prepareBrowserAttachmentForTask(task)

	for _, name := range browserToolNames {
		require.True(t, react.config.GetAiToolManager().IsRecentlyUsedTool(name), name)
	}
	materials := aicommon.BuildPromptFrozenOpenMaterials(react.config)
	for _, name := range browserToolNames {
		require.Contains(t, materials.TimelineOpen, "## Tool: "+name)
	}
	output := react.config.Timeline.GetTimelineOutput()
	require.NotEmpty(t, output)
	rendered := output[len(output)-1].Content
	require.Contains(t, rendered, "## Attached Browser")
	require.Contains(t, rendered, "`A` (Chrome A)")
	require.NotContains(t, rendered, "device-a")
	require.Contains(t, rendered, "promoted for direct use")
}

func TestPrepareBrowserAttachmentReportsMissingBridgeTools(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)
	task := aicommon.NewStatefulTaskBase("browser-missing", "inspect A", context.Background(), react.Emitter, true)
	task.SetAttachedBrowserBindings([]aicommon.AttachedBrowserBinding{{
		DeviceID: "device-a", Reference: "A",
	}})
	react.SetCurrentTask(task)
	react.prepareBrowserAttachmentForTask(task)

	output := react.config.Timeline.GetTimelineOutput()
	require.NotEmpty(t, output)
	rendered := strings.Join(func() []string {
		parts := make([]string, 0, len(output))
		for _, item := range output {
			parts = append(parts, item.Content)
		}
		return parts
	}(), "\n")
	require.Contains(t, rendered, "Browser bridge is unavailable")
	require.Contains(t, rendered, "browser.capability.catalog")
	require.Contains(t, rendered, "instead of falling back to Rod")
}

func TestActivateBrowserAttachmentForResumedTaskKeepsExistingTimeline(t *testing.T) {
	browserToolNames := aicommon.AttachedBrowserBridgeToolNames()
	tools := make([]*aitool.Tool, 0, len(browserToolNames))
	for _, name := range browserToolNames {
		tools = append(tools, aitool.NewWithoutCallback(name))
	}
	react := newBrowserAttachmentTestReAct(t, aicommon.WithTools(tools...))
	for _, name := range browserToolNames {
		react.config.GetAiToolManager().DisableTool(name)
	}
	task := aicommon.NewStatefulTaskBase("browser-recovery", "resume", context.Background(), react.Emitter, true)
	task.SetAttachedBrowserBindings([]aicommon.AttachedBrowserBinding{{
		DeviceID: "device-a", Reference: "A",
	}})

	before := len(react.config.Timeline.GetTimelineOutput())
	react.activateBrowserAttachmentForResumedTask(task)
	require.Equal(t, before, len(react.config.Timeline.GetTimelineOutput()),
		"resuming an already initialized task must not inject the browser introduction twice")
	require.Equal(t, []string{"A"}, browserReferences(react.GetActiveBrowserAttachments()))
	for _, name := range browserToolNames {
		require.True(t, react.config.GetAiToolManager().IsRecentlyUsedTool(name), name)
	}
}
