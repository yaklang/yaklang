package yak

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type rawScriptGateway struct {
	aispec.AIClient
	config *aispec.AIConfig
}

func (g *rawScriptGateway) LoadOption(opts ...aispec.AIConfigOption) {
	g.config = aispec.NewDefaultAIConfig(opts...)
}
func (g *rawScriptGateway) GetConfig() *aispec.AIConfig         { return g.config }
func (g *rawScriptGateway) CheckValid() error                   { return nil }
func (g *rawScriptGateway) Chat(string, ...any) (string, error) { return g.config.Model, nil }
func (g *rawScriptGateway) ExtractData(string, string, map[string]any) (map[string]any, error) {
	return map[string]any{"model": g.config.Model}, nil
}

func TestScriptAIKeepsRawGatewayBehavior(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	aispec.Register("script-raw-gateway", func() aispec.AIClient { return &rawScriptGateway{} })
	model := func(name string) *ypb.AIModelConfig {
		return &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "script-raw-gateway"}, ModelName: name}
	}
	for _, hasPrimary := range []bool{false, true} {
		global := &consts.TieredAIConfig{Enabled: true, SingleModelMode: true,
			LightweightConfigs: []*ypb.AIModelConfig{model("speed-model")}}
		if hasPrimary {
			global.IntelligentConfigs = []*ypb.AIModelConfig{model("primary-model")}
		}
		consts.SetTieredAIConfig(global)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := NewScriptEngine(1).ExecuteExWithContext(ctx, `
explicit = ai.Chat("direct", ai.type("script-raw-gateway"), ai.model("explicit-model"))~
assert explicit == "explicit-model"
speed = ai.Chat("normal tier selection", ai.speedPriority())~
assert speed == "speed-model"
named = ai.LightweightChat("unchanged entry")~
assert named == "speed-model"
data = ai.FunctionCall("direct", {"model":"string"}, ai.type("script-raw-gateway"), ai.model("explicit-model"))~
assert data.model == "explicit-model"
`, nil)
		cancel()
		require.NoError(t, err, "scripts must not be intercepted by single-model Config policy")
	}
}
