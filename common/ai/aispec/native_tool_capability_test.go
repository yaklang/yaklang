package aispec

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestProviderFunctionCallDefault(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider *ypb.ThirdPartyApplicationConfig
		want     bool
	}{
		{"script default off", &ypb.ThirdPartyApplicationConfig{Type: "yak-script"}, false},
		{"script opt in", &ypb.ThirdPartyApplicationConfig{Type: "yak-script", EnableNativeToolCalling: true}, true},
		{"ordinary provider unchanged", &ypb.ThirdPartyApplicationConfig{Type: "openai"}, true},
		{"unknown provider unchanged", &ypb.ThirdPartyApplicationConfig{Type: "unknown"}, true},
		{"unset", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, ProviderFunctionCallDefault(tc.provider)) })
	}
}
