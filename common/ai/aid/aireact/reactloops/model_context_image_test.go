package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// Image preparation happens after the iteration captures its quality model.
// A global switch in that gap must not replace its image capabilities, estimator,
// transport model, or context budget, including during an overflow rebuild.
func TestModelContextImagePreparationUsesIterationSnapshot(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, supportsB := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/nextSupportsImages=%v", native, supportsB), func(t *testing.T) {
				yakit.GuardConfigurationForTest(t)
				model := func(endpoint string, supports bool) *ypb.AIModelConfig {
					return &ypb.AIModelConfig{
						ModelName: "same-model", SupportsImageInput: proto.Bool(supports),
						SupportsImageToolCalling: proto.Bool(supports), ImageTokenEstimator: "patch32",
						Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: endpoint},
					}
				}
				a, b := model("https://a.example.test/v1", true), model("https://b.example.test/v1", supportsB)
				b.ImageTokenEstimator = "tile512"
				registerModelContextTestConfig(t, a)
				registerModelContextTestConfig(t, b)
				aKey, bKey := aispec.ModelContextIdentity(a), aispec.ModelContextIdentity(b)
				selectContextTestModel(a)
				cfg := aicommon.NewConfig(context.Background(),
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true),
					aicommon.WithDisallowMCPServers(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithNoOpMemoryTriage(), aicommon.WithTieredAICallback(false),
					aicommon.WithImageInput(&ypb.AIImageInputConfig{Mode: "auto"}))
				loop := makeSchemaStabilityTestLoop(cfg)
				t.Cleanup(loop.Release)
				invoker := mock.NewMockInvoker(cfg.GetContext())
				invoker.SetConfig(cfg)
				loop.invoker, loop.functionCallMode = invoker, native
				loop.SetCurrentTask(cfg.DefaultTask)
				cfg.Timeline.PushTextWithImages(cfg.AcquireId(), "current screenshot", imageFixture(t, 1))
				setContextTestThreshold(t, cfg, aKey, 400000)
				input := loop.captureLoopPromptBuildInput("image-binding", "inspect screenshot", "", nil, "", &LoopActionHandlerOperator{})

				selectContextTestModel(b)
				setContextTestThreshold(t, cfg, bKey, 300000)
				check := func(wantKey string, wantLimit int64) *LoopPromptAssemblyResult {
					result, err := loop.generateLoopPromptFromInput(input)
					require.NoError(t, err)
					require.NotNil(t, loop.imagePrompt, "image capability must come from the captured request model")
					require.Equal(t, wantKey, aispec.ModelContextIdentity(loop.imagePrompt.model))
					require.NotNil(t, input.aiBinding)
					require.Equal(t, input.aiBinding.ModelConfig.GetImageTokenEstimator(), loop.imagePrompt.model.GetImageTokenEstimator())
					inputLimit := wantLimit
					if result.Mode == aicommon.LoopPromptModeCompressContext {
						// Keep the existing compression-request ceiling separate from
						// the model-specific threshold that triggers compression.
						inputLimit = aicommon.TimelineCompressionMaxInputTokens
					}
					require.Equal(t, inputLimit, loop.imagePrompt.inputLimit)
					require.Equal(t, wantLimit, loop.wholePromptTokenLimit())
					req := aicommon.NewAIRequest(result.Prompt, loop.imageRequestOptions()...)
					require.Equal(t, wantKey, aispec.ModelContextIdentity(req.GetImageModel()))
					require.NotEmpty(t, req.GetImageList())
					require.NoError(t, req.CheckImageBudget())
					return result
				}
				first := check(aKey, 400000)
				overflow := &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{
					Provider: "openai", Model: a.ModelName, ContextKey: aKey, MaxContextTokens: 200000,
				}}
				require.True(t, loop.reduceTimelineLimitForPromptOverflow(overflow, first.Prompt))
				input.overflowCompression = true
				check(aKey, 160000)

				input = loop.captureLoopPromptBuildInput("next-image-binding", "inspect screenshot", "", nil, "", &LoopActionHandlerOperator{})
				if supportsB {
					check(bKey, 300000)
				} else {
					_, err := loop.generateLoopPromptFromInput(input)
					require.NoError(t, err)
					require.Nil(t, loop.imagePrompt)
					require.EqualValues(t, 300000, loop.wholePromptTokenLimit())
				}
			})
		}
	}
}
