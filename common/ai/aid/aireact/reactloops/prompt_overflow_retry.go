package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// defaultPromptOverflowRetryMax mirrors the aicommon default for configs that do
// not expose the retry budget.
const defaultPromptOverflowRetryMax = 2

// promptOverflowRetryMax reads the bounded retry budget. 0 disables the retry.
func (r *ReActLoop) promptOverflowRetryMax() int {
	if provider, ok := r.config.(interface{ GetPromptOverflowRetryMax() int }); ok {
		if max := provider.GetPromptOverflowRetryMax(); max >= 0 {
			return max
		}
	}
	return defaultPromptOverflowRetryMax
}

// Apply quality settings before measurement. Auxiliary loops retain their
// original runtime-only threshold, including when they share a Config.
func (r *ReActLoop) syncPromptOverflowTimelineLimit() {
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return
	}
	timeline := config.GetTimeline()
	if timeline == nil {
		return
	}
	base := config.GetTimelineContentSizeLimit()
	if base <= 0 {
		return
	}
	effective := config.EffectiveTimelineTokenLimit(config.AiServerName, config.AiModelName, base)
	if provider, model, key := r.primaryModelContext(); key != "" {
		effective = config.EffectiveModelContextLimit(provider, model, base, key)
		if effective > 0 {
			timeline.SetTimelineContentLimit(effective)
		}
		return
	}
	if _, _, key := config.PrimaryModelContext(); key != "" && r.useSpeedPriorityAI {
		timeline.SetTimelineContentLimit(effective)
		return
	}
	if effective <= 0 || effective >= base {
		return
	}
	timeline.SetTimelineContentLimit(effective)
}

// reduceTimelineLimitForPromptOverflow records the confirmed overflow for the
// model that rejected the prompt and lowers the timeline limit for the rebuild.
// It returns false when no tighter limit is available, so the caller stops.
func (r *ReActLoop) noteFittingTimelineThroughID(id *int64) {
	if r == nil || id == nil {
		return
	}
	copied := *id
	r.lastFittingTimelineThroughID = &copied
}

func (r *ReActLoop) reduceTimelineLimitForPromptOverflow(overflowErr error, prompt string) bool {
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return false
	}
	evidence, ok := aicommon.PromptOverflowEvidenceFrom(overflowErr)
	if !ok {
		return false
	}
	measured := int64(r.completeInputTokens(prompt))
	var limit int64
	if _, _, key := r.primaryModelContext(); key != "" {
		// Overflow evidence belongs to the request that failed, which may finish
		// after the selected model changes. Never fall back to name-only recovery
		// for a quality request with a concrete context identity.
		if evidence.ContextKey == "" {
			return false
		}
		limit = config.RecordModelContextOverflow(evidence.Provider, evidence.Model, measured, &evidence, evidence.ContextKey)
		config.EmitModelContextFor(evidence.Provider, evidence.Model, measured, evidence.ContextKey)
	} else {
		limit = config.RecordPromptOverflow(evidence.Provider, evidence.Model, measured, &evidence)
	}
	if limit <= 0 {
		return false
	}
	timeline := config.GetTimeline()
	if timeline == nil {
		return false
	}
	timeline.SetTimelineContentLimit(limit)
	return true
}

// The new persisted setting belongs to the quality loop only. Keep auxiliary
// Speed loops on the original runtime-only limit and retry mechanism.
func (r *ReActLoop) primaryModelContext() (provider, model, key string) {
	if r.useSpeedPriorityAI {
		return
	}
	if r.requestAIBinding != nil && r.requestAIBinding.ModelConfig != nil {
		modelConfig := r.requestAIBinding.ModelConfig
		return modelConfig.GetProvider().GetType(), modelConfig.GetModelName(), aispec.ModelContextIdentity(modelConfig)
	}
	if config, ok := r.config.(*aicommon.Config); ok {
		return config.PrimaryModelContext()
	}
	return
}
func (r *ReActLoop) promptOverflowAutoAdjustDisabled(err error) bool {
	_, _, key := r.primaryModelContext()
	evidence, ok := aicommon.PromptOverflowEvidenceFrom(err)
	return ok && key != "" && evidence.ContextKey != "" && yakit.GetAIModelContext(consts.GetGormProfileDatabase(), evidence.Provider, evidence.Model, evidence.ContextKey).DisableAutoAdjust
}

func (r *ReActLoop) confirmModelContextSuccess(descriptor *LoopResultDescriptor, tokens int64) {
	if _, _, key := r.primaryModelContext(); key == "" || descriptor == nil {
		return
	}
	identity := descriptor.Snapshot()
	if identity.ContextKey == "" {
		return
	}
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return
	}
	config.ConfirmModelContextSuccess(identity.Provider, identity.Model, identity.ContextKey)
	config.EmitModelContextFor(identity.Provider, identity.Model, tokens, identity.ContextKey)
}
