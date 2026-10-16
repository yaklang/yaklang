package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
// the pre-prompt compression measures. It only ever lowers the limit: a session
// that already had to compress must not silently go back to the full window.
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
	if effective <= 0 || effective >= base {
		return
	}
	timeline.SetTimelineContentLimit(effective)
}

// reduceTimelineLimitForPromptOverflow records the confirmed overflow for the
// model that rejected the prompt and lowers the timeline limit for the rebuild.
// It returns false when no tighter limit is available, so the caller stops.
func (r *ReActLoop) reduceTimelineLimitForPromptOverflow(overflowErr error, prompt string) bool {
	config, ok := r.config.(*aicommon.Config)
	if !ok || config == nil {
		return false
	}
	evidence, ok := aicommon.PromptOverflowEvidenceFrom(overflowErr)
	if !ok {
		return false
	}
	measured := int64(aicommon.MeasureTokens(prompt))
	limit := config.RecordPromptOverflow(evidence.Provider, evidence.Model, measured, &evidence)
	if limit <= 0 {
		return false
	}
	timeline := config.GetTimeline()
	if timeline == nil {
		return false
	}
	timeline.SetTimelineContentLimit(limit)
	// A confirmed overflow is new evidence that compression is required; do not
	// let an earlier compression failure suppress this attempt for a minute.
	timeline.ClearCompressionBackoff()
	return true
}
