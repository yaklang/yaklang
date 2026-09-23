package volcengine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestLoadOptionExtraOptionsCompatibility(t *testing.T) {
	previous := aispec.EnableNewLoadOption
	t.Cleanup(func() { aispec.EnableNewLoadOption = previous })

	for _, tc := range []struct {
		name      string
		enabled   bool
		url       string
		modelName string
	}{
		{
			name:      "modern applies extra options",
			enabled:   true,
			url:       "https://extra.example/custom",
			modelName: "extra-model",
		},
		{
			name:      "legacy ignores extra options",
			enabled:   false,
			url:       "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
			modelName: "doubao-lite-4k",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aispec.EnableNewLoadOption = tc.enabled
			client := &GatewayClient{ExtraOptions: []aispec.AIConfigOption{
				aispec.WithBaseURL("https://extra.example/custom"),
				aispec.WithModel("extra-model"),
			}}
			client.LoadOption(aispec.WithAPIKey("test-key"))
			require.Equal(t, tc.url, client.targetUrl)
			require.Equal(t, tc.modelName, client.GetConfig().Model)
			require.Equal(t, "test-key", client.GetConfig().APIKey)
		})
	}
}
