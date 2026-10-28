package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestProviderFunctionCallDefaultDoesNotLoadGlobalConfig(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	wasLoaded := aiconfig.IsConfigLoaded()
	t.Cleanup(func() {
		if wasLoaded {
			aiconfig.EnsureConfigLoaded()
		} else {
			aiconfig.ResetConfigLoaded()
		}
	})
	for _, tc := range []struct {
		name   string
		tiered *consts.TieredAIConfig
		want   bool
	}{
		{name: "no configured model", want: true},
		{name: "script text", tiered: &consts.TieredAIConfig{IntelligentConfigs: []*ypb.AIModelConfig{
			{ModelName: "text", Provider: &ypb.ThirdPartyApplicationConfig{Type: "yak-script"}},
		}}},
		{name: "script native", want: true, tiered: &consts.TieredAIConfig{IntelligentConfigs: []*ypb.AIModelConfig{
			{ModelName: "native", Provider: &ypb.ThirdPartyApplicationConfig{Type: "yak-script", EnableNativeToolCalling: true}},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aiconfig.ResetConfigLoaded()
			consts.SetTieredAIConfig(tc.tiered)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := newConfig(ctx)
			cfg.applyProviderFunctionCallDefault()
			require.False(t, aiconfig.IsConfigLoaded(), "protocol selection must not initialize global model routing")
			require.Same(t, tc.tiered, consts.GetTieredAIConfig(), "protocol selection must not replace the effective config")
			require.Equal(t, tc.want, cfg.EnableFunctionCallMode)
		})
	}
}

func TestProviderFunctionCallDefault(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, tc := range []struct {
		name, provider string
		opts           []ConfigOption
		want           bool
		enableNative   bool
	}{
		{name: "script default", provider: "yak-script"},
		{name: "script enabled", provider: "yak-script", enableNative: true, want: true},
		{name: "explicit text overrides enabled provider", provider: "yak-script", enableNative: true, opts: []ConfigOption{WithEnableFunctionCallMode(false)}},
		{name: "standard provider", provider: "openai", want: true},
		{name: "unknown provider keeps default", provider: "unregistered-test-provider", want: true},
		{name: "explicit native remains explicit", provider: "yak-script", opts: []ConfigOption{WithEnableFunctionCallMode(true)}, want: true},
		{name: "explicit text", provider: "openai", opts: []ConfigOption{WithEnableFunctionCallMode(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, SingleModelMode: true,
				IntelligentConfigs: []*ypb.AIModelConfig{{ModelName: "protocol-test", Provider: &ypb.ThirdPartyApplicationConfig{Type: tc.provider, EnableNativeToolCalling: tc.enableNative}}}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := append([]ConfigOption{WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true)}, tc.opts...)
			cfg := NewConfig(ctx, opts...)
			require.Equal(t, tc.want, cfg.EnableFunctionCallMode)
			require.Equal(t, tc.want, cfg.GetConfigBool("EnableFunctionCallMode"))
			child := NewConfig(ctx, ConvertConfigToOptions(cfg)...)
			require.Equal(t, tc.want, child.EnableFunctionCallMode)
			require.Equal(t, tc.want, child.GetConfigBool("EnableFunctionCallMode"))
		})
	}
}

func TestProviderFunctionCallDefaultModelSelection(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	previousPromote, previousCallback := hotPatchPromoteIntelligentConfig, hotPatchGetIntelligentCallback
	t.Cleanup(func() {
		hotPatchPromoteIntelligentConfig, hotPatchGetIntelligentCallback = previousPromote, previousCallback
	})
	hotPatchPromoteIntelligentConfig = func(string, string) error { return nil }
	hotPatchGetIntelligentCallback = func(string, string) (AICallbackType, error) {
		return func(c AICallerConfigIf, _ *AIRequest) (*AIResponse, error) { return c.NewAIResponse(), nil }, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := newConfig(ctx)
	aiconfig.EnsureConfigLoaded()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, IntelligentConfigs: []*ypb.AIModelConfig{
		{ModelName: "text", Provider: &ypb.ThirdPartyApplicationConfig{Type: "yak-script"}},
		{ModelName: "native", Provider: &ypb.ThirdPartyApplicationConfig{Type: "yak-script", EnableNativeToolCalling: true}},
		{ModelName: "ordinary", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}},
	}})
	for _, tc := range []struct {
		provider, model string
		want            bool
	}{
		{"yak-script", "native", true}, {"yak-script", "text", false}, {"openai", "ordinary", true}, {"yak-script", "text", false},
	} {
		options := cfg.ProcessHotPatchMessage(&ypb.AIInputEvent{IsConfigHotpatch: true, HotpatchType: HotPatchType_AIService,
			Params: &ypb.AIStartParams{AIService: tc.provider, AIModelName: tc.model}})
		for _, option := range options {
			require.NoError(t, option(cfg))
		}
		require.Equal(t, tc.want, cfg.EnableFunctionCallMode)
		require.Equal(t, tc.want, cfg.GetConfigBool("EnableFunctionCallMode"))
	}
}
