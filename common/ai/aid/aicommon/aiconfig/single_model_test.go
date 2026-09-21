package aiconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type sessionRoutingGateway struct {
	aispec.AIClient
	config *aispec.AIConfig
}

func (g *sessionRoutingGateway) LoadOption(opts ...aispec.AIConfigOption) {
	g.config = aispec.NewDefaultAIConfig(opts...)
}
func (g *sessionRoutingGateway) CheckValid() error                   { return nil }
func (g *sessionRoutingGateway) GetConfig() *aispec.AIConfig         { return g.config }
func (g *sessionRoutingGateway) Chat(string, ...any) (string, error) { return g.config.Model, nil }

func TestSingleModelSessionDirectChat(t *testing.T) {
	EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	aispec.Register("config-session-test", func() aispec.AIClient { return &sessionRoutingGateway{} })
	model := func(name string) *ypb.AIModelConfig {
		return &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "config-session-test"}, ModelName: name}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true,
		IntelligentConfigs: []*ypb.AIModelConfig{model("quality")}, LightweightConfigs: []*ypb.AIModelConfig{model("speed")}})
	for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
		result, err := GetGlobalManager().NewChatCallback(tier, "", "", true)("test")
		require.NoError(t, err)
		require.Equal(t, "quality", result)
	}
	result, err := LightweightChat("test")
	require.NoError(t, err)
	require.Equal(t, "speed", result)
}

func TestSingleModelManagerGlobalAndSessionPolicy(t *testing.T) {
	EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	model := func(name string) *ypb.AIModelConfig {
		return &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "test"}, ModelName: name}
	}
	for _, global := range []bool{false, true} {
		stored := &consts.TieredAIConfig{Enabled: true, SingleModelMode: global,
			IntelligentConfigs: []*ypb.AIModelConfig{model("quality")},
			LightweightConfigs: []*ypb.AIModelConfig{model("speed")}, VisionConfigs: []*ypb.AIModelConfig{model("vision")}}
		consts.SetTieredAIConfig(stored)
		for _, local := range []bool{false, true} {
			mode := global || local
			for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
				selected, err := GetGlobalManager().BindModelConfig(tier, "", "", mode)()
				require.NoError(t, err)
				if global || local {
					require.Equal(t, "quality", selected.ModelName)
					selected.ModelName = "modified"
				} else {
					require.Equal(t, GetGlobalManager().GetFirstConfig(tier).ModelName, selected.ModelName)
				}
			}
			require.Equal(t, global, consts.IsSingleAIModelMode())
			require.Equal(t, "quality", stored.IntelligentConfigs[0].ModelName)
			require.Equal(t, "speed", stored.LightweightConfigs[0].ModelName)
		}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{LightweightConfigs: []*ypb.AIModelConfig{model("speed")}})
	_, err := GetGlobalManager().BindModelConfig(consts.TierLightweight, "", "", true)()
	require.ErrorIs(t, err, consts.ErrInvalidSingleAIModel)
}

func TestSingleModelLoadingDoesNotBackfillUnusedTiers(t *testing.T) {
	saveAndRestore(t)
	ResetConfigLoaded()
	cfg := &ypb.AIGlobalConfig{SingleModelMode: true, IntelligentModels: []*ypb.AIModelConfig{{
		Provider: &ypb.ThirdPartyApplicationConfig{Type: "ollama"}, ModelName: "sole"}}}
	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) { return cfg, nil })
	EnsureConfigLoaded()
	require.True(t, IsAIModelRoutingEnabled())
	require.False(t, IsTieredAIConfig(), "single-model mode is independent of the legacy Enabled switch")
	require.True(t, IsFallbackDisabled())
	require.Len(t, cfg.IntelligentModels, 1)
	require.Empty(t, cfg.LightweightModels)
	require.Empty(t, cfg.VisionModels)
	require.NoError(t, doVerifyAIConfig())
	for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
		model, err := GetGlobalManager().BindModelConfig(tier, "", "")()
		require.NoError(t, err)
		require.Equal(t, "sole", model.ModelName)
	}
	model, err := GetGlobalManager().BindModelConfig(consts.TierLightweight, "", "other")()
	require.NoError(t, err)
	require.Equal(t, "sole", model.ModelName)
	for _, policy := range []consts.RoutingPolicy{consts.PolicyPerformance, consts.PolicyCost, consts.PolicyBalance, consts.PolicyAuto} {
		model, err := GetModelByPolicy(policy)
		require.NoError(t, err)
		require.Equal(t, "sole", model.ModelName)
	}
}

func TestSingleModelCorruptStoredConfigFailsClosed(t *testing.T) {
	saveAndRestore(t)
	ResetConfigLoaded()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true})
	SetAIGlobalConfigGetter(func() (*ypb.AIGlobalConfig, error) { return &ypb.AIGlobalConfig{SingleModelMode: true}, nil })
	EnsureConfigLoaded()
	require.True(t, consts.IsSingleAIModelMode())
	_, err := GetGlobalManager().BindModelConfig(consts.TierLightweight, "", "")()
	require.ErrorIs(t, err, consts.ErrInvalidSingleAIModel)
	require.ErrorIs(t, doVerifyAIConfig(), consts.ErrInvalidSingleAIModel)
}
