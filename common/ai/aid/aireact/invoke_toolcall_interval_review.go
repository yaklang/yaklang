package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// _invokeToolCall_IntervalReviewWithContext is called periodically during tool execution to review progress.
// It returns true if the tool should continue, false if it should be cancelled.
func (r *ReAct) _invokeToolCall_IntervalReviewWithContext(
	ctx context.Context,
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
) (bool, error) {
	// Check context at the beginning
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	elapsed := time.Since(startTime)
	log.Infof("toolcall interval review #%d triggered for tool [%s], elapsed: %v", reviewCount, tool.Name, elapsed)

	// Generate the interval review prompt with full context
	prompt, err := r.promptManager.GenerateIntervalReviewPromptWithContext(
		tool, params, stdoutSnapshot, stderrSnapshot, startTime, reviewCount,
	)
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
				aicommon.WithActionFieldStreamHandler([]string{
					"reason", "progress_summary", "estimated_remaining_time",
				}, func(key string, reader io.Reader) {
					reader = utils.JSONStringReader(utils.UTF8Reader(reader))
					switch key {
					case "estimated_remaining_time":
						r.Emitter.EmitDefaultStreamEvent(
							"interval-review",
							io.MultiReader(bytes.NewBufferString("预估时间："), reader),
							rsp.GetTaskIndex(),
						)
					default:
						r.Emitter.EmitDefaultStreamEvent(
							"interval-review",
							reader,
							rsp.GetTaskIndex(),
						)
					}
				}),
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
// The handler maintains its own state (start time and review count) in a closure.
func (r *ReAct) CreateIntervalReviewHandler() func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error) {
	if r.config.DisableIntervalReview {
		return nil
	}

	// State maintained in closure
	var startTime time.Time
	var reviewCount int32

	return func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte) (bool, error) {
		// Initialize start time on first call
		if startTime.IsZero() {
			startTime = time.Now()
		}

		// Increment review count
		count := int(atomic.AddInt32(&reviewCount, 1))

		return r._invokeToolCall_IntervalReviewWithContext(ctx, tool, params, stdoutSnapshot, stderrSnapshot, startTime, count)
	}
}

// GetIntervalReviewDuration returns the configured interval review duration.
// Returns 0 if interval review is disabled.
func (r *ReAct) GetIntervalReviewDuration() time.Duration {
	if r.config.DisableIntervalReview {
		return 0
	}
	if r.config.IntervalReviewDuration <= 0 {
		return time.Second * 20 // default 20 seconds
	}
	return r.config.IntervalReviewDuration
}
