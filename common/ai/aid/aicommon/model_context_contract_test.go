package aicommon

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func modelContextPatch(provider, model string, tokens int64) *ypb.AIInputEvent {
	return &ypb.AIInputEvent{IsConfigHotpatch: true, HotpatchType: HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{AIService: provider, AIModelName: model, RuntimeContextSizeLimit: tokens}}
}

func TestModelContextContractManualFloorCannotBeBypassed(t *testing.T) {
	for _, value := range []int64{-1, 1, 60 * 1000, 100*1000 - 1, 100 * 1000, 100001, 102400, 500 * 1000, 600 * 1000} {
		t.Run(strconv.FormatInt(value, 10), func(t *testing.T) {
			cfg := newConfig(context.Background())
			model := uuid.NewString()
			cfg.UpdateAIModelInfo("test", model)
			before := cfg.ModelContextStatus(0)
			event := modelContextPatch("test", model, value)
			options := cfg.ProcessHotPatchMessage(event)
			require.Len(t, options, 1)
			err := options[0](cfg)
			if value < yakit.MinimumAIModelContextTokens {
				require.ErrorContains(t, err, "100K")
				require.Equal(t, before, cfg.ModelContextStatus(0), "rejected input must leave runtime and durable state unchanged")
				_, changed := mergeHotpatchSessionStartParams(&ypb.AIStartParams{}, event)
				require.False(t, changed, "invalid values must not reappear on resume")
				require.Error(t, yakit.SaveAIModelContext(consts.GetGormProfileDatabase(), "test", model, yakit.AIModelContext{ContextTokens: 500 * 1000, ThresholdTokens: value, Source: "manual"}))
			} else {
				require.NoError(t, err)
				require.EqualValues(t, value, cfg.ModelContextStatus(0).ThresholdTokens)
				require.EqualValues(t, 500*1000, cfg.ModelContextStatus(0).ContextTokens, "a manual threshold is not evidence of capacity")
			}
		})
	}
}

func TestModelContextContractIndependentPendingModels(t *testing.T) {
	cfg := newConfig(context.Background())
	model := uuid.NewString()
	for _, provider := range []string{"quality", "lightweight"} {
		cfg.RecordPromptOverflow(provider, model, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	}
	cfg.ConfirmPromptContextSuccess("quality", model, 90000)
	require.False(t, cfg.ModelContextStatusFor("quality", model, 0).Pending)
	require.True(t, cfg.ModelContextStatusFor("lightweight", model, 0).Pending)
	require.Equal(t, "default", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "lightweight", model).Source)
	cfg.ConfirmPromptContextSuccess("lightweight", model, 80000)
	require.False(t, cfg.ModelContextStatusFor("lightweight", model, 0).Pending)
	require.Equal(t, "learned", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "lightweight", model).Source)
}

func TestModelContextContractCanceledRecoveryDoesNotLearn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := newConfig(ctx)
	model := uuid.NewString()
	cfg.RecordPromptOverflow("test", model, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	cancel()
	cfg.ConfirmPromptContextSuccess("test", model, 90000)
	require.Equal(t, "default", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model).Source)
	fresh := newConfig(context.Background())
	require.EqualValues(t, 500*1000, fresh.ModelContextStatusFor("test", model, 0).ThresholdTokens)
}

func TestModelContextContractLateProbeCannotOverwriteManualEdit(t *testing.T) {
	model := uuid.NewString()
	first, second := newConfig(context.Background()), newConfig(context.Background())
	first.RecordPromptOverflow("test", model, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	// Another session explicitly changes this same model while recovery is in flight.
	for _, option := range second.ProcessHotPatchMessage(modelContextPatch("test", model, 300*1000)) {
		require.NoError(t, option(second))
	}
	first.ConfirmPromptContextSuccess("test", model, 90000)
	status := first.ModelContextStatusFor("test", model, 0)
	require.Equal(t, "manual", status.Source)
	require.False(t, status.Pending)
	require.Nil(t, status.Change, "a superseded probe must not report an automatic adjustment")
	require.EqualValues(t, 300*1000, status.ThresholdTokens)
	legacy := newConfig(context.Background())
	require.NoError(t, WithTimelineContentLimit(128*1000)(legacy))
	require.EqualValues(t, 300*1000, legacy.ModelContextStatusFor("test", model, 0).ThresholdTokens,
		"a restored session's old explicit ceiling must not override the model setting")
	// A subsequent failure against the new manual setting can still be learned.
	first.RecordPromptOverflow("test", model, 200000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	first.ConfirmPromptContextSuccess("test", model, 90000)
	require.EqualValues(t, 102400, first.ModelContextStatusFor("test", model, 0).ThresholdTokens)
}

func TestModelContextContractRecoveryExplainsOriginalSettingAndPersistsSuccess(t *testing.T) {
	cfg := newConfig(context.Background())
	model := uuid.NewString()
	cfg.UpdateAIModelInfo("test", model)
	for _, option := range cfg.ProcessHotPatchMessage(modelContextPatch("test", model, 500*1000)) {
		require.NoError(t, option(cfg))
	}
	db := consts.GetGormProfileDatabase()
	// Two rejected attempts must not publish an unverified model setting.
	for _, maximum := range []int64{375 * 1000, 250 * 1000} {
		require.NotZero(t, cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: maximum}))
		require.True(t, cfg.ModelContextStatus(0).Pending)
		require.EqualValues(t, 500*1000, yakit.GetAIModelContext(db, "test", model).ThresholdTokens)
	}
	cfg.ConfirmPromptContextSuccess("test", model, 150*1000)
	confirmed := cfg.ModelContextStatus(150 * 1000)
	require.False(t, confirmed.Pending)
	require.Equal(t, "confirmed", confirmed.Change.Reason)
	require.EqualValues(t, 500*1000, confirmed.Change.PreviousThresholdTokens, "explain the original setting, not just the last retry")
	require.EqualValues(t, 200*1000, confirmed.Change.ThresholdTokens)
	cfg.ConfirmPromptContextSuccess("test", model, 160*1000)
	require.Equal(t, confirmed.Change.ID, cfg.ModelContextStatus(0).Change.ID, "later successful requests must not announce recovery again")
	fresh := newConfig(context.Background())
	fresh.UpdateAIModelInfo("test", model)
	require.EqualValues(t, 200*1000, fresh.ModelContextStatus(0).ThresholdTokens)
	require.Equal(t, "learned", fresh.ModelContextStatus(0).Source)
	require.EqualValues(t, 500*1000, fresh.ModelContextStatusFor("test", model+"-other", 0).ThresholdTokens)
	// An explicit new setting starts a new attempt, and may be learned again.
	for _, option := range fresh.ProcessHotPatchMessage(modelContextPatch("test", model, 500*1000)) {
		require.NoError(t, option(fresh))
	}
	require.EqualValues(t, 500*1000, fresh.ModelContextStatus(0).ThresholdTokens)
	require.NotZero(t, fresh.RecordPromptOverflow("test", model, 300*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250 * 1000}))
	fresh.ConfirmPromptContextSuccess("test", model, 150*1000)
	require.EqualValues(t, 500*1000, fresh.ModelContextStatus(0).Change.PreviousThresholdTokens)
	require.EqualValues(t, 200*1000, fresh.ModelContextStatus(0).ThresholdTokens)
	require.NotEqual(t, confirmed.Change.ID, fresh.ModelContextStatus(0).Change.ID)
}

func TestModelContextContractManualEditStartsANewRecoveryOrigin(t *testing.T) {
	cfg := newConfig(context.Background())
	model := uuid.NewString()
	cfg.UpdateAIModelInfo("test", model)
	cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 375 * 1000})
	for _, option := range cfg.ProcessHotPatchMessage(modelContextPatch("test", model, 600*1000)) {
		require.NoError(t, option(cfg))
	}
	cfg.ConfirmPromptContextSuccess("test", model, 150*1000)
	require.EqualValues(t, 600*1000, cfg.ModelContextStatus(0).ThresholdTokens)
	require.Nil(t, cfg.ModelContextStatus(0).Change)
	cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250 * 1000})
	cfg.ConfirmPromptContextSuccess("test", model, 150*1000)
	require.EqualValues(t, 600*1000, cfg.ModelContextStatus(0).Change.PreviousThresholdTokens)
	require.EqualValues(t, 200*1000, cfg.ModelContextStatus(0).ThresholdTokens)
}

func TestModelContextContractZeroFollowsSavedModelAcrossSessions(t *testing.T) {
	model := uuid.NewString()
	cfg := newConfig(context.Background())
	cfg.UpdateAIModelInfo("test", model)
	cfg.RecordPromptOverflow("test", model, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: 128000})
	cfg.ConfirmPromptContextSuccess("test", model, 90000)
	fresh := newConfig(context.Background())
	fresh.UpdateAIModelInfo("test", model)
	for _, option := range fresh.ProcessHotPatchMessage(modelContextPatch("test", model, 0)) {
		require.NoError(t, option(fresh))
	}
	require.EqualValues(t, 102400, fresh.ModelContextStatus(0).ThresholdTokens)
	require.Equal(t, "learned", fresh.ModelContextStatus(0).Source)
	fresh.UpdateAIModelInfo("test", uuid.NewString())
	require.EqualValues(t, 500*1000, fresh.ModelContextStatus(0).ThresholdTokens)
}

func TestModelContextContractEmitsPendingAndConfirmedChanges(t *testing.T) {
	var mu sync.Mutex
	var events []ModelContextStatus
	cfg := NewTestConfig(context.Background(), WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type != schema.EVENT_TYPE_MODEL_CONTEXT {
			return
		}
		var status ModelContextStatus
		if json.Unmarshal(event.Content, &status) == nil {
			mu.Lock()
			events = append(events, status)
			mu.Unlock()
		}
	}))
	model := uuid.NewString()
	cfg.UpdateAIModelInfo("test", model)
	cfg.EmitModelContext(300000)
	cfg.RecordPromptOverflow("test", model, 300000, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000})
	cfg.EmitModelContext(300000)
	cfg.ConfirmPromptContextSuccess("test", model, 45000)
	cfg.EmitModelContext(45000)
	cfg.EmitModelContext(46000)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, events, 4)
	require.EqualValues(t, 500*1000, events[0].ThresholdTokens)
	require.Nil(t, events[0].Change)
	require.True(t, events[1].Pending)
	require.True(t, events[1].TooSmall)
	require.EqualValues(t, 100*1000, events[1].MinimumThresholdTokens)
	require.Equal(t, "overflow", events[1].Change.Reason)
	require.EqualValues(t, 500*1000, events[1].Change.PreviousThresholdTokens)
	require.EqualValues(t, 51200, events[1].Change.ThresholdTokens)
	require.False(t, events[2].Pending)
	require.Equal(t, "confirmed", events[2].Change.Reason)
	require.NotEqual(t, events[1].Change.ID, events[2].Change.ID)
	require.Equal(t, events[2].Change.ID, events[3].Change.ID, "periodic updates must not repeat notifications")
}

func TestModelContextContractDisabledAutoAdjustInvalidatesPendingAndPersists(t *testing.T) {
	cfg := newConfig(context.Background())
	model := uuid.NewString()
	cfg.UpdateAIModelInfo("test", model)
	require.NotZero(t, cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250 * 1000}))
	disabled := true
	patch := modelContextPatch("test", model, 0)
	patch.Params.DisableContextAutoAdjust = &disabled
	for _, option := range cfg.ProcessHotPatchMessage(patch) {
		require.NoError(t, option(cfg))
	}
	require.True(t, cfg.ModelContextStatus(0).DisableAutoAdjust)
	require.EqualValues(t, 500*1000, cfg.ModelContextStatus(0).ThresholdTokens)
	require.False(t, cfg.ModelContextStatus(0).Pending)
	cfg.ConfirmPromptContextSuccess("test", model, 150*1000)
	require.Nil(t, cfg.ModelContextStatus(0).Change, "late success must not announce or persist a disabled recovery")
	require.Zero(t, cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250 * 1000}))
	fresh := newConfig(context.Background())
	require.True(t, fresh.ModelContextStatusFor("test", model, 0).DisableAutoAdjust)
	require.False(t, fresh.ModelContextStatusFor("other", model, 0).DisableAutoAdjust)
	for _, option := range cfg.ProcessHotPatchMessage(modelContextPatch("test", model, 600*1000)) {
		require.NoError(t, option(cfg))
	}
	require.True(t, cfg.ModelContextStatus(0).DisableAutoAdjust)
	before := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model)
	for _, option := range cfg.ProcessHotPatchMessage(modelContextPatch("test", model, 1000*1000+1)) {
		require.Error(t, option(cfg))
	}
	require.Equal(t, before, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model))
	_, changed := mergeHotpatchSessionStartParams(&ypb.AIStartParams{RuntimeContextSizeLimit: 600 * 1000}, modelContextPatch("test", model, 1000*1000+1))
	require.False(t, changed, "an above-maximum edit must not change restored session settings")
	disabled = false
	for _, option := range cfg.ProcessHotPatchMessage(patch) {
		require.NoError(t, option(cfg))
	}
	require.False(t, cfg.ModelContextStatus(0).DisableAutoAdjust)
	require.NotZero(t, cfg.RecordPromptOverflow("test", model, 400*1000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250 * 1000}))
	cfg.ConfirmPromptContextSuccess("test", model, 150*1000)
	require.EqualValues(t, 200*1000, cfg.ModelContextStatus(0).ThresholdTokens)
}
