package aiconfig

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type singleModelTestGateway struct {
	sessionRoutingGateway
	observe func(*aispec.AIConfig) error
}

func (g *singleModelTestGateway) Chat(string, ...any) (string, error) {
	if err := g.observe(g.config); err != nil {
		return "", err
	}
	if g.config.StreamHandler != nil {
		g.config.StreamHandler(nil)
	}
	return g.config.Model, nil
}
func TestBoundSingleModelCallbacks(t *testing.T) {
	EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	var calls, alternatives int
	var observed []*aispec.AIConfig
	failure := false
	aispec.Register("single-route-test", func() aispec.AIClient {
		return &singleModelTestGateway{observe: func(cfg *aispec.AIConfig) error {
			calls++
			observed = append(observed, cfg)
			if failure {
				return errors.New("provider unavailable")
			}
			if len(cfg.Images) > 0 {
				return errors.New("model does not support images")
			}
			return nil
		}}
	})
	aispec.Register("single-alternative-test", func() aispec.AIClient {
		return &singleModelTestGateway{observe: func(*aispec.AIConfig) error { alternatives++; return nil }}
	})
	sole := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "single-route-test"}, ModelName: "sole-model"}
	other := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "single-alternative-test", APIKey: "must-not-leak"}, ModelName: "other-model"}
	inactiveSameProvider := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "single-route-test", APIKey: "inactive-key-must-not-leak"}, ModelName: "inactive-model"}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, SingleModelMode: true,
		IntelligentConfigs: []*ypb.AIModelConfig{sole, inactiveSameProvider}, LightweightConfigs: []*ypb.AIModelConfig{other}, VisionConfigs: []*ypb.AIModelConfig{other}})
	ctx := context.Background()
	streamed := 0
	opts := []aispec.AIConfigOption{aispec.WithContext(ctx), aispec.WithThinkingLevel("high"), aispec.WithFunctionCallRetryTimes(1),
		aispec.WithStreamHandler(func(io.Reader) { streamed++ })}
	for _, tier := range []consts.ModelTier{consts.TierIntelligent, consts.TierLightweight, consts.TierVision} {
		_, err := GetGlobalManager().NewChatCallback(tier, "", "")("test", opts...)
		require.NoError(t, err)
	}
	_, err := GetGlobalManager().NewChatCallback(consts.TierLightweight, "", "")("test", opts...)
	require.NoError(t, err)
	require.Equal(t, 4, calls)
	require.Equal(t, 4, streamed)
	for index, cfg := range observed {
		require.Equal(t, "sole-model", cfg.Model)
		require.Equal(t, "single-route-test", cfg.Type)
		require.Equal(t, []string{"high", "none", "high", "none"}[index], cfg.ThinkingLevel)
		require.Equal(t, ctx, cfg.Context)
		require.Empty(t, cfg.APIKey)
		require.True(t, cfg.DisableProviderFallback)
	}
	for _, selector := range []aispec.AIConfigOption{aispec.WithModel("other-model"), aispec.WithType("single-alternative-test")} {
		result, err := GetGlobalManager().NewChatCallback(consts.TierLightweight, "", "")("test", selector)
		require.NoError(t, err)
		require.Equal(t, "sole-model", result)
	}
	failure = true
	_, err = GetGlobalManager().NewChatCallback(consts.TierLightweight, "", "")("test")
	require.ErrorContains(t, err, "provider unavailable")
	failure = false
	_, err = GetGlobalManager().NewChatCallback(consts.TierVision, "", "")("image", func(cfg *aispec.AIConfig) { cfg.Images = []*aispec.ImageDescription{{}} })
	require.ErrorContains(t, err, "does not support images")
	require.Zero(t, alternatives, "errors and unsupported vision must never change model")
	consts.SetTieredAIConfig(&consts.TieredAIConfig{SingleModelMode: true})
	before := calls
	_, err = GetGlobalManager().NewChatCallback(consts.TierLightweight, "", "")("test")
	require.ErrorIs(t, err, consts.ErrInvalidSingleAIModel)
	require.Equal(t, before, calls)
	// Disabling single-model routing restores the untouched tier configuration.
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, LightweightConfigs: []*ypb.AIModelConfig{other}})
	_, err = LightweightChat("test")
	require.NoError(t, err)
	require.Equal(t, 1, alternatives)
}

func TestSingleModelManagerSelectionIsFinal(t *testing.T) {
	EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	aispec.Register("manager-selection-test", func() aispec.AIClient { return &sessionRoutingGateway{} })
	model := func(name string) *ypb.AIModelConfig {
		return &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "manager-selection-test"}, ModelName: name}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{SingleModelMode: true, IntelligentConfigs: []*ypb.AIModelConfig{model("before")}})
	opts, err := GetGlobalManager().BindRequestOptions(consts.TierLightweight, "", "")(aispec.WithSpeedPriority())
	require.NoError(t, err)
	cb, err := ai.CreateChatterFromConfig(model("before"))
	require.NoError(t, err)
	consts.SetTieredAIConfig(&consts.TieredAIConfig{SingleModelMode: true, IntelligentConfigs: []*ypb.AIModelConfig{model("after")}})
	result, err := ai.Chat("resolved", opts...)
	require.NoError(t, err)
	require.Equal(t, "before", result)
	result, err = cb("fixed callback")
	require.NoError(t, err)
	require.Equal(t, "before", result)
	result, err = GetGlobalManager().NewChatCallback(consts.TierIntelligent, "", "")("next manager selection")
	require.NoError(t, err)
	require.Equal(t, "after", result)
}
