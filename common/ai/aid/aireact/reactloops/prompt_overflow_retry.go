package reactloops

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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

// syncPromptOverflowTimelineLimit applies the per-model dynamic threshold before
// the pre-prompt compression measures, including restored model settings and
// model switches. The configured session limit remains separate.
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
	if effective <= 0 {
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

// Shrink the prefix sent for compression, preserving the newer timeline entries
// for subsequent turns. This also recovers when the first request is too large
// and no successful timeline boundary exists yet.
func (r *ReActLoop) narrowOverflowCompressionBoundary(rejectedThroughID *int64) {
	if r.lastFittingTimelineThroughID != nil && rejectedThroughID != nil && *r.lastFittingTimelineThroughID < *rejectedThroughID {
		return
	}
	timeline := timelineFromConfig(r.config)
	if timeline == nil {
		return
	}
	var through []int64
	if rejectedThroughID != nil {
		through = []int64{*rejectedThroughID}
	}
	candidates, err := timeline.CompressionCandidates(through...)
	if err != nil || len(candidates) < 2 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	id := candidates[(len(candidates)-1)/2].ID
	r.lastFittingTimelineThroughID = &id
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
	if evidence.Provider == "" {
		evidence.Provider = config.AiServerName
	}
	if evidence.Model == "" {
		evidence.Model = config.AiModelName
	}
	if r.promptMode == aicommon.LoopPromptModeNormal && evidence.Model != "" {
		// A gateway fallback or a Speed loop can use a different model than the
		// initially configured quality model. Rebuild against the failed model.
		config.UpdateAIModelInfo(evidence.Provider, evidence.Model)
	}
	measured := int64(aicommon.MeasureTokens(prompt))
	limit := config.RecordPromptOverflow(evidence.Provider, evidence.Model, measured, &evidence)
	capacity := evidence.MaxContextTokens
	if capacity <= 0 {
		capacity = limit
	}
	if capacity > 0 && capacity < yakit.MinimumAIModelContextTokens {
		r.emitContextCompressionWarning(fmt.Sprintf("模型上下文过小（%d tokens，小于 100K），请切换到支持更长上下文的模型。", capacity))
	}
	config.EmitModelContextFor(evidence.Provider, evidence.Model, measured)
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

// Resolve the model that actually rejected the request, including gateway fallbacks.
func (r *ReActLoop) promptOverflowAutoAdjustDisabled(overflowErr error) bool {
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return false
	}
	evidence, ok := aicommon.PromptOverflowEvidenceFrom(overflowErr)
	if !ok {
		return false
	}
	if evidence.Provider == "" {
		evidence.Provider = config.AiServerName
	}
	if evidence.Model == "" {
		evidence.Model = config.AiModelName
	}
	return yakit.GetAIModelContext(consts.GetGormProfileDatabase(), evidence.Provider, evidence.Model).DisableAutoAdjust
}
