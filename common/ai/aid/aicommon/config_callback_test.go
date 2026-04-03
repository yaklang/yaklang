package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

func TestWithAICallback_WrapsTieredCallbacks(t *testing.T) {
	cfg := newConfig(context.Background())

	seen := make(map[string]AICallerConfigIf)
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		seen[req.GetPrompt()] = i
		return NewUnboundAIResponse(), nil
	}

	require.NoError(t, WithAICallback(cb)(cfg))
	require.NotNil(t, cfg.OriginalAICallback)
	require.NotNil(t, cfg.QualityPriorityAICallback)
	require.NotNil(t, cfg.SpeedPriorityAICallback)

	_, err := cfg.OriginalAICallback(cfg, NewAIRequest("original"))
	require.NoError(t, err)
	_, err = cfg.QualityPriorityAICallback(cfg, NewAIRequest("quality"))
	require.NoError(t, err)
	_, err = cfg.SpeedPriorityAICallback(cfg, NewAIRequest("speed"))
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
	cfg := newConfig(context.Background())

	var gotConfig AICallerConfigIf
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		gotConfig = i
		return NewUnboundAIResponse(), nil
	}

	require.NoError(t, WithFastAICallback(cb)(cfg))
	require.NotNil(t, cfg.OriginalAICallback)
	require.Nil(t, cfg.QualityPriorityAICallback)
	require.Nil(t, cfg.SpeedPriorityAICallback)

	_, err := cfg.OriginalAICallback(cfg, NewAIRequest("original"))
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

	cfg := newConfig(context.Background())

	seen := make(map[string]AICallerConfigIf)
	cb := func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		seen[req.GetPrompt()] = i
		return NewUnboundAIResponse(), nil
	}

	require.NoError(t, WithAutoTieredAICallback(cb)(cfg))
	require.NotNil(t, cfg.OriginalAICallback)
	require.NotNil(t, cfg.QualityPriorityAICallback)
	require.NotNil(t, cfg.SpeedPriorityAICallback)

	_, err := cfg.QualityPriorityAICallback(cfg, NewAIRequest("quality"))
	require.NoError(t, err)
	_, err = cfg.SpeedPriorityAICallback(cfg, NewAIRequest("speed"))
	require.NoError(t, err)

	qualityCfg, ok := seen["quality"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierIntelligent, qualityCfg.tier)

	speedCfg, ok := seen["speed"].(*tierAwareConsumptionCaller)
	require.True(t, ok)
	require.Equal(t, consts.TierLightweight, speedCfg.tier)
}
