package aicommon

import (
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// promptOverflowDefaultRetryMax bounds compress-and-retry rounds per iteration
// after a confirmed prompt overflow.
const promptOverflowDefaultRetryMax = 2

// promptOverflowMinDynamicLimit keeps one overflow from collapsing the
// timeline window to nothing.
const promptOverflowMinDynamicLimit = int64(4 * 1024)

// Each provider/model has an independent pending probe. A successful call to a
// quality model must not confirm a lightweight model's failed request.
type promptOverflowLimitState struct {
	mu     sync.RWMutex
	models map[string]*promptOverflowProbe
}

type promptOverflowProbe struct {
	yakit.AIModelContext
	reportedMaximum bool
	pending         bool
	change          *ModelContextChange
	// Keep the user-visible starting point across multiple compress-and-retry rounds.
	originalThresholdTokens int64
}

type ModelContextChange struct {
	ID                      string `json:"id"`
	Reason                  string `json:"reason"` // overflow, confirmed, manual
	PreviousThresholdTokens int64  `json:"previous_threshold_tokens"`
	ThresholdTokens         int64  `json:"threshold_tokens"`
}

func newModelContextChange(reason string, previous, threshold int64) *ModelContextChange {
	return &ModelContextChange{ID: uuid.NewString(), Reason: reason, PreviousThresholdTokens: previous, ThresholdTokens: threshold}
}

func promptOverflowModelKey(provider, model string) string {
	return strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)
}

// RecordPromptOverflow stages evidence without writing durable model metadata.
// Return zero if the threshold cannot be tightened further; reported capacity
// can still be confirmed by a later successful request.
func (c *Config) RecordPromptOverflow(provider, model string, measuredPromptTokens int64, ev *aispec.PromptOverflowEvidence) int64 {
	if c == nil {
		return 0
	}
	reported := ev != nil && ev.MaxContextTokens > 0
	var target int64
	if reported {
		target = ev.MaxContextTokens * 8 / 10
	} else {
		target = measuredPromptTokens * 6 / 10
	}
	if target <= 0 {
		return 0
	}
	saved := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model)
	if saved.DisableAutoAdjust {
		return 0
	}
	base := c.EffectiveTimelineTokenLimit(provider, model, c.GetTimelineContentSizeLimit())
	target = max(target, min(promptOverflowMinDynamicLimit, base))
	tightened := target < base
	if !tightened {
		if !reported {
			return 0
		}
		target = base
	}
	key := promptOverflowModelKey(provider, model)
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.models == nil {
		state.models = make(map[string]*promptOverflowProbe)
	}
	old := state.models[key]
	if old != nil && old.ManualRevision == saved.ManualRevision && old.ThresholdTokens <= target {
		return 0
	}
	capacity := target
	if reported {
		capacity = ev.MaxContextTokens
	}
	original := base
	if old != nil && old.pending && old.ManualRevision == saved.ManualRevision {
		original = old.originalThresholdTokens
	}
	state.models[key] = &promptOverflowProbe{
		AIModelContext:  yakit.AIModelContext{ContextTokens: capacity, ThresholdTokens: target, Source: "learned", ManualRevision: saved.ManualRevision},
		reportedMaximum: reported, pending: true, change: newModelContextChange("overflow", base, target),
		originalThresholdTokens: original,
	}
	if !tightened {
		return 0
	}
	return target
}

func (c *Config) EffectiveTimelineTokenLimit(provider, model string, base int64) int64 {
	if c == nil {
		return base
	}
	saved := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model)
	// Once a model has a record, an old session's startup default must not
	// override it (including a manual increase above that old default).
	if saved.Source != "default" || base <= 0 || base == int64(DefaultTimelineContentSizeLimit) || saved.ThresholdTokens < base {
		base = saved.ThresholdTokens
	}
	state := &c.promptOverflowLimit
	state.mu.RLock()
	defer state.mu.RUnlock()
	probe := state.models[promptOverflowModelKey(provider, model)]
	// A manual update from another session supersedes an in-flight probe.
	if probe != nil && (probe.pending || strings.TrimSpace(model) == "") && probe.ManualRevision == saved.ManualRevision {
		return min(base, probe.ThresholdTokens)
	}
	return base
}

func (c *Config) ResetPromptOverflowLimit() {
	if c == nil {
		return
	}
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	state.models = nil
}

func (c *Config) resetModelPromptOverflowLimit(provider, model string) {
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.models, promptOverflowModelKey(provider, model))
}

// Confirmation requires successful provider acceptance, not merely constructing
// a shorter prompt. Cancellation and late results after manual edits never save.
func (c *Config) ConfirmPromptContextSuccess(provider, model string, acceptedPromptTokens ...int64) {
	if c == nil || (c.Ctx != nil && c.Ctx.Err() != nil) {
		return
	}
	state := &c.promptOverflowLimit
	state.mu.Lock()
	defer state.mu.Unlock()
	key := promptOverflowModelKey(provider, model)
	probe := state.models[key]
	if probe == nil || !probe.pending {
		return
	}
	previous := probe.originalThresholdTokens
	if !probe.reportedMaximum && len(acceptedPromptTokens) > 0 {
		probe.ThresholdTokens = max(probe.ThresholdTokens, acceptedPromptTokens[0])
		probe.ContextTokens = probe.ThresholdTokens
	}
	if strings.TrimSpace(model) == "" {
		// SDK callbacks may omit identity. Keep their recovery local to this Config.
		probe.pending = false
		probe.change = newModelContextChange("confirmed", previous, probe.ThresholdTokens)
		return
	}
	if err := yakit.SaveAIModelContext(consts.GetGormProfileDatabase(), provider, model, probe.AIModelContext); err != nil {
		if err == yakit.ErrAIModelContextChanged {
			delete(state.models, key)
		} else {
			log.Warnf("save learned model context: %v", err)
		}
		return
	}
	probe.AIModelContext = yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model)
	probe.pending = false
	probe.change = newModelContextChange("confirmed", previous, probe.ThresholdTokens)
}

type ModelContextStatus struct {
	yakit.AIModelContext
	Provider               string              `json:"provider"`
	Model                  string              `json:"model"`
	PromptTokens           int64               `json:"prompt_tokens"`
	Pending                bool                `json:"pending"`
	TooSmall               bool                `json:"too_small"`
	MinimumThresholdTokens int64               `json:"minimum_threshold_tokens"`
	Change                 *ModelContextChange `json:"change,omitempty"`
	Error                  string              `json:"error,omitempty"`
}

func (c *Config) ModelContextStatus(promptTokens int64) ModelContextStatus {
	c.m.Lock()
	provider, model := c.AiServerName, c.AiModelName
	c.m.Unlock()
	return c.ModelContextStatusFor(provider, model, promptTokens)
}

func (c *Config) ModelContextStatusFor(provider, model string, promptTokens int64) ModelContextStatus {
	status := ModelContextStatus{AIModelContext: yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model), Provider: provider, Model: model, PromptTokens: promptTokens, MinimumThresholdTokens: yakit.MinimumAIModelContextTokens}
	status.ThresholdTokens = c.EffectiveTimelineTokenLimit(provider, model, c.GetTimelineContentSizeLimit())
	state := &c.promptOverflowLimit
	state.mu.RLock()
	if probe := state.models[promptOverflowModelKey(provider, model)]; probe != nil && probe.ManualRevision == status.ManualRevision {
		status.Change = probe.change
		if probe.pending || strings.TrimSpace(model) == "" {
			status.ContextTokens, status.Pending, status.Source = probe.ContextTokens, probe.pending, "learned"
		}
	}
	state.mu.RUnlock()
	status.TooSmall = status.ContextTokens < yakit.MinimumAIModelContextTokens
	return status
}

func (c *Config) EmitModelContext(promptTokens int64) {
	c.emitModelContextStatus(c.ModelContextStatus(promptTokens))
}

func (c *Config) EmitModelContextFor(provider, model string, promptTokens int64) {
	c.emitModelContextStatus(c.ModelContextStatusFor(provider, model, promptTokens))
}

func (c *Config) emitModelContextStatus(status ModelContextStatus) {
	if c.Emitter != nil {
		c.Emitter.EmitJSON(schema.EVENT_TYPE_MODEL_CONTEXT, "model_context", status)
	}
}

// GetPromptOverflowRetryMax returns the bounded retry budget. 0 disables the
// compress-and-retry path.
func (c *Config) GetPromptOverflowRetryMax() int {
	if c == nil {
		return 0
	}
	return c.promptOverflowRetryMax
}
