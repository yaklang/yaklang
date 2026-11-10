package aicommon

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestImageInputConfigCloningAndValidation(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true), WithWorkdir(t.TempDir()), WithImageInput(&ypb.AIImageInputConfig{Mode: "native", MainModelSupportsImages: proto.Bool(true)}))
	child := NewConfig(context.Background(), ConvertConfigToOptions(cfg)...)
	require.Equal(t, "native", child.GetImageInputConfig().Mode)
	require.True(t, child.GetImageInputConfig().GetMainModelSupportsImages())
	copy := child.GetImageInputConfig()
	copy.Mode = "describe"
	require.Equal(t, "native", child.GetImageInputConfig().Mode)
	require.Error(t, WithImageInput(&ypb.AIImageInputConfig{Mode: "invalid"})(cfg))
	require.Error(t, WithImageInput(&ypb.AIImageInputConfig{MaxLongEdge: 63})(cfg))
}

func TestImageRequestChecksCompleteTextAndMediaBudget(t *testing.T) {
	req := NewAIRequest("small prompt", WithAIRequest_ImageBudget(ImageRequestBudget{InputLimit: 100, NonTextTokens: 200, OutputReserve: 10, MaxBytes: 10000}))
	require.ErrorIs(t, req.CheckImageBudget(), aispec.ErrPromptOverflow)
	req = NewAIRequest("small prompt", WithAIRequest_ImageBudget(ImageRequestBudget{InputLimit: 10000, MediaBytes: 5000, MaxBytes: 1000}))
	require.ErrorContains(t, req.CheckImageBudget(), "byte budget")
	req = NewAIRequest("small prompt", WithAIRequest_ImageBudget(ImageRequestBudget{InputLimit: 10000, NonTextTokens: 200, OutputReserve: 10, MediaBytes: 500, MaxBytes: 1000}))
	require.NoError(t, req.CheckImageBudget())
}

func TestImageUsageObserverHandlesLateReportsWithoutSharingMutableState(t *testing.T) {
	resp := NewAIResponse(nil)
	defer resp.Close()
	calls := 0
	resp.OnUsage(func(u *aispec.ChatUsage) {
		calls++
		require.Equal(t, 100, u.PromptTokens)
		require.Equal(t, 100, resp.GetUsageInfo().PromptTokens)
		u.PromptTokens = 999
	})
	resp.SetUsageInfo(&aispec.ChatUsage{PromptTokens: 100})
	require.Equal(t, 1, calls)
	require.Equal(t, 100, resp.GetUsageInfo().PromptTokens)
	resp.OnUsage(func(u *aispec.ChatUsage) { calls++; require.Equal(t, 100, u.PromptTokens) })
	require.Equal(t, 2, calls)
}

func TestPinnedImageModelSurvivesTierHotpatch(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	observed := make(chan string, 1)
	aispec.Register("image-model-pin-test", func() aispec.AIClient {
		return &tieredSingleModeGateway{observe: func(c *aispec.AIConfig) { observed <- c.Model }}
	})
	before := &ypb.AIModelConfig{ModelName: "before", Provider: &ypb.ThirdPartyApplicationConfig{Type: "image-model-pin-test"}, SupportsImageInput: proto.Bool(true)}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{IntelligentConfigs: []*ypb.AIModelConfig{before}})
	callback, err := GetIntelligentAIModelCallback(false)
	require.NoError(t, err)
	request := NewAIRequest("image projection", WithAIRequest_ImageModel(before))
	after := proto.Clone(before).(*ypb.AIModelConfig)
	after.ModelName = "after"
	consts.SetTieredAIConfig(&consts.TieredAIConfig{IntelligentConfigs: []*ypb.AIModelConfig{after}})
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true))
	response, err := callback(cfg, request)
	require.NoError(t, err)
	_, err = io.ReadAll(response.GetOutputStreamReader("pin", true, cfg.GetEmitter()))
	require.NoError(t, err)
	require.Equal(t, "before", <-observed)
}
