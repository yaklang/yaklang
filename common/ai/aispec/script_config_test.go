package aispec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestScriptModelConfigOptions(t *testing.T) {
	model := &ypb.AIModelConfig{ModelName: "display-label", Provider: &ypb.ThirdPartyApplicationConfig{
		Type: "yak-script", EnableNativeToolCalling: true, Script: "chat = func(req, emit) { return req.Prompt }",
		BaseURL: "https://ignored.invalid", APIKey: "ignored-key", Proxy: "ignored-proxy",
		Endpoint: "https://ignored.invalid/chat", EnableEndpoint: true,
		Headers:     []*ypb.KVPair{{Key: "X-Ignored", Value: "ignored"}},
		ExtraParams: []*ypb.KVPair{{Key: "script", Value: "must not override"}, {Key: "script_timeout_seconds", Value: "1"}},
	}, ExtraParams: []*ypb.KVPair{{Key: "script", Value: "must not override either"}}}
	data, err := json.Marshal(model)
	require.NoError(t, err)
	model = new(ypb.AIModelConfig)
	require.NoError(t, json.Unmarshal(data, model))
	wire, err := proto.Marshal(model)
	require.NoError(t, err)
	decoded := new(ypb.AIModelConfig)
	require.NoError(t, proto.Unmarshal(wire, decoded))
	require.Equal(t, model.Provider.Script, decoded.Provider.Script)
	model = decoded
	// Apply only explicit provider options, without ambient engine defaults.
	config := &AIConfig{}
	for _, option := range BuildOptionsFromConfig(model) {
		option(config)
	}
	require.Equal(t, model.Provider.Script, config.Script)
	require.True(t, config.EnableNativeToolCalling)
	require.True(t, decoded.Provider.EnableNativeToolCalling)
	require.Equal(t, "display-label", config.Model)
	require.Empty(t, config.BaseURL)
	require.Empty(t, config.Endpoint)
	require.Empty(t, config.APIKey)
	require.Empty(t, config.Proxy)
	require.Empty(t, config.Headers)
	require.Zero(t, config.Timeout)
	require.False(t, config.EnableEndpoint)
	// There is no fallback to development-era ExtraParams or cross-provider script handling.
	model.Provider.Script = ""
	config = &AIConfig{}
	for _, option := range BuildOptionsFromConfig(model) {
		option(config)
	}
	require.Empty(t, config.Script)
	model.Provider.Type = "openai"
	config = &AIConfig{}
	for _, option := range BuildOptionsFromConfig(model) {
		option(config)
	}
	require.Empty(t, config.Script)
	require.Equal(t, "ignored-key", config.APIKey)
}
