package reactloops

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func imageFixture(t *testing.T, n int) *aimedia.Image {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, 1024, 512))
	pixels.Set(1, 1, color.RGBA{R: uint8(n + 1), A: 255})
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, pixels))
	img, err := aimedia.FromBytes(context.Background(), b.Bytes(), "image/png")
	require.NoError(t, err)
	return img
}

func imageTestLoop(t *testing.T, policy *ypb.AIImageInputConfig, options ...aicommon.ConfigOption) (*ReActLoop, *aicommon.Config) {
	t.Helper()
	ctx := context.Background()
	opts := []aicommon.ConfigOption{aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithImageInput(policy), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected main-model call")
		})}
	opts = append(opts, options...)
	cfg := aicommon.NewConfig(ctx, opts...)
	loop := makeSchemaStabilityTestLoop(cfg)
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	loop.invoker = inv
	loop.SetCurrentTask(mock.NewMockStatefulTask(ctx, "turn-1", "check if the dialog opened"))
	return loop, cfg
}

func buildImagePrompt(t *testing.T, loop *ReActLoop) *LoopPromptAssemblyResult {
	t.Helper()
	input := loop.captureLoopPromptBuildInput("image-test", "check if the dialog opened", "", nil, "", &LoopActionHandlerOperator{})
	result, err := loop.generateLoopPromptFromInput(input)
	require.NoError(t, err)
	loop.promptMode = result.Mode
	return result
}

func TestNativeImagesFollowTimelineNotResourceCatalogue(t *testing.T) {
	ctx := context.Background()
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native"})
	catalog, err := cfg.GetImageCatalog()
	require.NoError(t, err)
	archived, err := catalog.Register(ctx, "old-capture", imageFixture(t, 0))
	require.NoError(t, err)
	current, err := catalog.Register(ctx, "current-capture", imageFixture(t, 1))
	require.NoError(t, err)
	id := cfg.AcquireId()
	cfg.Timeline.PushTextWithImages(id, "[screenshot]: the current dialog", current.Image)
	result := buildImagePrompt(t, loop)
	require.Len(t, result.Images, 1)
	require.Equal(t, id, result.Images[0].TimelineItemID)
	require.Contains(t, result.Prompt, current.Image.ID)
	require.NotContains(t, result.Prompt, archived.Image.ID)
	req := aicommon.NewAIRequest(result.Prompt, loop.imageRequestOptions()...)
	require.Len(t, req.GetImageList(), 1)
	require.NoError(t, req.CheckImageBudget())
	cfg.Timeline.SoftDelete(id)
	result = buildImagePrompt(t, loop)
	require.Empty(t, result.Images)
	require.Empty(t, loop.imageRequestOptions())
	require.Len(t, catalog.Snapshot(), 2, "removing context must not delete retained resources")
}

func TestNativeImagesKeepOrderAndDoNotUseSeparateActiveSlots(t *testing.T) {
	ctx := context.Background()
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native", MaxActiveImages: 1})
	for n := 1; n <= 3; n++ {
		img := imageFixture(t, n)
		img.Source = aimedia.Source{Kind: "tool", ToolCallID: fmt.Sprintf("capture-%d", n)}
		cfg.Timeline.PushToolResult(&aitool.ToolResult{ID: cfg.AcquireId(), Name: "screenshot", ToolCallID: img.Source.ToolCallID, Success: true, Images: []*aimedia.Image{img}})
	}
	result := buildImagePrompt(t, loop)
	require.Len(t, loop.imagePrompt.inputs, 3)
	for i, ref := range result.Images {
		require.Equal(t, fmt.Sprintf("capture-%d", i+1), ref.Image.Source.ToolCallID)
	}
	first, err := loop.prepareImages(ctx, result.Images[:1])
	require.NoError(t, err)
	duplicate, err := loop.prepareImages(ctx, append(result.Images[:1:1], result.Images[0]))
	require.NoError(t, err)
	require.Len(t, duplicate.inputs, 1)
	require.Equal(t, first.imageTokens, duplicate.imageTokens)
}

func TestPassiveVisionWritesTimelineAndPromptBuildMakesNoModelCalls(t *testing.T) {
	ctx := context.Background()
	calls := 0
	vision := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls++
		require.Len(t, req.GetImageList(), 1)
		require.Contains(t, req.GetPrompt(), "check if the dialog opened")
		resp := aicommon.NewAIResponse(c)
		resp.EmitOutputStream(strings.NewReader("Visible fact: the settings dialog is open; save success is not visible."))
		resp.Close()
		return resp, nil
	}
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "auto"}, aicommon.WithVisionPriorityAICallback(vision))
	tool := &aitool.ToolResult{ID: cfg.AcquireId(), Name: "screenshot", ToolCallID: "screen-1", Success: true, Images: []*aimedia.Image{imageFixture(t, 1)}}
	cfg.Timeline.PushToolResult(tool)
	require.NoError(t, loop.ObserveToolImages(ctx, tool, "check if the dialog opened"))
	require.Equal(t, 1, calls)
	require.Contains(t, cfg.Timeline.DumpForPrompt(), "settings dialog is open")
	require.NoError(t, loop.ObserveToolImages(ctx, tool, "check if the dialog opened"))
	require.Equal(t, 1, calls, "replayed ingress is not analyzed twice")
	for range 2 {
		result := buildImagePrompt(t, loop)
		require.Contains(t, result.Prompt, "settings dialog is open")
		require.Empty(t, loop.imageRequestOptions())
	}
	require.Equal(t, 1, calls, "prompt assembly must never call vision")
}

func TestActiveVisionAnalyzesNewQuestionsAndNativeInspectCreatesContext(t *testing.T) {
	ctx := context.Background()
	calls := 0
	vision := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls++
		require.Contains(t, req.GetPrompt(), fmt.Sprintf("question-%d", calls))
		resp := aicommon.NewAIResponse(c)
		resp.EmitOutputStream(strings.NewReader(fmt.Sprintf("visible answer-%d", calls)))
		resp.Close()
		return resp, nil
	}
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native"}, aicommon.WithVisionPriorityAICallback(vision))
	catalog, err := cfg.GetImageCatalog()
	require.NoError(t, err)
	ref, err := catalog.Register(ctx, "screen", imageFixture(t, 2))
	require.NoError(t, err)
	for i := 1; i <= 2; i++ {
		action, err := aicommon.ExtractAction(fmt.Sprintf(`{"@action":"analyze_image","image_id":%q,"question":"question-%d"}`, ref.ID, i), "analyze_image")
		require.NoError(t, err)
		op := NewActionHandlerOperator(loop.GetCurrentTask())
		loopAction_AnalyzeImage.ActionHandler(loop, action, op)
		require.True(t, op.IsContinued())
		require.NotContains(t, op.GetFeedback().String(), "could not")
	}
	require.Equal(t, 2, calls)
	require.Contains(t, cfg.Timeline.DumpForPrompt(), "answer-1")
	require.Contains(t, cfg.Timeline.DumpForPrompt(), "answer-2")
	require.Empty(t, buildImagePrompt(t, loop).Images, "vision text does not attach pixels to the main model")
	action, err := aicommon.ExtractAction(fmt.Sprintf(`{"@action":"inspect_image","image_id":%q,"x":0,"y":0,"width":256,"height":128}`, ref.ID), "inspect_image")
	require.NoError(t, err)
	op := NewActionHandlerOperator(loop.GetCurrentTask())
	loopAction_InspectImage.ActionHandler(loop, action, op)
	require.NotContains(t, op.GetFeedback().String(), "could not")
	result := buildImagePrompt(t, loop)
	require.Len(t, result.Images, 1)
	require.Equal(t, 256, result.Images[0].Image.Width)
	require.Equal(t, ref.Image.ID, result.Images[0].Image.Source.OriginalImageID)
	require.Equal(t, 2, calls, "native inspect does not call vision")
}

func TestVisionFailureIsRecordedAsFailureNotSuccessfulObservation(t *testing.T) {
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "describe"}, aicommon.WithVisionPriorityAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, fmt.Errorf("vision unavailable")
	}))
	result := &aitool.ToolResult{ID: cfg.AcquireId(), Name: "screenshot", ToolCallID: "bad-screen", Success: true, Images: []*aimedia.Image{imageFixture(t, 1)}}
	cfg.Timeline.PushToolResult(result)
	require.ErrorContains(t, loop.ObserveToolImages(context.Background(), result, "check result"), "unavailable")
	prompt := buildImagePrompt(t, loop)
	require.Contains(t, prompt.Prompt, "NOT been understood")
	require.Contains(t, prompt.Prompt, "vision unavailable")
	require.Empty(t, loop.imageRequestOptions())
}

func TestImageCompressionUsesTimelineAndDoesNotResurrectSummarizedImages(t *testing.T) {
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native", VisualTokenLimit: 1})
	cfg.Timeline.PushTextWithImages(cfg.AcquireId(), "[screenshot]: inspect this image", imageFixture(t, 3))
	loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
	result := buildImagePrompt(t, loop)
	require.Equal(t, aicommon.LoopPromptModeCompressContext, result.Mode)
	require.NotEmpty(t, loop.imageRequestOptions(), "multimodal compression can see the evidence it summarizes")
	_, err := cfg.Timeline.ApplyCompressionSelection(&aicommon.TimelineCompressionSelection{Summary: "The screenshot showed the dialog; source image is retained as a resource."}, "")
	require.NoError(t, err)
	result = buildImagePrompt(t, loop)
	require.Equal(t, aicommon.LoopPromptModeNormal, result.Mode)
	require.Empty(t, result.Images)
	require.Empty(t, loop.imageRequestOptions())
	require.Contains(t, result.Prompt, "screenshot showed the dialog")
}

func TestImageAttachmentPersistsResourceAndReportsMissingOriginal(t *testing.T) {
	ctx := context.Background()
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native", MaxLongEdge: 512})
	img := imageFixture(t, 1)
	raw, err := img.Bytes(ctx)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "upload.png")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	require.NoError(t, loop.AddImageAttachment(ctx, path))
	require.NoError(t, os.Remove(path))
	result := buildImagePrompt(t, loop)
	require.Len(t, result.Images, 1)
	decoded, _, err := image.Decode(bytes.NewReader(loop.imagePrompt.inputs[0].Data))
	require.NoError(t, err)
	require.Equal(t, 512, decoded.Bounds().Dx())
	catalog, err := cfg.GetImageCatalog()
	require.NoError(t, err)
	require.Equal(t, 1024, catalog.Snapshot()[0].Image.Width)
	require.NoError(t, os.Remove(catalog.Snapshot()[0].Image.Path))
	_, err = loop.prepareImages(ctx, result.Images)
	require.ErrorContains(t, err, "cannot load image")
}

func TestBothActionProtocolsCarryRequestLocalImages(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			loop := newCallAILoopTransactionTestLoop(t, native, func(req *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				require.Len(t, req.GetImageList(), 1)
				require.Equal(t, []byte("request-image-bytes"), req.GetImageList()[0].Data)
				resp := aicommon.NewAIResponse(nil)
				if native {
					cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: "observe-1", Type: "function", Function: aispec.FuncReturn{Name: "observe", Arguments: `{}`}}})
					cfg.FinishReasonCallback("tool_calls", nil)
				} else {
					resp.EmitOutputStream(strings.NewReader(`{"@action":"observe"}`))
				}
				resp.Close()
				return resp, nil
			})
			loop.imagePrompt = &imagePromptSnapshot{inputs: []*aicommon.ImageData{{Data: []byte("request-image-bytes")}}, inputLimit: 100000, maxBytes: 1000000, reserve: 1000}
			loop.actions.Set("observe", &LoopAction{ActionType: "observe"})
			calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil, func(a, b io.Reader) { _, _ = io.Copy(io.Discard, a); _, _ = io.Copy(io.Discard, b) }, func(_, _ string, a, b io.Reader) { _, _ = io.Copy(io.Discard, a); _, _ = io.Copy(io.Discard, b) })
			require.NoError(t, err)
			require.Len(t, calls, 1)
		})
	}
}

func TestImageAutoFallbackRecordsVisionInTimelineOnlyForExplicitRejection(t *testing.T) {
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "auto", MainModelSupportsImages: proto.Bool(true)}, aicommon.WithVisionPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		resp := aicommon.NewAIResponse(c)
		resp.EmitOutputStream(strings.NewReader("Visible fallback observation."))
		resp.Close()
		return resp, nil
	}))
	cfg.Timeline.PushTextWithImages(cfg.AcquireId(), "[screenshot]:", imageFixture(t, 1))
	buildImagePrompt(t, loop)
	require.False(t, loop.fallbackImageRoute(fmt.Errorf("AI provider returned HTTP 401: does not support image input")))
	require.False(t, loop.fallbackImageRoute(fmt.Errorf("AI provider returned HTTP 400: bad argument")))
	require.True(t, loop.fallbackImageRoute(fmt.Errorf("AI provider returned HTTP 400: this model does not support image input")))
	result := buildImagePrompt(t, loop)
	require.Contains(t, result.Prompt, "Visible fallback observation")
	require.Empty(t, loop.imageRequestOptions())
	require.NoError(t, aicommon.WithImageInputMode("native")(cfg))
	buildImagePrompt(t, loop)
	require.NotEmpty(t, loop.imageRequestOptions())
}

type imageAppendDuringAssemblyInvoker struct {
	*mock.MockInvoker
	config *aicommon.Config
	image  *aimedia.Image
}

func (i *imageAppendDuringAssemblyInvoker) AssembleLoopPrompt(tools []*aitool.Tool, input *aicommon.LoopPromptAssemblyInput) (*aicommon.LoopPromptAssemblyResult, error) {
	result, err := i.MockInvoker.AssembleLoopPrompt(tools, input)
	if err == nil {
		i.config.Timeline.PushTextWithImages(i.config.AcquireId(), "[screenshot]: late tool result", i.image)
	}
	return result, err
}

func TestNativeImagesAreCapturedWithTextBeforeLaterToolCompletions(t *testing.T) {
	loop, cfg := imageTestLoop(t, &ypb.AIImageInputConfig{Mode: "native"})
	first, later := imageFixture(t, 1), imageFixture(t, 2)
	cfg.Timeline.PushTextWithImages(cfg.AcquireId(), "[screenshot]: first result", first)
	inv := mock.NewMockInvoker(context.Background())
	inv.SetConfig(cfg)
	loop.invoker = &imageAppendDuringAssemblyInvoker{MockInvoker: inv, config: cfg, image: later}
	result := buildImagePrompt(t, loop)
	require.Len(t, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).Images, 2)
	require.Len(t, result.Images, 1)
	require.Len(t, loop.imagePrompt.inputs, 1)
	require.Equal(t, first.ID, result.Images[0].Image.ID)
	require.NotContains(t, result.Prompt, later.ID)
}
