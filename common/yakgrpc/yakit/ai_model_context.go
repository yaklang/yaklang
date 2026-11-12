package yakit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
)

// Token display units are decimal. Persisted values are integer token counts,
// including budgets saved by versions that displayed K using a factor of 1024.
const DefaultAIModelContextTokens int64 = 500 * 1000
const MinimumAIModelContextTokens int64 = 100 * 1000

// AIModelContext is local model metadata, never sent as a provider API parameter.
// Unknown models start with an optimistic window; learned values are only saved
// after the provider accepts a smaller request.
type AIModelContext struct {
	ContextTokens     int64  `json:"context_tokens"`
	ThresholdTokens   int64  `json:"threshold_tokens"`
	Source            string `json:"source"` // default, learned, manual
	DisableAutoAdjust bool   `json:"disable_auto_adjust"`
	ManualRevision    string `json:"manual_revision,omitempty"`
}

var aiModelContextMu sync.Mutex
var ErrAIModelContextChanged = errors.New("model context was manually changed while learning")

// Zero means follow the model's saved budget. The floor applies to user input,
// not to evidence that a provider really has a smaller context window.
func ValidateAIModelContextThreshold(tokens int64) error {
	if tokens != 0 && tokens < MinimumAIModelContextTokens {
		return fmt.Errorf("上下文压缩阈值不能低于 100K (%d tokens)", MinimumAIModelContextTokens)
	}
	return nil
}

// Match the slider's stable upper bound; user input must never expand its own range.
func AIModelContextMaximumThreshold(value AIModelContext) int64 {
	const step int64 = 1000 * 1000
	largest := max(step, value.ContextTokens, value.ThresholdTokens)
	return ((largest-1)/step + 1) * step
}

// SetAIModelContextSettings applies a user edit atomically. A mode-only edit preserves
// the threshold, including a learned value below the manual input floor.
func SetAIModelContextSettings(db *gorm.DB, provider, model string, threshold int64, disabled *bool) error {
	aiModelContextMu.Lock()
	defer aiModelContextMu.Unlock()
	if db == nil || strings.TrimSpace(model) == "" {
		return fmt.Errorf("model context requires a database and model identity")
	}
	value := GetAIModelContext(db, provider, model)
	if threshold != 0 || disabled == nil {
		if threshold < MinimumAIModelContextTokens {
			return fmt.Errorf("上下文压缩阈值不能低于 100K (%d tokens)", MinimumAIModelContextTokens)
		}
		if threshold > AIModelContextMaximumThreshold(value) {
			return fmt.Errorf("上下文压缩阈值不能超过滑条上限 (%d tokens)", AIModelContextMaximumThreshold(value))
		}
		value.ThresholdTokens, value.Source = threshold, "manual"
	}
	if disabled != nil {
		value.DisableAutoAdjust = *disabled
	}
	value.ManualRevision = uuid.NewString()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return SetKey(db, aiModelContextKey(provider, model), string(raw))
}

func aiModelContextKey(provider, model string) string {
	return fmt.Sprintf("ai-model-context:%x", sha256.Sum256([]byte(strings.TrimSpace(provider)+"\x00"+strings.TrimSpace(model))))
}

func GetAIModelContext(db *gorm.DB, provider, model string) AIModelContext {
	value := AIModelContext{ContextTokens: DefaultAIModelContextTokens, ThresholdTokens: DefaultAIModelContextTokens, Source: "default"}
	if db == nil || strings.TrimSpace(model) == "" {
		return value
	}
	var saved AIModelContext
	if json.Unmarshal([]byte(GetKey(db, aiModelContextKey(provider, model))), &saved) == nil && saved.ContextTokens > 0 && saved.ThresholdTokens > 0 {
		return saved
	}
	return value
}

// SaveAIModelContext updates one model independently of the global routing
// configuration. Concurrent successful probes may only lower a learned budget.
func SaveAIModelContext(db *gorm.DB, provider, model string, value AIModelContext) error {
	aiModelContextMu.Lock()
	defer aiModelContextMu.Unlock()
	if value.ContextTokens <= 0 || value.ThresholdTokens <= 0 {
		return fmt.Errorf("invalid model context budget")
	}
	if value.Source == "manual" {
		previous := GetAIModelContext(db, provider, model)
		if value.ThresholdTokens > AIModelContextMaximumThreshold(previous) {
			return fmt.Errorf("上下文压缩阈值不能超过滑条上限 (%d tokens)", AIModelContextMaximumThreshold(previous))
		}
		value.DisableAutoAdjust = previous.DisableAutoAdjust
		if err := ValidateAIModelContextThreshold(value.ThresholdTokens); err != nil {
			return err
		}
		value.ManualRevision = uuid.NewString()
	} else if value.Source != "learned" {
		return fmt.Errorf("invalid model context source: %s", value.Source)
	}
	if db == nil || strings.TrimSpace(model) == "" {
		return fmt.Errorf("model context requires a database and model identity")
	}
	if value.Source == "learned" {
		previous := GetAIModelContext(db, provider, model)
		if previous.DisableAutoAdjust || previous.ManualRevision != value.ManualRevision {
			return ErrAIModelContextChanged
		}
		if previous.Source != "default" {
			value.ContextTokens = min(value.ContextTokens, previous.ContextTokens)
			value.ThresholdTokens = min(value.ThresholdTokens, previous.ThresholdTokens)
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return SetKey(db, aiModelContextKey(provider, model), string(raw))
}
