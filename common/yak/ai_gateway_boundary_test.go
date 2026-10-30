package yak

import (
	"context"
	"encoding/json"
	"strings"
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
func (g *rawScriptGateway) GetConfig() *aispec.AIConfig { return g.config }
func (g *rawScriptGateway) CheckValid() error           { return nil }
func (g *rawScriptGateway) Chat(string, ...any) (string, error) {
	if g.config.ToolCallCallback != nil {
		data, _ := json.Marshal(map[string]any{"model": g.config.Model})
		g.config.ToolCallCallback([]*aispec.ToolCall{{ID: "output", Function: aispec.FuncReturn{Name: "object", Arguments: string(data)}}})
		g.config.ToolCallArgumentsStreamHandler(strings.NewReader(string(data)))
		g.config.FinishReasonCallback("tool_calls", nil)
		return "", nil
	}
	// Structured output can also use the text JSON protocol. Match the real
	// provider contract instead of returning a bare model name to its parser.
	data, _ := json.Marshal(map[string]any{"model": g.config.Model})
	if g.config.StreamHandler != nil {
		g.config.StreamHandler(strings.NewReader(string(data)))
	}
	return string(data), nil
}
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
assert json.loads(explicit).model == "explicit-model"
speed = ai.Chat("normal tier selection", ai.speedPriority())~
assert json.loads(speed).model == "speed-model"
named = ai.LightweightChat("unchanged entry")~
assert json.loads(named).model == "speed-model"
data = ai.FunctionCall("direct", {"model":"string"}, ai.type("script-raw-gateway"), ai.model("explicit-model"))~
assert data.model == "explicit-model"
`, nil)
		cancel()
		require.NoError(t, err, "scripts must not be intercepted by single-model Config policy")
	}
}
