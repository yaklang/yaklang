package aicommon

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

// AICallTraceRecord is an opt-in diagnostic record for one top-level AI call.
// It intentionally contains the complete prompt and model output and must only
// be written to a user-selected local trace directory.
type AICallTraceRecord struct {
	Timestamp       time.Time         `json:"timestamp"`
	DurationMS      int64             `json:"duration_ms"`
	Tier            string            `json:"tier,omitempty"`
	Sequence        int64             `json:"sequence,omitempty"`
	CallerLabel     string            `json:"caller_label,omitempty"`
	TaskIndex       string            `json:"task_index,omitempty"`
	Provider        string            `json:"provider,omitempty"`
	Model           string            `json:"model,omitempty"`
	ThinkingLevel   string            `json:"thinking_level,omitempty"`
	HTTPStatusCode  int               `json:"http_status_code,omitempty"`
	Prompt          string            `json:"prompt"`
	Output          string            `json:"output,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	RawHTTPResponse string            `json:"raw_http_response,omitempty"`
	Error           string            `json:"error,omitempty"`
	Usage           *aispec.ChatUsage `json:"usage,omitempty"`
}

type AICallTraceHook func(AICallTraceRecord)

// WithAICallTraceHook installs an opt-in diagnostic hook after normalized
// prompt construction and response completion. It does not alter AI behavior.
func WithAICallTraceHook(hook AICallTraceHook) ConfigOption {
	return func(c *Config) error {
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.aiCallTraceHook = hook
		c.m.Unlock()
		return nil
	}
}

func (c *Config) getAICallTraceHook() AICallTraceHook {
	if c == nil {
		return nil
	}
	if c.m == nil {
		return c.aiCallTraceHook
	}
	c.m.Lock()
	defer c.m.Unlock()
	return c.aiCallTraceHook
}

func emitAICallTrace(hook AICallTraceHook, record AICallTraceRecord) {
	if hook == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("AI call trace hook panicked: %v", recovered)
		}
	}()
	hook(record)
}
