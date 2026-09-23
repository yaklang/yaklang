package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSingleModelLoadingDoesNotBackfillUnusedTiers(t *testing.T) {
	saveAndRestore(t)
	ResetConfigLoaded()
	cfg := &ypb.AIGlobalConfig{SingleModelMode: true, IntelligentModels: []*ypb.AIModelConfig{{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: "ollama"}, ModelName: "sole"}}}
	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) { return cfg, nil })
	EnsureConfigLoaded()

	require.True(t, consts.IsSingleAIModelMode())
	require.True(t, IsTieredAIConfig(), "the intelligent model makes the global config available")
	require.Len(t, cfg.IntelligentModels, 1)
	require.Empty(t, cfg.LightweightModels)
	require.Empty(t, cfg.VisionModels)
	require.NoError(t, doVerifyAIConfig())
	require.Equal(t, "sole", GetGlobalManager().GetFirstConfig(consts.TierIntelligent).ModelName)
	require.Nil(t, GetGlobalManager().GetFirstConfig(consts.TierLightweight))
	require.Nil(t, GetGlobalManager().GetFirstConfig(consts.TierVision))
}

func TestSingleModelCorruptStoredConfigFailsClosed(t *testing.T) {
	saveAndRestore(t)
	ResetConfigLoaded()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true})
	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) {
		return &ypb.AIGlobalConfig{SingleModelMode: true}, nil
	})
	EnsureConfigLoaded()

	require.True(t, consts.IsSingleAIModelMode())
	require.ErrorIs(t, doVerifyAIConfig(), consts.ErrInvalidSingleAIModel)
}
