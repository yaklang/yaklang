package reactloops

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type timelineWritingInvoker struct {
	*mock.MockInvoker
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
