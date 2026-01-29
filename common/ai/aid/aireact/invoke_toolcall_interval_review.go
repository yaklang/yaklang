package aireact

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// _invokeToolCall_IntervalReview is called periodically during tool execution to review progress.
// It returns true if the tool should continue, false if it should be cancelled.
func (r *ReAct) _invokeToolCall_IntervalReview(
	ctx context.Context,
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
) (bool, error) {
	// Check context at the beginning
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Generate the interval review prompt
	prompt, err := r.promptManager.GenerateIntervalReviewPrompt(tool, params, stdoutSnapshot, stderrSnapshot)
	if err != nil {
		log.Errorf("failed to generate interval review prompt: %v", err)
		// If we can't generate the prompt, continue by default
		return true, nil
	}

	var shouldContinue = true
	var reviewReason string

	transErr := aicommon.CallAITransaction(r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(
				ctx,
				rsp.GetOutputStreamReader("interval-review", true, r.Emitter),
				"interval-toolcall-review",
			)
			if err != nil {
				log.Errorf("failed to extract interval review action: %v", err)
				// If extraction fails, continue by default
				return nil
			}

			decision := action.GetString("decision")
			reviewReason = action.GetString("reason")
			progressSummary := action.GetString("progress_summary")

			switch decision {
			case "continue":
				shouldContinue = true
				if progressSummary != "" {
					r.AddToTimeline("interval-review-continue", fmt.Sprintf(
						"Tool [%s] execution continues. Progress: %s. Reason: %s",
						tool.Name, progressSummary, reviewReason,
					))
				}
				log.Infof("interval review: tool [%s] should continue. Reason: %s", tool.Name, reviewReason)
			case "cancel":
				shouldContinue = false
				r.AddToTimeline("interval-review-cancel", fmt.Sprintf(
					"Tool [%s] execution cancelled by interval review. Reason: %s",
					tool.Name, reviewReason,
				))
				log.Warnf("interval review: tool [%s] should be cancelled. Reason: %s", tool.Name, reviewReason)
			default:
				// Unknown decision, continue by default
				shouldContinue = true
				log.Warnf("interval review: unknown decision '%s', continuing by default", decision)
			}

			// Log any concerns
			concerns := action.GetStringSlice("concerns", nil)
			if len(concerns) > 0 {
				for _, concern := range concerns {
					log.Warnf("interval review concern for tool [%s]: %s", tool.Name, concern)
				}
			}

			return nil
		},
	)

	if transErr != nil {
		log.Errorf("interval review transaction failed: %v", transErr)
		// If the transaction fails, continue by default
		return true, nil
	}

	return shouldContinue, nil
}

// CreateIntervalReviewHandler creates an interval review handler function for the ToolCaller.
// This handler will be called periodically during tool execution to check if it should continue.
// Returns nil if interval review is disabled.
func (r *ReAct) CreateIntervalReviewHandler() func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error) {
	if r.config.DisableIntervalReview {
		return nil
	}

	return func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error) {
		return r._invokeToolCall_IntervalReview(ctx, tool, params, stdoutSnapshot, stderrSnapshot)
	}
}

// GetIntervalReviewDuration returns the configured interval review duration.
// Returns 0 if interval review is disabled.
func (r *ReAct) GetIntervalReviewDuration() time.Duration {
	if r.config.DisableIntervalReview {
		return 0
	}
	if r.config.IntervalReviewDuration <= 0 {
		return time.Second * 10 // default 10 seconds
	}
	return r.config.IntervalReviewDuration
}
