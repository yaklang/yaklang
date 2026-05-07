package aicommon

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

func TestConfig_AddTierConsumption(t *testing.T) {
	cfg := newConfig(context.Background())

	cfg.AddTierConsumption(consts.TierIntelligent, 10, 5)
	cfg.AddTierConsumption(consts.TierIntelligent, 3, 7)
	cfg.AddTierConsumption(consts.TierLightweight, 2, 1)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(13), snapshot[string(consts.TierIntelligent)]["input_consumption"])
	require.Equal(t, int64(12), snapshot[string(consts.TierIntelligent)]["output_consumption"])
	require.Equal(t, int64(2), snapshot[string(consts.TierLightweight)]["input_consumption"])
	require.Equal(t, int64(1), snapshot[string(consts.TierLightweight)]["output_consumption"])
}

func TestConfig_AddTierCacheHitToken(t *testing.T) {
	cfg := newConfig(context.Background())

	cfg.AddTierCacheHitToken(consts.TierIntelligent, 9)
	cfg.AddTierCacheHitToken(consts.TierIntelligent, 4)
	cfg.AddTierCacheHitToken(consts.TierLightweight, 2)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(13), snapshot[string(consts.TierIntelligent)]["cache_hit_token"])
	require.Equal(t, int64(2), snapshot[string(consts.TierLightweight)]["cache_hit_token"])
	require.Equal(t, int64(15), cfg.GetCacheHitToken())
}

func TestConvertConfigToOptions_PreserveTierConsumptionStats(t *testing.T) {
	parent := newConfig(context.Background())
	parent.AddTierConsumption(consts.TierIntelligent, 4, 6)

	child := newConfig(context.Background())
	opts := ConvertConfigToOptions(parent)
	for _, opt := range opts {
		require.NoError(t, opt(child))
	}

	require.Same(
		t,
		parent.InitStatus.GetOrCreateConsumptionState().GetTierConsumptionStats(),
		child.InitStatus.GetOrCreateConsumptionState().GetTierConsumptionStats(),
	)

	child.AddTierConsumption(consts.TierIntelligent, 1, 2)
	snapshot := parent.GetTierConsumptionSnapshot()
	require.Equal(t, int64(5), snapshot[string(consts.TierIntelligent)]["input_consumption"])
	require.Equal(t, int64(8), snapshot[string(consts.TierIntelligent)]["output_consumption"])
}

func TestWrapper_TracksOutputConsumptionByTier(t *testing.T) {
	cfg := newConfig(context.Background())
	cb := cfg.wrapper(func(i AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		resp := i.NewAIResponse()
		resp.EmitOutputStream(strings.NewReader("hello"))
		resp.Close()
		return resp, nil
	}, consts.TierLightweight)

	req := NewAIRequest("ping")
	req.SetDetachCheckpoint(true)

	resp, err := cb(cfg, req)
	require.NoError(t, err)

	reasonReader, outputReader := resp.GetUnboundStreamReaderEx(nil, nil, nil)
	_, _ = io.ReadAll(reasonReader)
	_, _ = io.ReadAll(outputReader)

	require.Eventually(t, func() bool {
		snapshot := cfg.GetTierConsumptionSnapshot()
		return snapshot[string(consts.TierLightweight)]["output_consumption"] > 0
	}, time.Second, 20*time.Millisecond)
}

func TestWrapper_TracksCacheHitTokenByTier(t *testing.T) {
	cfg := newConfig(context.Background())
	rsp := NewUnboundAIResponse()
	rsp.totalOutputTokens.Store(9)
	rsp.SetUsageInfo(&aispec.ChatUsage{
		PromptTokens:     15,
		CompletionTokens: 7,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens: 12,
		},
	})

	cfg.finalizeTierConsumption(consts.TierLightweight, 20, rsp)

	snapshot := cfg.GetTierConsumptionSnapshot()
	require.Equal(t, int64(15), snapshot[string(consts.TierLightweight)]["input_consumption"])
	require.Equal(t, int64(7), snapshot[string(consts.TierLightweight)]["output_consumption"])
	require.Equal(t, int64(12), snapshot[string(consts.TierLightweight)]["cache_hit_token"])
	require.Equal(t, int64(12), cfg.GetCacheHitToken())
}
