package aicommon

import (
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// promptOverflowDefaultRetryMax bounds compress-and-retry rounds per iteration
// after a confirmed prompt overflow.
const promptOverflowDefaultRetryMax = 2

// promptOverflowMinDynamicLimit keeps one overflow from collapsing the
// timeline window to nothing.
const promptOverflowMinDynamicLimit = int64(4 * 1024)

// promptOverflowLimitState holds the per-model dynamic threshold produced by a
// confirmed overflow. It is runtime state, never persisted configuration.
type promptOverflowLimitState struct {
	mu        sync.RWMutex
	modelKey  string
	limit     int64
	updatedAt time.Time
}

func promptOverflowModelKey(provider, model string) string {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" && model == "" {
		return "unknown|unknown"
	}
	return provider + "|" + model
}

// promptOverflowKeysMatch reports whether a recorded key applies to the given
// model. The same model reached through a different gateway keeps its window,
// so an exact key or an identical model name both match.
func promptOverflowKeysMatch(recorded, provider, model string) bool {
	if recorded == "" {
		return false
	}
	if recorded == promptOverflowModelKey(provider, model) {
		return true
	}
	_, recordedModel, ok := strings.Cut(recorded, "|")
	model = strings.TrimSpace(model)
	return ok && recordedModel != "" && recordedModel != "unknown" && recordedModel == model
}

// RecordPromptOverflow stores a smaller timeline content limit for the model
// that rejected the prompt and returns the new limit. It returns 0 when no
// smaller limit is available, which means another compression round would not
// tighten anything and the caller must not retry.
func (c *Config) RecordPromptOverflow(provider, model string, measuredPromptTokens int64, ev *aispec.PromptOverflowEvidence) int64 {
	if c == nil {
		return 0
	}
	var target int64
	if ev != nil && ev.MaxContextTokens > 0 {
		target = ev.MaxContextTokens * 8 / 10
	} else if measuredPromptTokens > 0 {
		target = measuredPromptTokens * 6 / 10
	}
	if target <= 0 {
		return 0
	}
	base := c.GetTimelineContentSizeLimit()
	if base > 0 {
		floor := promptOverflowMinDynamicLimit
		if floor > base {
			floor = base
		}
		if target < floor {
			target = floor
		}
		if target >= base {
			// The configured threshold is already the tighter one.
			return 0
		}
	}
	key := promptOverflowModelKey(provider, model)
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.modelKey == key && state.limit > 0 && state.limit <= target {
		// Same model, no tighter limit than the one already recorded.
		return 0
	}
	state.modelKey, state.limit, state.updatedAt = key, target, time.Now()
	return target
}

// EffectiveTimelineTokenLimit returns the threshold the timeline should use for
// this model. A dynamic limit only applies to the model that produced it.
func (c *Config) EffectiveTimelineTokenLimit(provider, model string, base int64) int64 {
	if c == nil {
		return base
	}
	state := &c.promptOverflowLimit
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.limit <= 0 || state.modelKey == "" {
		return base
	}
	if !promptOverflowKeysMatch(state.modelKey, provider, model) {
		return base
	}
	if base > 0 && base < state.limit {
		return base
	}
	return state.limit
}

// ResetPromptOverflowLimit drops the dynamic limit, for example on session
// reset or when debugging.
func (c *Config) ResetPromptOverflowLimit() {
	if c == nil {
		return
	}
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	state.modelKey, state.limit, state.updatedAt = "", 0, time.Time{}
}

// GetPromptOverflowRetryMax returns the bounded retry budget. 0 disables the
// compress-and-retry path.
func (c *Config) GetPromptOverflowRetryMax() int {
	if c == nil {
		return 0
	}
	return c.promptOverflowRetryMax
}
