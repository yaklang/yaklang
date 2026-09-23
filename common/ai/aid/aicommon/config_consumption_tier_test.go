package aicommon

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

func TestConfig_AddTierConsumption(t *testing.T) {
	cfg := newConfig(context.Background())

	cfg.AddTierConsumption(consts.TierIntelligent, 10, 5)
	cfg.AddTierConsumption(consts.TierIntelligent, 3, 7)
	cfg.AddTierConsumption(consts.TierLightweight, 2, 1)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(13), snapshot[string(consts.TierIntelligent)]["input_consumption"])
	require.Equal(t, int64(12), snapshot[string(consts.TierIntelligent)]["output_consumption"])
	require.Equal(t, int64(2), snapshot[string(consts.TierLightweight)]["input_consumption"])
	require.Equal(t, int64(1), snapshot[string(consts.TierLightweight)]["output_consumption"])
}

func TestConfig_AddTierCacheHitToken(t *testing.T) {
	cfg := newConfig(context.Background())

	cfg.AddTierCacheHitToken(consts.TierIntelligent, 9)
	cfg.AddTierCacheHitToken(consts.TierIntelligent, 4)
	cfg.AddTierCacheHitToken(consts.TierLightweight, 2)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(13), snapshot[string(consts.TierIntelligent)]["cache_hit_token"])
	require.Equal(t, int64(2), snapshot[string(consts.TierLightweight)]["cache_hit_token"])
	require.Equal(t, int64(15), cfg.GetCacheHitToken())
}

func TestConfig_AddTierModelConsumption(t *testing.T) {
	cfg := newConfig(context.Background())

	cfg.AddTierModelConsumption(consts.TierLightweight, ModelConsumptionIdentity{
		ProviderType: "openai", ModelName: "gpt-5", ThinkingLevel: "none",
	}, 10, 5, 2)
	cfg.AddTierModelConsumption(consts.TierLightweight, ModelConsumptionIdentity{
		ProviderType: "openai", ModelName: "gpt-5", ThinkingLevel: "none",
	}, 3, 2, 1)
	cfg.AddTierModelConsumption(consts.TierLightweight, ModelConsumptionIdentity{
		ProviderType: "openai", ModelName: "gpt-5", ThinkingLevel: "high",
	}, 4, 3, 0)

	tierSnapshot := cfg.GetTierConsumptionSnapshot()[string(consts.TierLightweight)]
	require.Equal(t, int64(17), tierSnapshot["input_consumption"])
	require.Equal(t, int64(10), tierSnapshot["output_consumption"])
	require.Equal(t, int64(3), tierSnapshot["cache_hit_token"])

	models := cfg.GetTierModelConsumptionSnapshot()[string(consts.TierLightweight)]
	require.Len(t, models, 2)
	require.Equal(t, "high", models[0].ThinkingLevel)
	require.Equal(t, int64(4), models[0].InputConsumption)
	require.Equal(t, "none", models[1].ThinkingLevel)
	require.Equal(t, int64(13), models[1].InputConsumption)
	require.Equal(t, int64(7), models[1].OutputConsumption)
	require.Equal(t, int64(3), models[1].CacheHitToken)

	require.Equal(t, int64(17), cfg.GetInputConsumption())
	require.Equal(t, int64(10), cfg.GetOutputConsumption())
	require.Equal(t, int64(3), cfg.GetCacheHitToken())
}

func TestConsumptionPayloadSharesResolvedModeAndContainsNoProviderSecrets(t *testing.T) {
	parent := newConfig(context.Background())
	state := parent.ensureConsumptionState()
	state.InitializeEffectiveSingleModelMode(true)
	state.InitializeEffectiveSingleModelMode(false)

	child := newConfig(context.Background())
	for _, opt := range ConvertConfigToOptions(parent) {
		require.NoError(t, opt(child))
	}
	child.AddTierModelConsumption(consts.TierIntelligent, ModelConsumptionIdentity{
		ProviderType: "openai", ModelName: "gpt-5", ThinkingLevel: "",
	}, 2, 1, 0)

	payload := parent.BuildConsumptionPayload()
	require.Equal(t, true, payload["effective_single_model_mode"])
	modelSnapshot, ok := payload["tier_model_consumption"].(map[string][]ModelConsumptionSnapshot)
	require.True(t, ok)
	require.Equal(t, "auto", modelSnapshot[string(consts.TierIntelligent)][0].ThinkingLevel)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	serialized := strings.ToLower(string(raw))
	require.NotContains(t, serialized, "api_key")
	require.NotContains(t, serialized, "base_url")
	require.NotContains(t, serialized, "domain")
	require.NotContains(t, serialized, "headers")
}

func TestNewConfigInitializesEffectiveSingleModelModeForConsumption(t *testing.T) {
	cfg := NewConfig(context.Background(),
		WithSingleAIModelMode(true),
		WithFastAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) { return nil, nil }),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	)
	require.Equal(t, true, cfg.BuildConsumptionPayload()["effective_single_model_mode"])
}

func TestConvertConfigToOptions_PreserveTierConsumptionStats(t *testing.T) {
	parent := newConfig(context.Background())
	parent.AddTierConsumption(consts.TierIntelligent, 4, 6)

	child := newConfig(context.Background())
	opts := ConvertConfigToOptions(parent)
	for _, opt := range opts {
		require.NoError(t, opt(child))
	}

	require.Same(
		t,
		parent.InitStatus.GetOrCreateConsumptionState().GetTierConsumptionStats(),
		child.InitStatus.GetOrCreateConsumptionState().GetTierConsumptionStats(),
	)

	child.AddTierConsumption(consts.TierIntelligent, 1, 2)
	snapshot := parent.GetTierConsumptionSnapshot()
	require.Equal(t, int64(5), snapshot[string(consts.TierIntelligent)]["input_consumption"])
	require.Equal(t, int64(8), snapshot[string(consts.TierIntelligent)]["output_consumption"])
}

func TestWrapper_TracksOutputConsumptionByTier(t *testing.T) {
	cfg := newConfig(context.Background())
	cb := cfg.wrapper(func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		resp := i.NewAIResponse()
		resp.EmitOutputStream(strings.NewReader("hello"))
		resp.Close()
		return resp, nil
	}, consts.TierLightweight)

	req := NewAIRequest("ping")
	req.SetDetachCheckpoint(true)

	resp, err := cb(cfg, req)
	require.NoError(t, err)

	reasonReader, outputReader := resp.GetUnboundStreamReaderEx(nil, nil, nil)
	_, _ = io.ReadAll(reasonReader)
	_, _ = io.ReadAll(outputReader)

	require.Eventually(t, func() bool {
		snapshot := cfg.GetTierConsumptionSnapshot()
		return snapshot[string(consts.TierLightweight)]["output_consumption"] > 0
	}, time.Second, 20*time.Millisecond)
}

func TestWrapper_TracksCacheHitTokenByTier(t *testing.T) {
	cfg := newConfig(context.Background())
	rsp := NewUnboundAIResponse()
	rsp.SetModelInfo("openai", "gpt-5")
	rsp.SetThinkingLevel("none")
	rsp.totalOutputTokens.Store(9)
	rsp.SetUsageInfo(&aispec.ChatUsage{
		PromptTokens:     15,
		CompletionTokens: 7,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens: 12,
		},
	})

	cfg.finalizeTierConsumption(consts.TierLightweight, 20, rsp)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(3), snapshot[string(consts.TierLightweight)]["input_consumption"])
	require.Equal(t, int64(7), snapshot[string(consts.TierLightweight)]["output_consumption"])
	require.Equal(t, int64(12), snapshot[string(consts.TierLightweight)]["cache_hit_token"])
	require.Equal(t, int64(12), cfg.GetCacheHitToken())
	models := cfg.GetTierModelConsumptionSnapshot()[string(consts.TierLightweight)]
	require.Len(t, models, 1)
	require.Equal(t, "openai", models[0].ProviderType)
	require.Equal(t, "gpt-5", models[0].ModelName)
	require.Equal(t, "none", models[0].ThinkingLevel)
	require.Equal(t, int64(3), models[0].InputConsumption)
	require.Equal(t, int64(7), models[0].OutputConsumption)
	require.Equal(t, int64(12), models[0].CacheHitToken)
}

func TestAIChatCallbackCapturesFinalThinkingLevel(t *testing.T) {
	cfg := newConfig(context.Background())
	userModelInfo := make(chan string, 2)
	callback := AIChatToAICallbackType(func(_ string, opts ...aispec.AIConfigOption) (string, error) {
		resolved := aispec.NewDefaultAIConfig(opts...)
		if resolved.ModelInfoCallback != nil {
			resolved.ModelInfoCallback("openai", "gpt-5", resolved.ThinkingLevel)
		}
		if resolved.ModelInfoConfirmCallback != nil {
			resolved.ModelInfoConfirmCallback("openai", "gpt-5", resolved.ThinkingLevel)
		}
		if resolved.StreamHandler != nil {
			resolved.StreamHandler(strings.NewReader("ok"))
		}
		return "ok", nil
	})

	rsp, err := callback(cfg, NewAIRequest("test",
		WithAIRequest_ExtraSpecOpts(
			aispec.WithThinkingLevel("none"),
			aispec.WithModelInfoCallback(func(provider, model string) {
				userModelInfo <- provider + "/" + model
			}),
		),
	))
	require.NoError(t, err)
	_, output := rsp.GetUnboundStreamReaderEx(nil, nil, nil)
	_, err = io.ReadAll(output)
	require.NoError(t, err)
	require.Equal(t, "none", rsp.GetThinkingLevel())
	require.Equal(t, "openai", rsp.GetProviderName())
	require.Equal(t, "gpt-5", rsp.GetModelName())
	require.Equal(t, "openai/gpt-5", <-userModelInfo)
	select {
	case duplicate := <-userModelInfo:
		t.Fatalf("user model info callback invoked more than once: %s", duplicate)
	default:
	}
}
