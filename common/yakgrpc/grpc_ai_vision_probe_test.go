package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestProbeModelVisionRejectsUnsupportedAndKeepsTransientFailuresUnknown(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   int
		body   string
		status string
		value  *bool
	}{
		{"unsupported", 400, "this model does not support image input", "image_unsupported", proto.Bool(false)},
		{"invalid image request", 400, "invalid MIME type", "request_failed", nil},
		{"rate limited", 429, "this model does not support image input", "rate_limited", nil},
		{"unverified answer", 200, "", "unexpected_output", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			result, err := probeModelVision(context.Background(), &ypb.ProbeModelVisionRequest{
				Config: &ypb.ThirdPartyApplicationConfig{Type: "openai"}, Model: "fixture-model",
			}, func(_ string, options ...aispec.AIConfigOption) (string, error) {
				cfg := aispec.NewDefaultAIConfig(options...)
				require.True(t, cfg.DisableProviderFallback)
				require.Len(t, cfg.Images, 1)
				require.Empty(t, cfg.Tools)
				calls++
				cfg.RawHTTPRequestResponseCallback(nil, []byte(fmt.Sprintf("HTTP/1.1 %d Fixture\r\n\r\n", tt.code)), []byte(tt.body), nil)
				return "unknown", nil
			})
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, tt.status, result.GetImageInputStatus())
			require.Equal(t, tt.value, result.SupportsImageInput)
		})
	}
}
