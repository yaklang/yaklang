package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestWithAICallback_WrapsTieredCallbacks(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	seen := make(map[string]AICallerConfigIf)
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		seen[req.GetPrompt()] = i
		rsp := NewAIResponse(i)
		rsp.Close()
		return rsp, nil
	}

	require.NoError(t, WithAICallback(cb)(cfg))
	require.NotNil(t, cfg.GetOriginalAICallback())
	require.NotNil(t, cfg.GetQualityPriorityAICallback())
	require.NotNil(t, cfg.GetSpeedPriorityAICallback())

	_, err := cfg.GetOriginalAICallback()(cfg, NewAIRequest("original"))
	require.NoError(t, err)
	_, err = cfg.GetQualityPriorityAICallback()(cfg, NewAIRequest("quality"))
	require.NoError(t, err)
	_, err = cfg.GetSpeedPriorityAICallback()(cfg, NewAIRequest("speed"))
	require.NoError(t, err)

	origCfg, ok := seen["original"].(*Config)
	require.True(t, ok)
	require.Same(t, cfg, origCfg)

	qualityCfg, ok := seen["quality"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierIntelligent, qualityCfg.tier)

	speedCfg, ok := seen["speed"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierLightweight, speedCfg.tier)
}

func TestWithAICallback_WithAICallbackNegative(t *testing.T) {
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyPerformance,
	})
	seen := make(map[string]AICallerConfigIf)
	cfg := NewConfig(context.Background(), WithDisableDynamicPlanning(true),
		WithAICallback(func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			seen[req.GetPrompt()] = i
			rsp := NewAIResponse(i)
			rsp.Close()
			return rsp, nil
		}))

	require.NotNil(t, cfg.GetOriginalAICallback())
	require.NotNil(t, cfg.GetQualityPriorityAICallback())
	require.NotNil(t, cfg.GetSpeedPriorityAICallback())

	_, err := cfg.GetOriginalAICallback()(cfg, NewAIRequest("original"))
	require.NoError(t, err)
	_, err = cfg.GetQualityPriorityAICallback()(cfg, NewAIRequest("quality"))
	require.NoError(t, err)
	_, err = cfg.GetSpeedPriorityAICallback()(cfg, NewAIRequest("speed"))
	require.NoError(t, err)

	origCfg, ok := seen["original"].(*Config)
	require.True(t, ok)
	require.Same(t, cfg, origCfg)

	qualityCfg, ok := seen["quality"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierIntelligent, qualityCfg.tier)

	speedCfg, ok := seen["speed"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierLightweight, speedCfg.tier)
}

func TestWithFastAICallback_OnlySetsOriginal(t *testing.T) {
	cfg := NewTestConfig(context.Background())

	var gotConfig AICallerConfigIf
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		gotConfig = i
		rsp := NewAIResponse(gotConfig)
		rsp.Close()
		return rsp, nil
	}

	require.NoError(t, WithFastAICallback(cb)(cfg))
	require.NotNil(t, cfg.GetOriginalAICallback())
	require.Nil(t, cfg.GetQualityPriorityAICallback())
	require.Nil(t, cfg.GetSpeedPriorityAICallback())

	_, err := cfg.GetOriginalAICallback()(cfg, NewAIRequest("original"))
	require.NoError(t, err)

	_, ok := gotConfig.(*Config)
	require.True(t, ok)
}

func TestWithAutoTieredAICallback_FallbackWhenTieredDisabled(t *testing.T) {
	originalTiered := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(nil)
	t.Cleanup(func() {
		consts.SetTieredAIConfig(originalTiered)
	})

	cfg := NewTestConfig(context.Background())

	seen := make(map[string]AICallerConfigIf)
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		seen[req.GetPrompt()] = i
		rsp := NewAIResponse(i)
		rsp.Close()
		return rsp, nil
	}

	require.NoError(t, WithAutoTieredAICallback(cb)(cfg))
	require.NotNil(t, cfg.GetOriginalAICallback())
	require.NotNil(t, cfg.GetQualityPriorityAICallback())
	require.NotNil(t, cfg.GetSpeedPriorityAICallback())

	_, err := cfg.GetQualityPriorityAICallback()(cfg, NewAIRequest("quality"))
	require.NoError(t, err)
	_, err = cfg.GetSpeedPriorityAICallback()(cfg, NewAIRequest("speed"))
	require.NoError(t, err)

	qualityCfg, ok := seen["quality"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierIntelligent, qualityCfg.tier)

	speedCfg, ok := seen["speed"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierLightweight, speedCfg.tier)
}
