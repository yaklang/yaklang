package reactloops

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"testing"
)

func TestModelContextSettingOnlyControlsQualityLoop(t *testing.T) {
	model := &ypb.AIModelConfig{ModelName: uuid.NewString(), Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://example.test/v1", APIKey: "test-only"}}
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithPrimaryModelContext(model))
	quality, speed := makeSchemaStabilityTestLoop(cfg), makeSchemaStabilityTestLoop(cfg)
	speed.useSpeedPriorityAI = true
	registerModelContextTestConfig(t, model)
	key := aispec.ModelContextIdentity(model)
	require.EqualValues(t, 500000, quality.wholePromptTokenLimit())
	require.EqualValues(t, 300*1024, speed.wholePromptTokenLimit())
	require.NoError(t, yakit.SetAIModelContextSettings(consts.GetGormProfileDatabase(), "openai", model.ModelName, 800000, nil, key))
	require.EqualValues(t, 800000, quality.wholePromptTokenLimit())
	quality.syncPromptOverflowTimelineLimit()
	require.EqualValues(t, 800000, cfg.GetTimeline().GetTimelineContentLimit())
	speed.syncPromptOverflowTimelineLimit()
	require.EqualValues(t, 300*1024, cfg.GetTimeline().GetTimelineContentLimit())
	require.EqualValues(t, 300*1024, speed.wholePromptTokenLimit())
	require.EqualValues(t, 300*1024, cfg.GetTimelineContentSizeLimit())
	_, _, speedKey := speed.primaryModelContext()
	require.Empty(t, speedKey, "single-model mode must not opt the Speed loop into Quality settings")
	// An auxiliary error remains runtime-only, even for the same physical model.
	overflow := &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "openai", Model: model.ModelName, ContextKey: key, MaxContextTokens: 200000}}
	require.True(t, speed.reduceTimelineLimitForPromptOverflow(overflow, "prompt"))
	require.EqualValues(t, 800000, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "openai", model.ModelName, key).ThresholdTokens)
	require.False(t, cfg.ModelContextStatus(0).Pending)
}

func TestModelContextDisabledOnlyStopsMatchingQualityOverflow(t *testing.T) {
	model := &ypb.AIModelConfig{ModelName: uuid.NewString(), Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://example.test/v1", APIKey: "test-only"}}
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithPrimaryModelContext(model))
	registerModelContextTestConfig(t, model)
	key := aispec.ModelContextIdentity(model)
	disabled := true
	require.NoError(t, yakit.SetAIModelContextSettings(consts.GetGormProfileDatabase(), "openai", model.ModelName, 500000, &disabled, key))
	loop := makeSchemaStabilityTestLoop(cfg)
	overflow := &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "openai", Model: model.ModelName, ContextKey: key, MaxContextTokens: 200000}}
	require.True(t, loop.promptOverflowAutoAdjustDisabled(overflow))
	loop.useSpeedPriorityAI = true
	require.False(t, loop.promptOverflowAutoAdjustDisabled(overflow))
	loop.useSpeedPriorityAI = false
	overflow.ContextKey = "other-service"
	require.False(t, loop.promptOverflowAutoAdjustDisabled(overflow))
}

// Text compression already used Quality; native Speed compression already used
// Speed. This PR deliberately retains that existing protocol distinction.
func TestModelContextKeepsOriginalCompressionRouting(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			quality, speed := 0, 0
			callback := func(count *int) aicommon.AICallbackType {
				return func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					*count++
					return contextCompressionTestResponse(t, c, req, aicommon.ContextCompressionAction, map[string]any{"summary": "summary", "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}}, false), nil
				}
			}
			cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithQualityPriorityAICallback(callback(&quality)), aicommon.WithSpeedPriorityAICallback(callback(&speed)))
			// Keep the package test independent of aiforge's process registration;
			// execute the callback and response handler selected by the real scheduler.
			cfg.LiteForgeExecutor = func(prompt string, opts ...any) (*aicommon.ForgeResult, error) {
				var invoke *aicommon.LiteForgeInvokeRequest
				var configOpts []aicommon.ConfigOption
				for _, opt := range opts {
					switch v := opt.(type) {
					case *aicommon.LiteForgeInvokeRequest:
						invoke = v
					case aicommon.ConfigOption:
						configOpts = append(configOpts, v)
					}
				}
				require.NotNil(t, invoke)
				helper := aicommon.NewConfig(context.Background(), configOpts...)
				request := aicommon.NewAIRequest(prompt, aicommon.NewGeneralKVConfig(invoke.Options...).GetExtraRequestOpts()...)
				response, err := helper.CallAI(request)
				if err != nil {
					return nil, err
				}
				action, err := invoke.ResponseHandler(response)
				return &aicommon.ForgeResult{Action: action}, err
			}
			loop := makeSchemaStabilityTestLoop(cfg)
			loop.functionCallMode, loop.useSpeedPriorityAI = native, true
			loop.invoker = mock.NewMockInvoker(context.Background())
			loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
			_, _, err := loop.prepareLoopActionSchemas(&LoopActionHandlerOperator{})
			require.NoError(t, err)
			loop.promptMode = aicommon.LoopPromptModeCompressContext
			var streams sync.WaitGroup
			calls, _, _, err := loop.callAILoopTransaction(&streams, "STATIC_CONTEXT", "compression", &LoopActionHandlerOperator{}, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
			streams.Wait()
			require.NoError(t, err)
			require.Len(t, calls, 1)
			if native {
				require.Equal(t, 1, speed)
				require.Zero(t, quality)
			} else {
				require.Equal(t, 1, quality)
				require.Zero(t, speed)
			}
		})
	}
}

func registerModelContextTestConfig(t *testing.T, model *ypb.AIModelConfig) {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	cfg, err := yakit.GetAIGlobalConfig(db)
	require.NoError(t, err)
	if cfg == nil {
		cfg = &ypb.AIGlobalConfig{}
	}
	cfg.IntelligentModels = append(cfg.IntelligentModels, model)
	_, err = yakit.SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
}
