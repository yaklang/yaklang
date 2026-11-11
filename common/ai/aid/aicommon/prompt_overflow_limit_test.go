package aicommon

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPromptContextLearnsOnlyAfterMatchingSuccess(t *testing.T) {
	model := "context-test-" + uuid.NewString()
	cfg := newConfig(context.Background())
	cfg.AiServerName, cfg.AiModelName = "test", model
	db := consts.GetGormProfileDatabase()
	require.Equal(t, "default", yakit.GetAIModelContext(db, "test", model).Source)
	cfg.RecordPromptOverflow("test", model, 180000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	require.True(t, cfg.ModelContextStatus(180000).Pending)
	require.Equal(t, "default", yakit.GetAIModelContext(db, "test", model).Source)
	cfg.ConfirmPromptContextSuccess("test", "different-model")
	require.Equal(t, "default", yakit.GetAIModelContext(db, "test", model).Source)
	cfg.ConfirmPromptContextSuccess("test", model)
	require.Equal(t, yakit.AIModelContext{ContextTokens: 128000, ThresholdTokens: 102400, Source: "learned"}, yakit.GetAIModelContext(db, "test", model))
	require.False(t, cfg.ModelContextStatus(90000).Pending)
	fresh := newConfig(context.Background())
	require.EqualValues(t, 102400, fresh.EffectiveTimelineTokenLimit("test", model, int64(DefaultTimelineContentSizeLimit)))
	require.EqualValues(t, DefaultTimelineContentSizeLimit, fresh.EffectiveTimelineTokenLimit("test", model+"-other", int64(DefaultTimelineContentSizeLimit)))
}

func TestPromptContextWarnsBelow100K(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.AiServerName, cfg.AiModelName = "test", uuid.NewString()
	cfg.RecordPromptOverflow(cfg.AiServerName, cfg.AiModelName, 180000, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000})
	require.True(t, cfg.ModelContextStatus(180000).TooSmall)
}

func TestPromptContextUnknownMaximumUsesSuccessfulRequest(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.AiServerName, cfg.AiModelName = "test", uuid.NewString()
	cfg.RecordPromptOverflow(cfg.AiServerName, cfg.AiModelName, 200000, nil)
	require.EqualValues(t, 120000, cfg.ModelContextStatus(0).ThresholdTokens)
	cfg.ConfirmPromptContextSuccess(cfg.AiServerName, cfg.AiModelName, 150000)
	require.EqualValues(t, 150000, cfg.ModelContextStatus(0).ThresholdTokens)
	require.EqualValues(t, 150000, cfg.ModelContextStatus(0).ContextTokens)
}

func TestPromptContextLearnsCapacityWithSmallerManualThreshold(t *testing.T) {
	cfg := newConfig(context.Background())
	cfg.AiServerName, cfg.AiModelName = "test", uuid.NewString()
	require.NoError(t, WithTimelineContentLimit(100000)(cfg))
	require.Zero(t, cfg.RecordPromptOverflow(cfg.AiServerName, cfg.AiModelName, 180000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000}))
	cfg.ConfirmPromptContextSuccess(cfg.AiServerName, cfg.AiModelName, 80000)
	status := cfg.ModelContextStatus(0)
	require.EqualValues(t, 128000, status.ContextTokens)
	require.EqualValues(t, 100000, status.ThresholdTokens)
}

func TestPromptContextManualHotpatchIsModelScoped(t *testing.T) {
	cfg := newConfig(context.Background())
	model := uuid.NewString()
	cfg.AiServerName, cfg.AiModelName = "test", model
	cfg.RecordPromptOverflow("test", model, 180000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	for _, option := range cfg.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true, HotpatchType: HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{AIService: "test", AIModelName: model, RuntimeContextSizeLimit: 600 * 1024},
	}) {
		require.NoError(t, option(cfg))
	}
	status := cfg.ModelContextStatus(0)
	require.EqualValues(t, 600*1024, status.ThresholdTokens)
	require.Equal(t, "manual", status.Source)
	require.False(t, status.Pending)
	fresh := newConfig(context.Background())
	require.EqualValues(t, 600*1024, fresh.EffectiveTimelineTokenLimit("test", model, fresh.GetTimelineContentSizeLimit()))
	require.EqualValues(t, 500*1024, fresh.EffectiveTimelineTokenLimit("test", model+"-other", fresh.GetTimelineContentSizeLimit()))
	cfg.UpdateAIModelInfo("test", model+"-other")
	require.EqualValues(t, 500*1024, cfg.ModelContextStatus(0).ThresholdTokens, "switching models must not reuse a dragged threshold")
	params, changed := mergeHotpatchSessionStartParams(&ypb.AIStartParams{RuntimeContextSizeLimit: 300 * 1024}, &ypb.AIInputEvent{
		HotpatchType: HotPatchType_RuntimeContextSizeLimit,
		Params:       &ypb.AIStartParams{AIService: "test", AIModelName: model, RuntimeContextSizeLimit: 600 * 1024},
	})
	require.True(t, changed)
	require.Zero(t, params.GetRuntimeContextSizeLimit(), "restored sessions must resolve the selected model's saved setting")
}

func TestRecordPromptOverflowUsesReportedMaximum(t *testing.T) {
	cfg := newConfig(context.Background())
	require.EqualValues(t, 500*1024, cfg.GetTimelineContentSizeLimit())

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

// A provider alias is not proof that it has the same effective context capacity.
func TestEffectiveTimelineTokenLimitIsolatesProviderAndModel(t *testing.T) {
	cfg := newConfig(context.Background())
	base := cfg.GetTimelineContentSizeLimit()
	require.NotZero(t, cfg.RecordPromptOverflow("aibalance", "gpt-4o", 0, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000}))
	require.EqualValues(t, base, cfg.EffectiveTimelineTokenLimit("openai", "gpt-4o", base),
		"same model names at different providers must not share evidence")
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
