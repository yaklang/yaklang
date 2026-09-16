package aicommon

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const aiRateLimitMaxAttempts = 8
const aiRateLimitMaxWait = 2 * time.Minute

// Shared by request and transaction retries so nested loops cannot multiply
// the budget. Each transaction has its own budget, independent of other tasks.
type rateLimitBudget struct {
	mu        sync.Mutex
	started   time.Time
	attempts  int
	waited    time.Duration
	exhausted bool
}

func newRateLimitBudget() *rateLimitBudget {
	return &rateLimitBudget{started: time.Now()}
}

func (b *rateLimitBudget) reserve(delay time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempts++
	if b.exhausted || b.attempts >= aiRateLimitMaxAttempts || b.waited+delay > aiRateLimitMaxWait || time.Since(b.started)+delay > aiRateLimitMaxWait {
		b.exhausted = true
		return false
	}
	b.waited += delay
	return true
}

// AIRateLimitError keeps an unavailable provider distinguishable from an empty
// model response. A terminal error must not be retried by an outer AI loop.
type AIRateLimitError struct {
	Provider       string
	Model          string
	StatusCode     int
	Code           json.RawMessage
	Message        string
	Attempts       int
	WaitDuration   time.Duration
	BudgetExceeded bool
}

func (e *AIRateLimitError) Error() string {
	return fmt.Sprintf("AI provider HTTP %d (provider=%s model=%s code=%s attempts=%d waited=%s budget_exceeded=%t): %s", e.StatusCode, e.Provider, e.Model, e.Code, e.Attempts, e.WaitDuration, e.BudgetExceeded, e.Message)
}

func rateLimitError(b *rateLimitBudget, rsp *AIResponse) *AIRateLimitError {
	e := &AIRateLimitError{Provider: rsp.GetProviderName(), Model: rsp.GetModelName(), StatusCode: rsp.GetHTTPStatusCode(), Message: string(rsp.GetHTTPResponseBody())}
	if body := parse429Body(rsp); body != nil {
		e.Code = body.Error.Code
		if body.Error.Message != "" {
			e.Message = body.Error.Message
		}
	}
	if b != nil {
		b.mu.Lock()
		e.Attempts, e.WaitDuration, e.BudgetExceeded = b.attempts, b.waited, b.exhausted
		b.mu.Unlock()
	}
	return e
}
