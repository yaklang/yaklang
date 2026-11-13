package aicommon

import (
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Pending adjustments are isolated by concrete quality-model configuration.
type modelContextState struct {
	mu     sync.RWMutex
	models map[string]*modelContextProbe
}

type modelContextProbe struct {
	yakit.AIModelContext
	pending bool
	change  *ModelContextChange
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

func modelContextProbeKey(provider, model, contextKey string) string {
	return contextKey + "\x00" + strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(model)
}

// RecordModelContextOverflow stages evidence without writing durable model metadata.
// Return zero if the threshold cannot be tightened further, as in the original
// runtime-only overflow recovery. This layer adds persistence, not another retry.
func (c *Config) RecordModelContextOverflow(provider, model string, measuredPromptTokens int64, ev *aispec.PromptOverflowEvidence, contextKey string) int64 {
	if c == nil || contextKey == "" || model == "" {
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
	saved := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model, contextKey)
	if saved.DisableAutoAdjust {
		return 0
	}
	base := c.EffectiveModelContextLimit(provider, model, c.GetTimelineContentSizeLimit(), contextKey)
	target = max(target, min(promptOverflowMinDynamicLimit, base))
	if target >= base {
		return 0
	}
	key := modelContextProbeKey(provider, model, contextKey)
	state := &c.modelContextState
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.models == nil {
		state.models = make(map[string]*modelContextProbe)
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
	state.models[key] = &modelContextProbe{
		AIModelContext: yakit.AIModelContext{ContextTokens: capacity, ThresholdTokens: target, Source: "learned", ManualRevision: saved.ManualRevision},
		pending:        true, change: newModelContextChange("overflow", base, target),
		originalThresholdTokens: original,
	}
	return target
}

func (c *Config) EffectiveModelContextLimit(provider, model string, base int64, contextKey string) int64 {
	if c == nil {
		return base
	}
	saved := yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model, contextKey)
	// Once a model has a record, an old session's startup default must not
	// override it (including a manual increase above that old default).
	if saved.Source != "default" || base <= 0 || base == int64(DefaultTimelineContentSizeLimit) || saved.ThresholdTokens < base {
		base = saved.ThresholdTokens
	}
	state := &c.modelContextState
	state.mu.RLock()
	defer state.mu.RUnlock()
	probe := state.models[modelContextProbeKey(provider, model, contextKey)]
	// A manual update from another session supersedes an in-flight probe.
	if probe != nil && probe.pending && probe.ManualRevision == saved.ManualRevision {
		return c.EffectiveTimelineTokenLimit(provider, model, min(base, probe.ThresholdTokens))
	}
	return c.EffectiveTimelineTokenLimit(provider, model, base)
}

func (c *Config) ResetModelContextLimit() {
	if c == nil {
		return
	}
	state := &c.modelContextState
	state.mu.Lock()
	defer state.mu.Unlock()
	state.models = nil
}

func (c *Config) resetModelContextLimit(provider, model, contextKey string) {
	state := &c.modelContextState
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.models, modelContextProbeKey(provider, model, contextKey))
}

// Confirmation requires successful provider acceptance, not merely constructing
// a shorter prompt. Cancellation and late results after manual edits never save.
func (c *Config) ConfirmModelContextSuccess(provider, model, contextKey string) {
	if c == nil || (c.Ctx != nil && c.Ctx.Err() != nil) {
		return
	}
	state := &c.modelContextState
	state.mu.Lock()
	defer state.mu.Unlock()
	key := modelContextProbeKey(provider, model, contextKey)
	probe := state.models[key]
	if probe == nil || !probe.pending {
		return
	}
	previous := probe.originalThresholdTokens
	if err := yakit.SaveAIModelContext(consts.GetGormProfileDatabase(), provider, model, probe.AIModelContext, contextKey); err != nil {
		if err == yakit.ErrAIModelContextChanged || err == yakit.ErrAIModelContextOwnerDeleted {
			delete(state.models, key)
		} else {
			log.Warnf("save learned model context: %v", err)
		}
		return
	}
	probe.AIModelContext = yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model, contextKey)
	probe.pending = false
	probe.change = newModelContextChange("confirmed", previous, probe.ThresholdTokens)
}

type ModelContextStatus struct {
	yakit.AIModelContext
	ContextKey             string              `json:"context_key"`
	Provider               string              `json:"provider"`
	Model                  string              `json:"model"`
	PromptTokens           int64               `json:"prompt_tokens"`
	Pending                bool                `json:"pending"`
	TooSmall               bool                `json:"too_small"`
	MinimumThresholdTokens int64               `json:"minimum_threshold_tokens"`
	Change                 *ModelContextChange `json:"change,omitempty"`
	Error                  string              `json:"error,omitempty"`
}

// PrimaryModelContext returns only the quality-tier binding. Auxiliary loops do
// not opt in, even when single-model mode uses the same physical model.
func (c *Config) PrimaryModelContext() (provider, model, key string) {
	if c == nil || !c.modelContextSettingsEnabled {
		return
	}
	config := c.primaryModelContextConfig
	if config == nil {
		config = aiconfig.GetGlobalManager().GetFirstConfig(consts.TierIntelligent)
	}
	if config == nil {
		return
	}
	return config.GetProvider().GetType(), config.GetModelName(), aispec.ModelContextIdentity(config)
}

// WithPrimaryModelContext binds an explicit quality configuration (also useful
// for SDK callers). Normal tiered calls resolve the current quality config.
func WithPrimaryModelContext(model *ypb.AIModelConfig) ConfigOption {
	return func(c *Config) error {
		c.modelContextSettingsEnabled = model != nil
		c.primaryModelContextConfig = model
		return nil
	}
}

func (c *Config) ModelContextStatus(promptTokens int64) ModelContextStatus {
	provider, model, key := c.PrimaryModelContext()
	return c.ModelContextStatusFor(provider, model, promptTokens, key)
}

func (c *Config) ModelContextStatusFor(provider, model string, promptTokens int64, contextKey string) ModelContextStatus {
	status := ModelContextStatus{AIModelContext: yakit.GetAIModelContext(consts.GetGormProfileDatabase(), provider, model, contextKey), ContextKey: contextKey, Provider: provider, Model: model, PromptTokens: promptTokens, MinimumThresholdTokens: yakit.MinimumAIModelContextTokens}
	status.ThresholdTokens = c.EffectiveModelContextLimit(provider, model, c.GetTimelineContentSizeLimit(), contextKey)
	state := &c.modelContextState
	state.mu.RLock()
	if probe := state.models[modelContextProbeKey(provider, model, contextKey)]; probe != nil && probe.ManualRevision == status.ManualRevision {
		status.Change = probe.change
		if probe.pending {
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

func (c *Config) EmitModelContextFor(provider, model string, promptTokens int64, contextKey string) {
	c.emitModelContextStatus(c.ModelContextStatusFor(provider, model, promptTokens, contextKey))
}

func (c *Config) emitModelContextStatus(status ModelContextStatus) {
	if c.Emitter != nil && status.ContextKey != "" {
		c.Emitter.EmitJSON(schema.EVENT_TYPE_MODEL_CONTEXT, "model_context", status)
	}
}
