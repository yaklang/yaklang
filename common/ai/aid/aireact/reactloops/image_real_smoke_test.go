package reactloops

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Optional external acceptance uses only the public free vision provider and a
// generated colour swatch, never user screenshots, credentials or model config.
func TestRealImageProviderSmoke(t *testing.T) {
	if os.Getenv("YAK_IMAGE_REAL_SMOKE") != "1" {
		t.Skip("opt-in external provider acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	public := &ypb.AIModelConfig{Provider: yakit.DefaultAIBalanceProviderConfig(), ModelName: "memfit-vision-free"}
	previous := consts.GetTieredAIConfig()
	cached := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous); yakit.SetCachedAIGlobalConfigForTest(cached) })
	consts.SetTieredAIConfig(&consts.TieredAIConfig{IntelligentConfigs: []*ypb.AIModelConfig{public}, LightweightConfigs: []*ypb.AIModelConfig{public}, VisionConfigs: []*ypb.AIModelConfig{public}})
	yakit.SetCachedAIGlobalConfigForTest(yakit.DefaultAIGlobalConfig())
	service, err := aicommon.LoadAIService("aibalance", append(aispec.BuildOptionsFromConfig(public), aispec.WithContext(ctx), aispec.WithTimeout(20), aispec.WithDisableProviderFallback(true), aispec.WithNetworkDebug(false))...)
	require.NoError(t, err)
	var calls atomic.Int32
	cb := func(c aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls.Add(1)
		return service(c, r)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 96, 96))
	draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, pixels))
	img, err := aimedia.FromBytes(ctx, buf.Bytes(), "image/png")
	require.NoError(t, err)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(cb), aicommon.WithImageInputMode("native", true))
	loop := makeSchemaStabilityTestLoop(cfg)
	cfg.Timeline.PushTextWithImages(cfg.AcquireId(), "[attached_image]: generated colour swatch", img)
	nativeStart := time.Now()
	snapshot, err := loop.prepareImages(ctx, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).Images)
	require.NoError(t, err)
	loop.imagePrompt = snapshot
	opts := append(loop.imageRequestOptions(), aicommon.WithAIRequest_Context(ctx), aicommon.WithAIRequest_ExtraSpecOpts(aispec.WithMaxTokens(64), aispec.WithEnableThinking(false)))
	resp, err := cb(cfg, aicommon.NewAIRequest("What solid colour is the rectangle? Reply with one English colour word."+snapshot.caption, opts...))
	require.NoError(t, err)
	answer, err := io.ReadAll(resp.GetOutputStreamReader("real-image-smoke", true, cfg.GetEmitter()))
	require.NoError(t, err)
	require.NoError(t, resp.GetError())
	require.Contains(t, strings.ToLower(string(answer)), "red")
	t.Logf("native acceptance: calls=%d latency=%s estimated_visual_tokens=%d", calls.Load(), time.Since(nativeStart), snapshot.imageTokens)
	if usage := resp.GetUsageInfo(); usage != nil {
		t.Logf("public vision acceptance: input=%d output=%d", usage.PromptTokens, usage.CompletionTokens)
	}
	describeStart := time.Now()
	require.NoError(t, aicommon.WithImageInputMode("describe")(cfg))
	require.NoError(t, cfg.RecordImageObservation(ctx, "smoke-swatch", img, "What solid colour is the rectangle?"))
	described, err := loop.prepareImages(ctx, aicommon.RenderTimelineFrozenOpen(cfg.Timeline).Images)
	require.NoError(t, err)
	require.Nil(t, described)
	textReq := aicommon.NewAIRequest("Based on the provided visual observation, what solid colour is the rectangle? Reply with one English colour word."+cfg.Timeline.DumpForPrompt(), aicommon.WithAIRequest_Context(ctx), aicommon.WithAIRequest_ExtraSpecOpts(aispec.WithMaxTokens(64), aispec.WithEnableThinking(false)))
	require.Empty(t, textReq.GetImageList())
	textResp, err := cb(cfg, textReq)
	require.NoError(t, err)
	answer, err = io.ReadAll(textResp.GetOutputStreamReader("real-description-smoke", true, cfg.GetEmitter()))
	require.NoError(t, err)
	require.NoError(t, textResp.GetError())
	require.Contains(t, strings.ToLower(string(answer)), "red")
	t.Logf("describe acceptance: additional_calls=%d latency=%s main_image_inputs=%d", calls.Load()-1, time.Since(describeStart), len(textReq.GetImageList()))
}
