package aireact

import (
	"context"
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

func TestBuildReTaskKeepsBrowserForUnifiedProcessing(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)
	event := &ypb.AIInputEvent{
		FreeInput: "inspect A",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
			{Type: aicommon.AttachedResourceTypeSelected, Key: aicommon.AttachedResourceKeyContent, Value: "selection"},
		},
	}
	resources, control := normalizeInputAttachments(event)
	task := react.buildReTaskFromEvent(event, resources, control)

	require.Len(t, task.GetAttachedDatas(), 2)
	require.True(t, task.GetAttachedDatas()[0].HasType(aicommon.AttachedResourceTypeBrowser))
	require.Empty(t, react.GetBrowserAttachments(),
		"building a task must not perform browser-specific attachment processing")
}

func TestReActBrowserAttachmentsAreSessionState(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)
	task := aicommon.NewStatefulTaskBase("browser-session", "inspect", context.Background(), react.Emitter, true)
	bindBrowserRouteTestRuntime(task, react)

	react.AddBrowserAttachments([]aicommon.AttachedBrowserBinding{{
		DeviceID: "device-a", Reference: "A",
	}})
	params, err := aicommon.BindAttachedBrowserToolParams(task, "browser.capability.call", nil)
	require.NoError(t, err)
	require.Equal(t, "device-a", params.GetString("device_id"))

	// Browser resources only add to the session; removal is a separate operation.
	react.AddBrowserAttachments([]aicommon.AttachedBrowserBinding{
		{DeviceID: "device-b", Reference: "B"},
	})
	params, err = aicommon.BindAttachedBrowserToolParams(task, "browser.capability.call", aitool.InvokeParams{"browser_ref": "@B"})
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))

	child := aicommon.NewSubTaskBase(task, "browser-child", "continue", true)
	bindBrowserRouteTestRuntime(child, react)
	params, err = aicommon.BindAttachedBrowserToolParams(child, "browser.capability.call", aitool.InvokeParams{"browser_ref": "A"})
	require.NoError(t, err)
	require.Equal(t, "device-a", params.GetString("device_id"))

	other := newBrowserAttachmentTestReAct(t)
	require.Empty(t, other.GetBrowserAttachments(), "browser state must not leak across ReAct sessions")
}

func TestReActBrowserAttachmentSessionAddsWithoutRemoving(t *testing.T) {
	react := newBrowserAttachmentTestReAct(t)
	react.AddBrowserAttachments([]aicommon.AttachedBrowserBinding{{DeviceID: "device-a", Reference: "A"}})
	require.Equal(t, []string{"A"}, browserReferences(react.GetBrowserAttachments()))

	react.AddBrowserAttachments([]aicommon.AttachedBrowserBinding{
		{DeviceID: "device-a", Name: "duplicate", Reference: "A2"},
		{DeviceID: "device-b", Reference: "B"},
	})
	require.Equal(t, []string{"A", "B"}, browserReferences(react.GetBrowserAttachments()))
}
