package aicommon

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type tieredSingleModeGateway struct {
	aispec.AIClient
	config  *aispec.AIConfig
	observe func(*aispec.AIConfig)
}

func (g *tieredSingleModeGateway) LoadOption(opts ...aispec.AIConfigOption) {
	g.config = aispec.NewDefaultAIConfig(opts...)
}
func (g *tieredSingleModeGateway) GetConfig() *aispec.AIConfig { return g.config }
func (g *tieredSingleModeGateway) CheckValid() error           { return nil }
func (g *tieredSingleModeGateway) Chat(string, ...any) (string, error) {
	g.observe(g.config)
	if g.config.StreamHandler != nil {
		g.config.StreamHandler(strings.NewReader("ok"))
	}
	return "ok", nil
}

func TestWithTieredAICallbackSingleModelOption(t *testing.T) {
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })

	var observed []*aispec.AIConfig
	aispec.Register("tiered-single-mode-test", func() aispec.AIClient {
		return &tieredSingleModeGateway{observe: func(cfg *aispec.AIConfig) {
			observed = append(observed, cfg)
		}}
	})
	model := func(name string) *ypb.AIModelConfig {
		level := "high"
		return &ypb.AIModelConfig{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "tiered-single-mode-test", ReasoningEffort: &level},
			ModelName: name,
		}
	}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:            true,
		IntelligentConfigs: []*ypb.AIModelConfig{model("quality")},
		LightweightConfigs: []*ypb.AIModelConfig{model("speed")},
		VisionConfigs:      []*ypb.AIModelConfig{model("vision")},
	})

	invokeSpeed := func(cfg *Config) *aispec.AIConfig {
		response, err := cfg.CallSpeedPriorityAI(NewAIRequest("speed"))
		require.NoError(t, err)
		_, err = io.ReadAll(response.GetOutputStreamReader("test", true, cfg.GetEmitter()))
		require.NoError(t, err)
		require.NoError(t, response.GetError())
		return observed[len(observed)-1]
	}

	normal := NewConfig(context.Background(),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
		WithTieredAICallback(false),
	)
	got := invokeSpeed(normal)
	require.Equal(t, "speed", got.Model)
	require.Equal(t, "high", got.ThinkingLevel)

	// The explicit optional value makes callback initialization independent of
	// where the scheduling-only Config option appears.
	single := NewConfig(context.Background(),
		WithTieredAICallback(true),
		WithSingleAIModelMode(true),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	)
	got = invokeSpeed(single)
	require.Equal(t, "quality", got.Model)
	require.Equal(t, "none", got.ThinkingLevel)
	require.False(t, single.ResolveAuxiliaryTask(CallerLabelTaskShortID).ShouldRun())

	// Without an explicit override, the callback option reads the Config/global
	// mode at the point where that option is applied.
	globalBefore := consts.GetTieredAIConfig()
	globalBefore.SingleModelMode = true
	consts.SetTieredAIConfig(globalBefore)
	globalSingle := NewConfig(context.Background(),
		WithTieredAICallback(),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	)
	got = invokeSpeed(globalSingle)
	require.Equal(t, "quality", got.Model)
	require.Equal(t, "none", got.ThinkingLevel)
}

func TestCustomSpeedCallbackKeepsOptionSemantics(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	consts.SetTieredAIConfig(nil)

	var thinking string
	custom := func(caller AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		var config aispec.AIConfig
		for _, opt := range req.GetExtraSpecOpts() {
			opt(&config)
		}
		thinking = config.ThinkingLevel
		response := caller.NewAIResponse()
		response.EmitOutputStream(strings.NewReader("ok"))
		response.Close()
		return response, nil
	}
	cfg := NewConfig(context.Background(),
		WithSingleAIModelMode(true),
		WithSpeedPriorityAICallback(custom),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	)
	response, err := cfg.CallSpeedPriorityAI(NewAIRequest(
		"custom",
		WithAIRequest_ExtraSpecOpts(aispec.WithThinkingLevel("high")),
	))
	require.NoError(t, err)
	_, err = io.ReadAll(response.GetOutputStreamReader("test", true, cfg.GetEmitter()))
	require.NoError(t, err)
	require.Equal(t, "high", thinking, "custom callback must follow option configuration unchanged")
}

func TestSingleModelSchedulingOptionInheritedByChild(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	consts.SetTieredAIConfig(nil)

	parent := NewConfig(context.Background(),
		WithSingleAIModelMode(true),
		WithFastAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
			return nil, nil
		}),
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	)
	child := NewConfig(context.Background(),
		append(ConvertConfigToOptions(parent),
			WithAICallbacks(parent.GetRawAICallbacks()),
			WithDisableCreateDBRuntime(true),
		)...,
	)
	require.True(t, child.IsSingleAIModelMode())
	require.False(t, child.ResolveAuxiliaryTask(CallerLabelTaskShortID).ShouldRun())
}
