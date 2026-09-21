package ai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type singleModelTestGateway struct {
	TestGateway
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
func (g *singleModelTestGateway) ExtractData(string, string, map[string]any) (map[string]any, error) {
	if err := g.observe(g.config); err != nil {
		return nil, err
	}
	return map[string]any{"model": g.config.Model}, nil
}
func (g *singleModelTestGateway) SupportedStructuredStream() bool { return true }
func (g *singleModelTestGateway) StructuredStream(string, ...any) (chan *aispec.StructuredData, error) {
	if err := g.observe(g.config); err != nil {
		return nil, err
	}
	ch := make(chan *aispec.StructuredData)
	close(ch)
	return ch, nil
}

// Direct gateway users have already chosen their configuration. Global or
// session policy must not reject, rewrite, or validate that choice here.
func TestSingleModelDoesNotInterceptGateway(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	calls := 0
	aispec.Register("direct-gateway-test", func() aispec.AIClient {
		return &singleModelTestGateway{observe: func(cfg *aispec.AIConfig) error {
			calls++
			require.Equal(t, "explicit-model", cfg.Model)
			require.Equal(t, "explicit-key", cfg.APIKey)
			return nil
		}}
	})
	ctx := context.Background()
	selected := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "direct-gateway-test", APIKey: "explicit-key"}, ModelName: "explicit-model"}
	for _, configured := range []bool{false, true} {
		global := &consts.TieredAIConfig{Enabled: true, SingleModelMode: true, LightweightConfigs: []*ypb.AIModelConfig{selected}}
		if configured {
			global.IntelligentConfigs = []*ypb.AIModelConfig{{Provider: &ypb.ThirdPartyApplicationConfig{Type: "other-provider"}, ModelName: "global-model"}}
		}
		consts.SetTieredAIConfig(global)
		opts := append(aispec.BuildOptionsFromConfig(selected), aispec.WithContext(ctx), aispec.WithFunctionCallRetryTimes(1))
		result, err := Chat("direct", opts...)
		require.NoError(t, err)
		require.Equal(t, "explicit-model", result)
		_, err = FunctionCall("direct", map[string]any{}, opts...)
		require.NoError(t, err)
		_, err = StructuredStream("direct", opts...)
		require.NoError(t, err)
		chatter, err := LoadChater("direct-gateway-test", opts...)
		require.NoError(t, err)
		_, err = chatter("direct")
		require.NoError(t, err)
		chatter, err = CreateChatterFromConfig(selected)
		require.NoError(t, err)
		_, err = chatter("direct", aispec.WithContext(ctx))
		require.NoError(t, err)
		_, err = LightweightChat("direct")
		require.NoError(t, err)
	}
	require.Equal(t, 12, calls)
}
