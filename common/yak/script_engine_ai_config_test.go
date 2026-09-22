package yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise the benchmark's JSON headers, numeric conversions and variadic option
// list through the real VM, then compare with the frontend configuration path.
func TestScriptEngineAIModelConfigOptions(t *testing.T) {
	maxTokens, topK := int64(8192), int64(40)
	temperature, topP, frequencyPenalty := 0.7, 0.9, -0.5
	reasoningEffort := "high"
	zeroInt, zeroFloat := int64(0), 0.0

	for _, tt := range []struct {
		name     string
		script   string
		provider *ypb.ThirdPartyApplicationConfig
	}{
		{
			name: "frontend settings",
			script: `
headers = json.loads("{\"X-Benchmark\":\"test-run\"}")
maxTokens, _ = atoi("8192")
topK, _ = atoi("40")
options = append(options,
    ai.baseURL("https://example.com/v1"),
    ai.endpoint("https://example.com/custom/responses"),
    ai.enableEndpoint(true),
    ai.apiType(" responses "),
    ai.proxy("http://127.0.0.1:8080"),
    ai.extraHeader(headers),
    ai.extraHeader({"X-Literal": "literal-value"}),
    ai.reasoningEffort("high"),
    ai.maxTokens(maxTokens),
    ai.temperature(float("0.7")),
    ai.topP(float("0.9")),
    ai.topK(topK),
    ai.frequencyPenalty(float("-0.5")),
)
`,
			provider: &ypb.ThirdPartyApplicationConfig{
				BaseURL:        "https://example.com/v1",
				Endpoint:       "https://example.com/custom/responses",
				EnableEndpoint: true,
				APIType:        " responses ",
				Proxy:          "http://127.0.0.1:8080",
				Headers: []*ypb.KVPair{
					{Key: "X-Benchmark", Value: "test-run"},
					{Key: "X-Literal", Value: "literal-value"},
				},
				ReasoningEffort:  &reasoningEffort,
				MaxTokens:        &maxTokens,
				Temperature:      &temperature,
				TopP:             &topP,
				TopK:             &topK,
				FrequencyPenalty: &frequencyPenalty,
			},
		},
		{
			name: "explicit zero values",
			script: `
options = append(options,
    ai.enableEndpoint(true),
    ai.enableEndpoint(false),
    ai.extraHeader({}),
    ai.maxTokens(0),
    ai.temperature(0),
    ai.topP(0),
    ai.topK(0),
    ai.frequencyPenalty(0),
)
`,
			provider: &ypb.ThirdPartyApplicationConfig{
				MaxTokens:        &zeroInt,
				Temperature:      &zeroFloat,
				TopP:             &zeroFloat,
				TopK:             &zeroInt,
				FrequencyPenalty: &zeroFloat,
			},
		},
		{
			name:     "omitted optional values",
			provider: &ypb.ThirdPartyApplicationConfig{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var captured *aispec.AIConfig
			capture := func(options ...aispec.AIConfigOption) {
				captured = &aispec.AIConfig{}
				for _, option := range options {
					option(captured)
				}
			}
			var engineConfig *aiengine.AIEngineConfig
			captureEngine := func(option aiengine.AIEngineConfigOption) {
				engineConfig = &aiengine.AIEngineConfig{}
				option(engineConfig)
			}
			engine := NewScriptEngine(1)
			engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
				engine.SetVars(map[string]any{"capture": capture, "captureEngine": captureEngine})
				return nil
			})
			_, err := engine.ExecuteExWithContext(context.Background(), `
options = [ai.apiKey("test-key"), ai.model("test-model")]
`+tt.script+`
capture(options...)
captureEngine(aim.qualityPriorityAIConfig("openai", options...))
`, map[string]any{})
			require.NoError(t, err)
			require.NotNil(t, captured)
			require.NotNil(t, engineConfig)
			require.NotNil(t, engineConfig.QualityPriorityAICallback)

			tt.provider.APIKey = "test-key"
			frontendConfig := &aispec.AIConfig{}
			for _, option := range aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{
				Provider: tt.provider, ModelName: "test-model",
			}) {
				option(frontendConfig)
			}
			require.Equal(t, aispec.ExtraHeadersToMap(frontendConfig.Headers), aispec.ExtraHeadersToMap(captured.Headers))
			frontendConfig.Headers, captured.Headers = nil, nil
			require.Equal(t, frontendConfig, captured)
		})
	}
}
