package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// PlanningReviewControl is a callback that reviews plan or task decisions
// and returns appropriate suggestion params (e.g. {"suggestion": "continue"}).
// Returning an error causes fallback to the default auto-continue behavior.
type PlanningReviewControl func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error)

// DefaultAIPlanReviewControl is the default plan review callback.
// It simply returns auto-continue, matching the legacy YOLO behavior.
// Replace with an AI-driven implementation to enable intelligent plan review.
func DefaultAIPlanReviewControl(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
	log.Debugf("dynamic planning: plan review auto-continue (default)")
	return aitool.InvokeParams{"suggestion": "continue"}, nil
}

// DefaultAITaskReviewControl is the default task review callback.
// It simply returns auto-continue, matching the legacy YOLO behavior.
// Replace with an AI-driven implementation to enable intelligent task review.
func DefaultAITaskReviewControl(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
	log.Debugf("dynamic planning: task review auto-continue (default)")
	return aitool.InvokeParams{"suggestion": "continue"}, nil
}
