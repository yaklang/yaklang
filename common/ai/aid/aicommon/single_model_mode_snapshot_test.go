package aicommon

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type pr5128ModelCall struct {
	provider string
	model    string
	thinking string
}

func pr5128TierConfig(single bool, suffix string) *consts.TieredAIConfig {
	model := func(role, name string) *ypb.AIModelConfig {
		level := "high"
		return &ypb.AIModelConfig{
			Provider: &ypb.ThirdPartyApplicationConfig{
				Type: "test-pr5128-" + role, ReasoningEffort: &level,
			},
			ModelName: name + suffix,
		}
	}
	return &consts.TieredAIConfig{
		Enabled:            true,
		SingleModelMode:    single,
		IntelligentConfigs: []*ypb.AIModelConfig{model("quality", "Q")},
		LightweightConfigs: []*ypb.AIModelConfig{model("speed", "S")},
		VisionConfigs:      []*ypb.AIModelConfig{model("vision", "V")},
	}
}

func pr5128ModeFixture(t *testing.T, single bool) chan pr5128ModelCall {
	t.Helper()
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	observed := make(chan pr5128ModelCall, 32)
	for _, role := range []string{"quality", "speed", "vision"} {
		provider := "test-pr5128-" + role
		aispec.Register(provider, func() aispec.AIClient {
			return &tieredSingleModeGateway{observe: func(cfg *aispec.AIConfig) {
				observed <- pr5128ModelCall{cfg.Type, cfg.Model, cfg.ThinkingLevel}
			}}
		})
	}
	consts.SetTieredAIConfig(pr5128TierConfig(single, "0"))
	return observed
}

func pr5128NewModeConfig(t *testing.T, opts ...ConfigOption) *Config {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	base := []ConfigOption{
		WithDisableAutoSkills(true),
		WithDisableCreateDBRuntime(true),
	}
	return NewConfig(ctx, append(base, opts...)...)
}

func pr5128AssertMode(t *testing.T, cfg *Config, observed chan pr5128ModelCall, single bool, suffix string) {
	t.Helper()
	require.Equal(t, single, cfg.IsSingleAIModelMode())
	require.Equal(t, !single, cfg.ResolveAuxiliaryTask(CallerLabelTaskShortID).ShouldRun())
	require.True(t, cfg.ResolveAuxiliaryTask("pr5128-required-run").ShouldRun())
	require.Equal(t, single, cfg.DisableMemoryTriage)
	require.Equal(t, single, cfg.DisableIntentRecognition)
	require.Equal(t, single, cfg.DisableIntervalReview)
	require.Equal(t, single, cfg.GetConfigBool("disable_session_title_generation"))
	require.Equal(t, single, cfg.BuildConsumptionPayload()["effective_single_model_mode"])

	call := func(cb AICallbackType, role, model, thinking string) {
		t.Helper()
		require.NotNil(t, cb)
		response, err := cb(cfg, NewAIRequest("mode snapshot"))
		require.NoError(t, err)
		_, err = io.ReadAll(response.GetOutputStreamReader("pr5128", true, cfg.GetEmitter()))
		require.NoError(t, err)
		require.NoError(t, response.GetError())
		select {
		case got := <-observed:
			require.Equal(t, pr5128ModelCall{"test-pr5128-" + role, model + suffix, thinking}, got)
		default:
			t.Fatal("local provider was not called")
		}
	}
	call(cfg.GetQualityPriorityAICallback(), "quality", "Q", "high")
	if single {
		call(cfg.GetSpeedPriorityAICallback(), "quality", "Q", "none")
		call(cfg.GetVisionPriorityAICallback(), "quality", "Q", "high")
	} else {
		call(cfg.GetSpeedPriorityAICallback(), "speed", "S", "high")
		call(cfg.GetVisionPriorityAICallback(), "vision", "V", "high")
	}
}

func TestPR5128_ModeSnapshot_GlobalOffToOn(t *testing.T) {
	observed := pr5128ModeFixture(t, false)
	old := pr5128NewModeConfig(t, WithSingleAIModelMode(false))
	pr5128AssertMode(t, old, observed, false, "0")
	consts.SetTieredAIConfig(pr5128TierConfig(true, "1"))
	pr5128AssertMode(t, old, observed, false, "1")
	pr5128AssertMode(t, pr5128NewModeConfig(t, WithSingleAIModelMode(false)), observed, true, "1")
}

func TestPR5128_ModeSnapshot_GlobalOnToOff(t *testing.T) {
	observed := pr5128ModeFixture(t, true)
	old := pr5128NewModeConfig(t, WithSingleAIModelMode(false))
	pr5128AssertMode(t, old, observed, true, "0")
	consts.SetTieredAIConfig(pr5128TierConfig(false, "1"))
	pr5128AssertMode(t, old, observed, true, "1")
	pr5128AssertMode(t, pr5128NewModeConfig(t, WithSingleAIModelMode(false)), observed, false, "1")
}

func TestPR5128_ModeSnapshot_SessionOptInRemainsEnabled(t *testing.T) {
	observed := pr5128ModeFixture(t, false)
	old := pr5128NewModeConfig(t, WithSingleAIModelMode(true))
	pr5128AssertMode(t, old, observed, true, "0")
	consts.SetTieredAIConfig(pr5128TierConfig(true, "1"))
	pr5128AssertMode(t, old, observed, true, "1")
	pr5128AssertMode(t, pr5128NewModeConfig(t), observed, true, "1")
	consts.SetTieredAIConfig(pr5128TierConfig(false, "2"))
	pr5128AssertMode(t, old, observed, true, "2")
	pr5128AssertMode(t, pr5128NewModeConfig(t), observed, false, "2")
}

func TestPR5128_ModeSnapshot_GlobalWinsAtConstruction(t *testing.T) {
	observed := pr5128ModeFixture(t, true)
	pr5128AssertMode(t, pr5128NewModeConfig(t, WithSingleAIModelMode(false)), observed, true, "0")
}

func TestPR5128_TierRole_ModelConfigCanRefresh(t *testing.T) {
	observed := pr5128ModeFixture(t, false)
	old := pr5128NewModeConfig(t)
	pr5128AssertMode(t, old, observed, false, "0")
	consts.SetTieredAIConfig(pr5128TierConfig(true, "1"))
	pr5128AssertMode(t, old, observed, false, "1")
	pr5128AssertMode(t, pr5128NewModeConfig(t), observed, true, "1")
	require.Equal(t, "high", consts.GetTieredAIConfig().IntelligentConfigs[0].Provider.GetReasoningEffort())
}

func TestPR5128_ChildConfig_InheritsParentModeAcrossGlobalChange(t *testing.T) {
	for _, initial := range []bool{false, true} {
		name := "off-to-on"
		if initial {
			name = "on-to-off"
		}
		t.Run(name, func(t *testing.T) {
			observed := pr5128ModeFixture(t, initial)
			parent := pr5128NewModeConfig(t)
			childOpts := func() []ConfigOption {
				return append(ConvertConfigToOptions(parent), WithAICallbacks(parent.GetRawAICallbacks()))
			}
			pr5128AssertMode(t, pr5128NewModeConfig(t, childOpts()...), observed, initial, "0")
			consts.SetTieredAIConfig(pr5128TierConfig(!initial, "1"))
			pr5128AssertMode(t, pr5128NewModeConfig(t, childOpts()...), observed, initial, "1")
		})
	}
}
