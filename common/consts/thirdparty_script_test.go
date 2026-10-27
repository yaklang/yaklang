package consts

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScriptProviderConfigurationRoundTrip(t *testing.T) {
	original, exists := thirdPartyConfig.Load("yak-script")
	t.Cleanup(func() {
		if exists {
			thirdPartyConfig.Store("yak-script", original)
		} else {
			thirdPartyConfig.Delete("yak-script")
		}
	})
	const source = `chat = func(req, emit) { return req.Prompt }`
	config := &ypb.ThirdPartyApplicationConfig{Type: "yak-script", Script: source}
	UpdateThirdPartyApplicationConfig(config)
	require.Empty(t, config.ExtraParams, "script config must not gain flattened connection options")
	got, err := GetCommonThirdPartyApplicationConfig("yak-script")
	require.NoError(t, err)
	require.Equal(t, source, got.Script)
	all := AllThirdPartyApplicationConfig()
	found := false
	for _, item := range all {
		if item.Type == "yak-script" {
			require.Equal(t, source, item.Script)
			found = true
		}
	}
	require.True(t, found)
	var defaults struct{ Script string }
	require.NoError(t, GetThirdPartyApplicationConfig("yak-script", &defaults))
	require.Equal(t, source, defaults.Script)
	model := thirdPartyConfigToModelConfig(got)
	require.Equal(t, source, model.Provider.Script)
}
