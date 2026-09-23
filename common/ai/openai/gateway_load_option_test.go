package openai

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// The legacy branch remains selectable by callers of the public flag. These
// cases record URL differences that would be lost if the branch were removed.
func TestLoadOptionLegacyURLCompatibility(t *testing.T) {
	previous := aispec.EnableNewLoadOption
	t.Cleanup(func() { aispec.EnableNewLoadOption = previous })

	tests := []struct {
		name    string
		options []aispec.AIConfigOption
		modern  string
		legacy  string
	}{
		{
			name:   "default",
			modern: "https://api.openai.com/v1/chat/completions",
			legacy: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:    "base URL prefix",
			options: []aispec.AIConfigOption{aispec.WithBaseURL("https://proxy.example/v1")},
			modern:  "https://proxy.example/v1/chat/completions",
			legacy:  "https://proxy.example/v1",
		},
		{
			name: "explicit endpoint",
			options: []aispec.AIConfigOption{
				aispec.WithEndpoint("https://endpoint.example/custom"),
				aispec.WithEnableEndpoint(true),
			},
			modern: "https://endpoint.example/custom",
			legacy: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:    "responses API",
			options: []aispec.AIConfigOption{aispec.WithAPIType("responses")},
			modern:  "https://api.openai.com/v1/responses",
			legacy:  "https://api.openai.com/v1/chat/completions",
		},
		{
			name:    "HTTP default host",
			options: []aispec.AIConfigOption{aispec.WithNoHttps(true)},
			modern:  "http://api.openai.com/v1/chat/completions",
			legacy:  "https://api.openai.com/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, mode := range []struct {
				name    string
				enabled bool
				want    string
			}{
				{name: "modern", enabled: true, want: tt.modern},
				{name: "legacy", enabled: false, want: tt.legacy},
			} {
				t.Run(mode.name, func(t *testing.T) {
					aispec.EnableNewLoadOption = mode.enabled
					client := new(GatewayClient)
					client.LoadOption(tt.options...)
					if client.targetUrl != mode.want {
						t.Fatalf("target URL = %q, want %q", client.targetUrl, mode.want)
					}
				})
			}
		})
	}
}
