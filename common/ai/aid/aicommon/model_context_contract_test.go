package aicommon

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func contextTestModel() *ypb.AIModelConfig {
	return &ypb.AIModelConfig{ModelName: uuid.NewString(), Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://example.test/v1", APIKey: "test-only"}}
}
func bindContext(t *testing.T, cfg *Config, model *ypb.AIModelConfig) string {
	t.Helper()
	if model.GetContextOwnerId() == "" {
		db := consts.GetGormProfileDatabase()
		global, err := yakit.GetAIGlobalConfig(db)
		require.NoError(t, err)
		if global == nil {
			global = &ypb.AIGlobalConfig{}
		}
		global.IntelligentModels = append(global.IntelligentModels, model)
		_, err = yakit.SetAIGlobalConfig(db, global)
		require.NoError(t, err)
	}
	require.NoError(t, WithPrimaryModelContext(model)(cfg))
	return aispec.ModelContextIdentity(model)
}
func contextPatch(cfg *Config, tokens int64) *ypb.AIInputEvent {
	_, _, key := cfg.PrimaryModelContext()
	return &ypb.AIInputEvent{IsConfigHotpatch: true, HotpatchType: HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{ModelContextKey: key, RuntimeContextSizeLimit: tokens}}
}
func applyContextPatch(t *testing.T, cfg *Config, patch *ypb.AIInputEvent) error {
	t.Helper()
	opts := cfg.ProcessHotPatchMessage(patch)
	require.Len(t, opts, 1)
	return opts[0](cfg)
}

func TestModelContextContractManualBoundsAndIdentity(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	for _, tokens := range []int64{-1, 0, 1, 99999, 100000, 100001, 102400, 500000, 1000000, 1000001} {
		t.Run(strconv.FormatInt(tokens, 10), func(t *testing.T) {
			cfg := newConfig(context.Background())
			bindContext(t, cfg, contextTestModel())
			before := cfg.ModelContextStatus(0)
			patch := contextPatch(cfg, tokens)
			err := applyContextPatch(t, cfg, patch)
			if tokens < 100000 || tokens > 1000000 {
				require.Error(t, err)
				require.Equal(t, before, cfg.ModelContextStatus(0))
			} else {
				require.NoError(t, err)
				require.Equal(t, tokens, cfg.ModelContextStatus(0).ThresholdTokens)
			}
			patch.Params.ModelContextKey = "stale-config"
			require.ErrorContains(t, applyContextPatch(t, cfg, patch), "模型配置已变化")
		})
	}
}

func TestModelContextContractHotpatchResolvesCurrentModel(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	for _, mode := range []string{"omitted", "service_only", "model_only", "matching", "unrelated", "mode_only"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sessionID := uuid.NewString()
			cfg := NewTestConfig(ctx, WithPersistentSessionId(sessionID))
			model := contextTestModel()
			key := bindContext(t, cfg, model)
			_, err := yakit.CreateOrUpdateAISessionMetaStartParams(cfg.GetDB(), sessionID, &ypb.AIStartParams{
				RuntimeContextSizeLimit: 300 * 1024, ReviewPolicy: "manual",
			})
			require.NoError(t, err)
			patch := contextPatch(cfg, 600000)
			expected := int64(600000)
			switch mode {
			case "service_only":
				patch.Params.AIService = "openai"
			case "model_only":
				patch.Params.AIModelName = model.ModelName
			case "matching":
				patch.Params.AIService, patch.Params.AIModelName = "openai", model.ModelName
			case "unrelated":
				patch.Params.AIService, patch.Params.AIModelName = "other-service", "other-model"
			case "mode_only":
				expected = 500000
				disabled := true
				patch.Params.RuntimeContextSizeLimit = 0
				patch.Params.DisableContextAutoAdjust = &disabled
			}
			// The input loop persists session metadata before applying queued options.
			cfg.PersistSessionStartParamsFromHotpatch(patch)
			require.NoError(t, applyContextPatch(t, cfg, patch))
			saved, err := yakit.GetAISessionMetaStartParamsBySessionID(cfg.GetDB(), sessionID)
			require.NoError(t, err)
			require.Zero(t, saved.RuntimeContextSizeLimit, "resume must use the model setting, not a session override")
			require.Equal(t, "manual", saved.ReviewPolicy)
			require.Empty(t, saved.AIService)
			require.Empty(t, saved.AIModelName)
			provider, name, currentKey := cfg.PrimaryModelContext()
			require.Equal(t, "openai", provider)
			require.Equal(t, model.ModelName, name)
			require.Equal(t, key, currentKey)
			require.EqualValues(t, expected, cfg.GetTimeline().GetTimelineContentLimit())
			require.EqualValues(t, expected, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, name, key).ThresholdTokens)
			fresh := newConfig(ctx)
			bindContext(t, fresh, model)
			require.EqualValues(t, expected, fresh.ModelContextStatus(0).ThresholdTokens)
			require.Equal(t, mode == "mode_only", fresh.ModelContextStatus(0).DisableAutoAdjust)
		})
	}
}

func TestModelContextContractRejectedHotpatchPreservesSessionOverride(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	for _, mode := range []string{"below_minimum", "above_maximum", "zero_without_mode", "switched_model"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sessionID := uuid.NewString()
			cfg := NewTestConfig(ctx, WithPersistentSessionId(sessionID))
			model := contextTestModel()
			key := bindContext(t, cfg, model)
			_, err := yakit.CreateOrUpdateAISessionMetaStartParams(cfg.GetDB(), sessionID, &ypb.AIStartParams{
				RuntimeContextSizeLimit: 300 * 1024, ReviewPolicy: "manual",
			})
			require.NoError(t, err)
			patch := contextPatch(cfg, 600000)
			switch mode {
			case "below_minimum":
				patch.Params.RuntimeContextSizeLimit = 99999
			case "above_maximum":
				patch.Params.RuntimeContextSizeLimit = 1000001
			case "zero_without_mode":
				patch.Params.RuntimeContextSizeLimit = 0
			}
			opts := cfg.ProcessHotPatchMessage(patch)
			require.Len(t, opts, 1)
			if mode == "switched_model" {
				// Same service and model name, different configured provider. A queued
				// edit must recheck its key when applied, even without legacy fields.
				other := contextTestModel()
				other.ModelName = model.ModelName
				bindContext(t, cfg, other)
			}
			before := cfg.ModelContextStatus(0)
			cfg.PersistSessionStartParamsFromHotpatch(patch)
			require.Error(t, opts[0](cfg))
			require.Equal(t, before, cfg.ModelContextStatus(0))
			require.EqualValues(t, 500000, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "openai", model.ModelName, key).ThresholdTokens)
			saved, err := yakit.GetAISessionMetaStartParamsBySessionID(cfg.GetDB(), sessionID)
			require.NoError(t, err)
			require.EqualValues(t, 300*1024, saved.RuntimeContextSizeLimit)
			require.Equal(t, "manual", saved.ReviewPolicy)
		})
	}
}

func TestModelContextContractSuccessPersistsOnlyMatchingConfiguration(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	cfg := newConfig(context.Background())
	model := contextTestModel()
	key := bindContext(t, cfg, model)
	db := consts.GetGormProfileDatabase()
	for _, maximum := range []int64{375000, 250000} {
		require.EqualValues(t, maximum*8/10, cfg.RecordModelContextOverflow("openai", model.ModelName, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: maximum}, key))
		require.True(t, cfg.ModelContextStatus(0).Pending)
		require.EqualValues(t, 500000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)
	}
	cfg.ConfirmModelContextSuccess("openai", model.ModelName, "other-endpoint")
	require.True(t, cfg.ModelContextStatus(0).Pending)
	cfg.ConfirmModelContextSuccess("openai", model.ModelName, key)
	require.False(t, cfg.ModelContextStatus(0).Pending)
	fresh := newConfig(context.Background())
	bindContext(t, fresh, model)
	require.EqualValues(t, 200000, fresh.ModelContextStatus(0).ThresholdTokens)
	require.Equal(t, "learned", fresh.ModelContextStatus(0).Source)
	require.EqualValues(t, 500000, fresh.ModelContextStatusFor("openai", model.ModelName, 0, "other-endpoint").ThresholdTokens)
	// Explicitly resetting the setting permits a new adjustment using the same algorithm.
	require.NoError(t, applyContextPatch(t, fresh, contextPatch(fresh, 500000)))
	require.EqualValues(t, 240000, fresh.RecordModelContextOverflow("openai", model.ModelName, 400000, nil, key))
	fresh.ConfirmModelContextSuccess("openai", model.ModelName, key)
	require.EqualValues(t, 240000, fresh.ModelContextStatus(0).ThresholdTokens)
}

func TestModelContextContractLateSuccessAndCancellationDoNotOverwrite(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	for _, mode := range []string{"manual", "disabled", "cancelled", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first, second := newConfig(ctx), newConfig(context.Background())
			model := contextTestModel()
			key := bindContext(t, first, model)
			bindContext(t, second, model)
			require.EqualValues(t, 200000, first.RecordModelContextOverflow("openai", model.ModelName, 400000, &aispec.PromptOverflowEvidence{MaxContextTokens: 250000}, key))
			expected := int64(500000)
			switch mode {
			case "manual":
				expected = 300000
				require.NoError(t, applyContextPatch(t, second, contextPatch(second, expected)))
			case "disabled":
				patch := contextPatch(second, 0)
				disabled := true
				patch.Params.DisableContextAutoAdjust = &disabled
				require.NoError(t, applyContextPatch(t, second, patch))
				require.Zero(t, first.RecordModelContextOverflow("openai", model.ModelName, 300000, nil, key))
			case "cancelled":
				cancel()
			case "deleted":
				db := consts.GetGormProfileDatabase()
				global, err := yakit.GetAIGlobalConfig(db)
				require.NoError(t, err)
				kept := global.IntelligentModels[:0]
				for _, entry := range global.IntelligentModels {
					if entry.ContextOwnerId != model.ContextOwnerId {
						kept = append(kept, entry)
					}
				}
				global.IntelligentModels = kept
				_, err = yakit.SetAIGlobalConfig(db, global)
				require.NoError(t, err)
			}
			first.ConfirmModelContextSuccess("openai", model.ModelName, key)
			require.EqualValues(t, expected, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "openai", model.ModelName, key).ThresholdTokens)
		})
	}
}

func TestModelContextContractOriginalReductionFormula(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	for _, tc := range []struct {
		name              string
		measured, maximum int64
	}{
		{"reported", 280000, 200000}, {"measured", 200000, 0}, {"floor", 1000, 0}, {"not_tighter", 900000, 900000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, persisted := newConfig(context.Background()), newConfig(context.Background())
			model := contextTestModel()
			key := bindContext(t, persisted, model)
			require.NoError(t, WithTimelineContentLimit(500000)(original))
			ev := &aispec.PromptOverflowEvidence{MaxContextTokens: tc.maximum}
			want := original.RecordPromptOverflow("openai", model.ModelName, tc.measured, ev)
			require.Equal(t, want, persisted.RecordModelContextOverflow("openai", model.ModelName, tc.measured, ev, key))
		})
	}
}

func TestModelContextContractEventsAndLegacyDefaults(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	var events []ModelContextStatus
	cfg := NewTestConfig(context.Background(), WithEventHandler(func(event *schema.AiOutputEvent) {
		if event.Type == schema.EVENT_TYPE_MODEL_CONTEXT {
			var value ModelContextStatus
			require.NoError(t, json.Unmarshal(event.Content, &value))
			events = append(events, value)
		}
	}))
	require.EqualValues(t, 300*1024, cfg.GetTimelineContentSizeLimit())
	cfg.EmitModelContext(300000)
	require.Empty(t, events, "unbound SDK callbacks must not use the new persisted feature")
	model := contextTestModel()
	key := bindContext(t, cfg, model)
	cfg.EmitModelContext(300000)
	cfg.RecordModelContextOverflow("openai", model.ModelName, 300000, &aispec.PromptOverflowEvidence{MaxContextTokens: 64000}, key)
	cfg.EmitModelContext(300000)
	cfg.ConfirmModelContextSuccess("openai", model.ModelName, key)
	cfg.EmitModelContext(45000)
	require.Len(t, events, 3)
	require.EqualValues(t, 500000, events[0].ThresholdTokens)
	require.True(t, events[1].Pending)
	require.EqualValues(t, 51200, events[1].ThresholdTokens, "manual floor must not alter legacy automatic recovery")
	require.False(t, events[2].Pending)
	require.Equal(t, key, events[2].ContextKey)
	require.EqualValues(t, 300*1024, cfg.GetTimelineContentSizeLimit(), "the new setting does not overwrite auxiliary/SDK configuration")
}
