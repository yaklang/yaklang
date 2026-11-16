package reactloops

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type timelineWritingInvoker struct {
	*mock.MockInvoker
	browserBindings []aicommon.AttachedBrowserBinding
}

func (i *timelineWritingInvoker) AddToTimeline(entry, content string) {
	provider, ok := i.GetConfig().(interface {
		GetTimeline() *aicommon.Timeline
	})
	if !ok || provider.GetTimeline() == nil {
		return
	}
	provider.GetTimeline().PushText(i.GetConfig().AcquireId(), "[%s]:\n%s", entry, content)
}

func (i *timelineWritingInvoker) GetBrowserAttachments() []aicommon.AttachedBrowserBinding {
	return i.browserBindings
}

func (i *timelineWritingInvoker) SetBrowserAttachments(bindings []aicommon.AttachedBrowserBinding) {
	i.browserBindings = bindings
}

func TestTaskPreparationRunsBrowserResourceThroughCommonPipeline(t *testing.T) {
	ctx := context.Background()
	toolNames := aicommon.AttachedBrowserBridgeToolNames()
	tools := make([]*aitool.Tool, 0, len(toolNames))
	for _, name := range toolNames {
		tools = append(tools, aitool.NewWithoutCallback(name))
	}
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithTools(tools...),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithWorkdir(t.TempDir()),
	)
	for _, name := range toolNames {
		cfg.GetAiToolManager().DisableTool(name)
	}
	invoker := &timelineWritingInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("browser-common-prep", "inspect", ctx, cfg.GetEmitter())
	task.SetAttachedDatas([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(
			aicommon.AttachedResourceTypeBrowser,
			aicommon.AttachedResourceKeyBrowserDevice,
			`{"deviceId":"device-a","reference":"A"}`,
		),
		aicommon.NewAttachedResource(
			aicommon.AttachedResourceTypeBrowser,
			aicommon.AttachedResourceKeyBrowserDevice,
			`{"deviceId":"device-b","reference":"B"}`,
		),
	})
	loop.SetCurrentTask(task)
	loop.prepareTaskAttachedResources(task)

	require.Len(t, loop.GetAttachedResources(), 2)
	require.Equal(t, []aicommon.AttachedBrowserBinding{
		{DeviceID: "device-a", Reference: "A"},
		{DeviceID: "device-b", Reference: "B"},
	}, invoker.GetBrowserAttachments())
	for _, name := range toolNames {
		require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(name), name)
	}
	output := cfg.Timeline.GetTimelineOutput()
	require.NotEmpty(t, output)
	require.Contains(t, output[len(output)-1].Content, "## Attached Browser")
	require.Contains(t, output[len(output)-1].Content, "`A`")
	require.Contains(t, output[len(output)-1].Content, "`B`")
}

func TestAttachedFileContentPersistsAndPinSurvivesCompression(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithWorkdir(t.TempDir()),
	)
	invoker := &timelineWritingInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("attached-file-content", "inspect the attachment", ctx, cfg.GetEmitter())
	loop.SetCurrentTask(task)

	content := "INLINE_PREFIX\n" + strings.Repeat("ordinary attached content\n", 1000) + "ONLY_IN_FULL_FILE\n"
	resources := RunAttachedExtraResourcesInit(invoker, loop, []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(
			aicommon.AttachedResourceTypeFile,
			aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT,
			content,
		),
	})
	require.Len(t, resources, 1)
	fileResource, ok := resources[0].(*aicommon.AttachedFileContentResourceData)
	require.True(t, ok)
	rendered := fileResource.ToAttachData(loop)
	_, savedAfter, found := strings.Cut(rendered, "Full content saved to file: ")
	require.True(t, found)
	savedPath, _, _ := strings.Cut(savedAfter, "\n")
	require.NotEmpty(t, savedPath)
	t.Cleanup(func() { _ = os.Remove(savedPath) })
	persisted, err := os.ReadFile(savedPath)
	require.NoError(t, err)
	require.Equal(t, content, string(persisted))

	before := aicommon.RenderTimelineFrozenOpenWithOptions(cfg.Timeline, aicommon.TimelinePromptOptions{
		PromoteUserInput: true,
	})
	require.Contains(t, before.Open, "User pinned attached file content")
	require.Contains(t, before.Open, savedPath)
	require.Contains(t, before.Open, "INLINE_PREFIX")
	require.NotContains(t, before.Open, "ONLY_IN_FULL_FILE")

	throughID := before.ThroughID
	_, err = cfg.Timeline.ApplyCompressionSelection(&aicommon.TimelineCompressionSelection{
		ThroughID: &throughID,
		Summary:   "The attached file preview was compacted.",
	}, "")
	require.NoError(t, err)

	after := aicommon.RenderTimelineFrozenOpenWithOptions(cfg.Timeline, aicommon.TimelinePromptOptions{
		PromoteUserInput: true,
	})
	require.Contains(t, after.PromotedUserInputHistory, "User pinned attached file content")
	require.Contains(t, after.PromotedUserInputHistory, savedPath)
	require.NotContains(t, after.PromptText(), "INLINE_PREFIX")
	require.NotContains(t, after.PromptText(), "ONLY_IN_FULL_FILE")
}

func TestAttachedFilePathPinSurvivesCompression(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithWorkdir(t.TempDir()),
	)
	invoker := &timelineWritingInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("attached-file-path", "inspect the attachment", ctx, cfg.GetEmitter())
	loop.SetCurrentTask(task)

	path := t.TempDir() + "/report.txt"
	require.NoError(t, os.WriteFile(path, []byte("FILE_PATH_PREVIEW"), 0600))
	resources := RunAttachedExtraResourcesInit(invoker, loop, []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(
			aicommon.AttachedResourceTypeFile,
			aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
			path,
		),
	})
	require.Len(t, resources, 1)

	view := aicommon.RenderTimelineFrozenOpenWithOptions(cfg.Timeline, aicommon.TimelinePromptOptions{
		PromoteUserInput: true,
	})
	throughID := view.ThroughID
	_, err := cfg.Timeline.ApplyCompressionSelection(&aicommon.TimelineCompressionSelection{
		ThroughID: &throughID,
		Summary:   "The attached file preview was compacted.",
	}, "")
	require.NoError(t, err)

	view = aicommon.RenderTimelineFrozenOpenWithOptions(cfg.Timeline, aicommon.TimelinePromptOptions{
		PromoteUserInput: true,
	})
	require.Contains(t, view.PromotedUserInputHistory, "User pinned attached file `"+path+"`")
	require.NotContains(t, view.PromptText(), "FILE_PATH_PREVIEW")
}

func TestAttachedEmptyFileContentIsStillPersisted(t *testing.T) {
	ctx := context.Background()
	cfg := aicommon.NewConfig(
		ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithWorkdir(t.TempDir()),
	)
	invoker := &timelineWritingInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("empty-attached-file-content", "inspect the attachment", ctx, cfg.GetEmitter())
	loop.SetCurrentTask(task)

	resources := RunAttachedExtraResourcesInit(invoker, loop, []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(
			aicommon.AttachedResourceTypeFile,
			aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT,
			"",
		),
	})
	require.Len(t, resources, 1)
	fileResource, ok := resources[0].(*aicommon.AttachedFileContentResourceData)
	require.True(t, ok)
	rendered := fileResource.ToAttachData(loop)
	_, savedAfter, found := strings.Cut(rendered, "Full content saved to file: ")
	require.True(t, found)
	savedPath, _, _ := strings.Cut(savedAfter, "\n")
	require.NotEmpty(t, savedPath)
	t.Cleanup(func() { _ = os.Remove(savedPath) })
	persisted, err := os.ReadFile(savedPath)
	require.NoError(t, err)
	require.Empty(t, persisted)

	view := aicommon.RenderTimelineFrozenOpenWithOptions(cfg.Timeline, aicommon.TimelinePromptOptions{
		PromoteUserInput: true,
	})
	require.Contains(t, view.Open, "User pinned attached file content (`file_content`, 0 bytes)")
	require.Contains(t, view.Open, savedPath)
}
