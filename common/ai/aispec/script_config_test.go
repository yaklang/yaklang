package aispec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScriptModelConfigOptions(t *testing.T) {
	model := &ypb.AIModelConfig{ModelName: "script-model", Provider: &ypb.ThirdPartyApplicationConfig{
		Type: "yak-script", ExtraParams: []*ypb.KVPair{{Key: "script", Value: "old"}, {Key: "script_timeout_seconds", Value: "240"}},
	}, ExtraParams: []*ypb.KVPair{{Key: "script", Value: "new"}}}
	// Exercise the JSON used to persist and exchange AI model configuration.
	data, err := json.Marshal(model)
	require.NoError(t, err)
	model = new(ypb.AIModelConfig)
	require.NoError(t, json.Unmarshal(data, model))
	config := NewDefaultAIConfig(BuildOptionsFromConfig(model)...)
	require.Equal(t, "new", config.Script)
	require.Equal(t, float64(240), config.Timeout)
	require.Equal(t, "script-model", config.Model)
	model.Provider.Type = "openai"
	config = NewDefaultAIConfig(BuildOptionsFromConfig(model)...)
	require.Empty(t, config.Script)
}
