package sessionruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSingleModelStartParamsPolicy(t *testing.T) {
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, global := range []bool{false, true} {
		consts.SetTieredAIConfig(&consts.TieredAIConfig{SingleModelMode: global})
		for _, local := range []bool{false, true} {
			params := &ypb.AIStartParams{SingleModelMode: local, DisableAISearchForge: true}
			opts := ConvertStartParamsToReActConfig(params)
			opts = append(opts, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithFastAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					t.Error("configuration must not call AI")
					return nil, nil
				}))
			ctx, cancel := context.WithCancel(context.Background())
			cfg := aicommon.NewConfig(ctx, opts...)
			require.Equal(t, global || local, cfg.IsSingleAIModelMode())
			require.Equal(t, local, params.GetSingleModelMode(), "never persist the global OR result as a session setting")
			require.Equal(t, global, consts.IsSingleAIModelMode())
			cancel()
		}
	}
}

func TestSingleModelStartParamsIgnoreDeprecatedModelSelection(t *testing.T) {
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, global := range []bool{false, true} {
		consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, SingleModelMode: global,
			IntelligentConfigs: []*ypb.AIModelConfig{
				{Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}, ModelName: "global-quality"},
				{Provider: &ypb.ThirdPartyApplicationConfig{Type: "ollama"}, ModelName: "selected-quality"},
			}})
		opts := ConvertStartParamsToReActConfig(&ypb.AIStartParams{SingleModelMode: !global, AIService: "ollama", AIModelName: "obsolete-model", DisableAISearchForge: true})
		opts = append(opts, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true))
		ctx, cancel := context.WithCancel(context.Background())
		cfg := aicommon.NewConfig(ctx, opts...)
		require.True(t, cfg.IsSingleAIModelMode())
		require.Equal(t, "global-quality", cfg.AiModelName)
		require.Equal(t, "global-quality", consts.GetIntelligentAIConfigs()[0].ModelName)
		cancel()
	}
}
