package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 关键词: cost_calc_test, ComputeWeightedTokens 单元测试

func TestComputeWeightedTokens_NilUsage(t *testing.T) {
	got := ComputeWeightedTokens(&AiModelMeta{
		InputTokenMultiplier:  1.0,
		OutputTokenMultiplier: 1.0,
	}, nil)
	assert.Equal(t, int64(0), got)
}

func TestComputeWeightedTokens_NilMeta_DefaultMultipliers(t *testing.T) {
	usage := &aispec.ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
	}
	// meta == nil -> input=1.0, output=1.0
	// pureInput = 100 - 0 - 0 = 100
	// weighted = 100*1.0 + 50*1.0 = 150
	assert.Equal(t, int64(150), ComputeWeightedTokens(nil, usage))
}

func TestComputeWeightedTokens_FourDimensionMultipliers(t *testing.T) {
	meta := &AiModelMeta{
		InputTokenMultiplier:    2.0,
		OutputTokenMultiplier:   3.0,
		CacheCreationMultiplier: 1.25,
		CacheHitMultiplier:      0.1,
	}
	usage := &aispec.ChatUsage{
		PromptTokens:     1000, // 包含 cached + cache_create
		CompletionTokens: 200,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens:             300,
			CacheCreationInputTokens: 100,
		},
	}
	// pureInput = 1000 - 300 - 100 = 600
	// weighted = 600*2.0 + 200*3.0 + 100*1.25 + 300*0.1
	//         = 1200 + 600 + 125 + 30 = 1955
	assert.Equal(t, int64(1955), ComputeWeightedTokens(meta, usage))
}

func TestComputeWeightedTokens_LegacyTrafficMultiplierIgnored(t *testing.T) {
	// 老 TrafficMultiplier 字节倍率体系已停用：四维全为 0 时不再回落到 TrafficMultiplier，
	// 而是直接采用标准默认倍率（input=1.0/output=1.0/cache_create=1.25/cache_hit=0.1）。
	// 关键词: 老 TrafficMultiplier 停用, 四维默认回落
	meta := &AiModelMeta{
		TrafficMultiplier: 2.0, // 即便设置也应被忽略
	}
	usage := &aispec.ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens:             10,
			CacheCreationInputTokens: 5,
		},
	}
	// pureInput = 100 - 10 - 5 = 85
	// 标准默认倍率：weighted = 85*1.0 + 50*1.0 + 5*1.25 + 10*0.1 = 85 + 50 + 6.25 + 1 = 142.25 -> 142
	assert.Equal(t, int64(142), ComputeWeightedTokens(meta, usage))
}

func TestComputeWeightedTokens_PartialFieldFallback(t *testing.T) {
	// 仅设 input/output，cache 维度走默认值 (1.25 / 0.1)
	meta := &AiModelMeta{
		InputTokenMultiplier:  1.5,
		OutputTokenMultiplier: 2.0,
		// CacheCreationMultiplier=0 -> 默认 1.25
		// CacheHitMultiplier=0      -> 默认 0.1
	}
	usage := &aispec.ChatUsage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens:             100,
			CacheCreationInputTokens: 50,
		},
	}
	// pureInput = 1000 - 100 - 50 = 850
	// weighted = 850*1.5 + 200*2.0 + 50*1.25 + 100*0.1
	//         = 1275 + 400 + 62.5 + 10 = 1747.5 -> 1748
	assert.Equal(t, int64(1748), ComputeWeightedTokens(meta, usage))
}

func TestComputeWeightedTokens_NoOverlapDoubleCounting(t *testing.T) {
	// 验证 prompt 全部由 cached 构成时不会重复计费
	meta := &AiModelMeta{
		InputTokenMultiplier:    1.0,
		OutputTokenMultiplier:   1.0,
		CacheCreationMultiplier: 1.0,
		CacheHitMultiplier:      1.0,
	}
	usage := &aispec.ChatUsage{
		PromptTokens: 100,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens: 100,
		},
	}
	// pureInput = 100 - 100 - 0 = 0
	// weighted = 0*1 + 0*1 + 0*1 + 100*1 = 100 (不是 200)
	assert.Equal(t, int64(100), ComputeWeightedTokens(meta, usage))
}

func TestComputeWeightedTokens_NegativePureInputClamped(t *testing.T) {
	// 上游异常：cached + cache_create > prompt
	meta := &AiModelMeta{
		InputTokenMultiplier:    1.0,
		OutputTokenMultiplier:   1.0,
		CacheCreationMultiplier: 1.0,
		CacheHitMultiplier:      1.0,
	}
	usage := &aispec.ChatUsage{
		PromptTokens:     50,
		CompletionTokens: 10,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens:             80,
			CacheCreationInputTokens: 20,
		},
	}
	// pureInput clamp to 0
	// weighted = 0 + 10 + 20 + 80 = 110
	assert.Equal(t, int64(110), ComputeWeightedTokens(meta, usage))
}

func TestComputeWeightedTokens_ZeroUsage(t *testing.T) {
	meta := &AiModelMeta{InputTokenMultiplier: 1.0, OutputTokenMultiplier: 1.0}
	usage := &aispec.ChatUsage{}
	assert.Equal(t, int64(0), ComputeWeightedTokens(meta, usage))
}

func TestResolveMultipliers_LegacyTrafficIgnored(t *testing.T) {
	// 老 TrafficMultiplier 已停用：四维全 0 时回落到标准默认倍率，不再受 TrafficMultiplier 影响。
	// 关键词: resolveMultipliers 老 TrafficMultiplier 停用
	r := resolveMultipliers(&AiModelMeta{TrafficMultiplier: 3.0})
	assert.Equal(t, 1.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.1, r.CacheHit, 1e-9)
}

func TestResolveMultipliers_NilMeta(t *testing.T) {
	r := resolveMultipliers(nil)
	assert.Equal(t, 1.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.Equal(t, 0.1, r.CacheHit)
}
