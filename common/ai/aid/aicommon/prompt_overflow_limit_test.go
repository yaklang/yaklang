package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestRecordPromptOverflowUsesReportedMaximum(t *testing.T) {
	cfg := newConfig(context.Background())
	require.EqualValues(t, 300*1024, cfg.GetTimelineContentSizeLimit())

	limit := cfg.RecordPromptOverflow("openai", "gpt-4o", 150000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	require.EqualValues(t, 102400, limit, "80%% of the reported maximum")

	require.Zero(t, cfg.RecordPromptOverflow("openai", "gpt-4o", 150000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000}),
		"an identical overflow must not offer a retry")

	limit = cfg.RecordPromptOverflow("openai", "gpt-4o", 150000, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000})
	require.EqualValues(t, 51200, limit, "a tighter report must lower the limit")

	require.EqualValues(t, 51200, cfg.EffectiveTimelineTokenLimit("openai", "gpt-4o", cfg.GetTimelineContentSizeLimit()))
}

func TestRecordPromptOverflowFallsBackToMeasuredPrompt(t *testing.T) {
	cfg := newConfig(context.Background())
	limit := cfg.RecordPromptOverflow("deepseek", "deepseek-chat", 100000, &aispec.PromptOverflowEvidence{})
	require.EqualValues(t, 60000, limit, "60%% of the failed prompt estimate")
}

func TestRecordPromptOverflowRefusesLimitsThatCannotTighten(t *testing.T) {
	cfg := newConfig(context.Background())
	require.NoError(t, WithTimelineContentLimit(8000)(cfg))

	require.Zero(t, cfg.RecordPromptOverflow("openai", "gpt-4o", 0, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000}),
		"a reported maximum above the configured threshold cannot help")
	require.Zero(t, cfg.RecordPromptOverflow("openai", "gpt-4o", 0, &aispec.PromptOverflowEvidence{}),
		"no evidence and no measurement means no retry")
	require.EqualValues(t, 8000, cfg.EffectiveTimelineTokenLimit("openai", "gpt-4o", cfg.GetTimelineContentSizeLimit()))
}

func TestRecordPromptOverflowClampsToFloor(t *testing.T) {
	cfg := newConfig(context.Background())
	limit := cfg.RecordPromptOverflow("", "", 0, &aispec.PromptOverflowEvidence{MaxContextTokens: 2000})
	require.EqualValues(t, promptOverflowMinDynamicLimit, limit)
	require.EqualValues(t, promptOverflowMinDynamicLimit, cfg.EffectiveTimelineTokenLimit("", "", cfg.GetTimelineContentSizeLimit()))
	require.EqualValues(t, cfg.GetTimelineContentSizeLimit(),
		cfg.EffectiveTimelineTokenLimit("other", "model", cfg.GetTimelineContentSizeLimit()),
		"a dynamic limit must not leak to another model")
}

func TestResetPromptOverflowLimit(t *testing.T) {
	cfg := newConfig(context.Background())
	require.NotZero(t, cfg.RecordPromptOverflow("openai", "gpt-4o", 0, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000}))
	cfg.ResetPromptOverflowLimit()
	require.EqualValues(t, cfg.GetTimelineContentSizeLimit(),
		cfg.EffectiveTimelineTokenLimit("openai", "gpt-4o", cfg.GetTimelineContentSizeLimit()))
}

// The same model behind another gateway keeps the reduced window; a different
// model must fall back to the configured threshold.
func TestEffectiveTimelineTokenLimitMatchesByModel(t *testing.T) {
	cfg := newConfig(context.Background())
	base := cfg.GetTimelineContentSizeLimit()
	require.NotZero(t, cfg.RecordPromptOverflow("aibalance", "gpt-4o", 0, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000}))
	require.EqualValues(t, 51200, cfg.EffectiveTimelineTokenLimit("openai", "gpt-4o", base),
		"the same model name keeps the dynamic limit")
	require.EqualValues(t, base, cfg.EffectiveTimelineTokenLimit("openai", "gpt-5", base))
	require.EqualValues(t, base, cfg.EffectiveTimelineTokenLimit("", "", base))
}

func TestPromptOverflowRetryMaxDefaultAndOverride(t *testing.T) {
	cfg := newConfig(context.Background())
	require.Equal(t, promptOverflowDefaultRetryMax, cfg.GetPromptOverflowRetryMax())

	require.NoError(t, WithPromptOverflowRetryMax(0)(cfg))
	require.Zero(t, cfg.GetPromptOverflowRetryMax(), "0 disables the retry")

	require.Error(t, WithPromptOverflowRetryMax(-1)(cfg))
}
