package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func normalizeIntervalReviewFieldContent(reader io.Reader) (string, bool) {
	raw, err := io.ReadAll(utils.UTF8Reader(reader))
	if err != nil {
		log.Warnf("interval review field read failed: %v", err)
		return "", false
	}

	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", false
	}

	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		value, ok := decoded.(string)
		if !ok {
			log.Warnf("interval review field expected string, got %T: %s", decoded, utils.ShrinkString(text, 160))
			return "", false
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return "", false
		}
		return value, true
	}

	return text, true
}

// _invokeToolCall_IntervalReviewWithContext is called periodically during tool execution to review progress.
// It returns true if the tool should continue, false if it should be cancelled.
func (r *ReAct) _invokeToolCall_IntervalReviewWithContext(
	ctx context.Context,
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (bool, error) {
	return r._invokeToolCall_IntervalReviewWithContextForTaskAndEmitter(
		ctx,
		r.GetCurrentTask(),
		r.Emitter,
		tool,
		params,
		stdoutSnapshot,
		stderrSnapshot,
		startTime,
		reviewCount,
		callExpectations,
	)
}

func (r *ReAct) _invokeToolCall_IntervalReviewWithContextForTask(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (bool, error) {
	return r._invokeToolCall_IntervalReviewWithContextForTaskAndEmitter(
		ctx,
		task,
		r.Emitter,
		tool,
		params,
		stdoutSnapshot,
		stderrSnapshot,
		startTime,
		reviewCount,
		callExpectations,
	)
}

// _invokeToolCall_IntervalReviewWithContextForTaskAndEmitter binds both the
// prompt context and every review stream event to the immutable owner of one
// tool call. Scalar callers keep using r.Emitter through the wrappers above;
// batch callers pass their associative child emitter so concurrent reviews do
// not lose or borrow a sibling's CallToolID/ProcessesId metadata.
func (r *ReAct) _invokeToolCall_IntervalReviewWithContextForTaskAndEmitter(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	emitter *aicommon.Emitter,
	tool *aitool.Tool,
	params aitool.InvokeParams,
	stdoutSnapshot, stderrSnapshot []byte,
	startTime time.Time,
	reviewCount int,
	callExpectations string,
) (bool, error) {
	if emitter == nil {
		// Preserve the historical scalar fallback for callers that do not own a
		// more specific emitter.
		emitter = r.Emitter
	}

	// Check context at the beginning
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	elapsed := time.Since(startTime)
	log.Infof("toolcall interval review #%d triggered for tool [%s], elapsed: %v", reviewCount, tool.Name, elapsed)

	// Generate a bounded speed-priority prompt from the newest Timeline facts.
	prompt, err := r.promptManager.GenerateIntervalReviewPromptWithContextForTask(
		task, tool, params, stdoutSnapshot, stderrSnapshot, startTime, reviewCount, callExpectations,
	)
	if err != nil {
		log.Errorf("failed to generate interval review prompt: %v", err)
		// The scheduler reports the failure and continues by default.
		return true, fmt.Errorf("generate interval review prompt: %w", err)
	}

	var shouldContinue = true
	var reviewReason string

	transErr := aicommon.CallAITransaction(r.config, prompt, r.config.CallSpeedPriorityAI,
		func(rsp *aicommon.AIResponse) error {
			boundEmitter := rsp.BindEmitter(emitter)
			action, err := aicommon.ExtractActionFromStream(
				ctx,
				rsp.GetOutputStreamReader("interval-review", true, emitter),
				"interval-toolcall-review",
				aicommon.WithActionFieldStreamHandler([]string{
					"reason", "progress_summary", "estimated_remaining_time",
				}, func(key string, reader io.Reader) {
					content, ok := normalizeIntervalReviewFieldContent(reader)
					if !ok {
						return
					}
					switch key {
					case "estimated_remaining_time":
						boundEmitter.EmitDefaultStreamEvent(
							"interval-review",
							strings.NewReader("预估时间："+content),
							rsp.GetTaskIndex(),
						)
					default:
						boundEmitter.EmitDefaultStreamEvent(
							"interval-review",
							strings.NewReader(content),
							rsp.GetTaskIndex(),
						)
					}
				}),
			)
			if err != nil {
				log.Errorf("failed to extract interval review action: %v", err)
				return fmt.Errorf("extract interval review action: %w", err)
			}

			decision := action.GetString("decision")
			reviewReason = action.GetString("reason")
			progressSummary := action.GetString("progress_summary")

			switch decision {
			case "continue":
				shouldContinue = true
				// Interval reviews are timing-dependent auxiliary observations. Do
				// not write them to the coordinator Timeline: AddToTimeline allocates
				// a replay ID, so merely changing review frequency would shift every
				// later checkpoint. The streamed review event and this runtime log
				// retain visibility without mutating replay state.
				log.Infof(
					"interval review: tool [%s] should continue. Progress: %s. Reason: %s",
					tool.Name, progressSummary, reviewReason,
				)
			case "cancel":
				shouldContinue = false
				log.Warnf(
					"interval review: tool [%s] should be cancelled. Progress: %s. Reason: %s",
					tool.Name, progressSummary, reviewReason,
				)
			default:
				// Unknown decision, continue by default
				shouldContinue = true
				log.Warnf("interval review: unknown decision '%s', continuing by default", decision)
			}
			return nil
		},
		aicommon.WithAIRequest_CallerLabel("toolcall-interval-review"),
		aicommon.WithAIRequest_Context(ctx),
		aicommon.WithAIRequest_DetachCheckpoint(),
	)

	if transErr != nil {
		log.Errorf("interval review transaction failed: %v", transErr)
		// The scheduler reports the failure and continues by default.
		return true, fmt.Errorf("interval review transaction: %w", transErr)
	}

	return shouldContinue, nil
}

// CreateIntervalReviewHandler creates an interval review handler function for the ToolCaller.
// This handler will be called periodically during tool execution to check if it should continue.
// Returns nil if interval review is disabled.
// The handler maintains its own state (start time and review count) in a closure.
func (r *ReAct) CreateIntervalReviewHandler() func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error) {
	return r.CreateIntervalReviewHandlerForTask(r.GetCurrentTask())
}

// CreateIntervalReviewHandlerForTask binds progress reviews to the task that
// owns the ToolCaller. Batch workers use this form instead of consulting the
// mutable ReAct.currentTask pointer when each interval fires.
func (r *ReAct) CreateIntervalReviewHandlerForTask(task aicommon.AIStatefulTask) func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error) {
	return r.CreateIntervalReviewHandlerForTaskAndEmitter(task, r.Emitter)
}

// CreateIntervalReviewHandlerForTaskAndEmitter creates a handler whose prompt
// and emitted streams belong to the same immutable tool-call child. Batch
// workers use it with their associative emitter; scalar callers continue to use
// CreateIntervalReviewHandlerForTask and retain the previous emitter behavior.
func (r *ReAct) CreateIntervalReviewHandlerForTaskAndEmitter(task aicommon.AIStatefulTask, emitter *aicommon.Emitter) func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error) {
	if r.config.DisableIntervalReview {
		return nil
	}

	// The scheduler injects the actual tool execution start and review count via
	// context. Keep local fallbacks for direct handler callers and old tests.
	fallbackStartTime := time.Now()
	var fallbackReviewCount int

	return func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdoutSnapshot, stderrSnapshot []byte, callExpectations string) (bool, error) {
		startTime := fallbackStartTime
		reviewCount := 0
		if metadata, ok := aicommon.ToolCallIntervalReviewMetadataFromContext(ctx); ok {
			startTime = metadata.ToolExecutionStartedAt
			reviewCount = metadata.ReviewCount
		} else {
			fallbackReviewCount++
			reviewCount = fallbackReviewCount
		}

		return r._invokeToolCall_IntervalReviewWithContextForTaskAndEmitter(ctx, task, emitter, tool, params, stdoutSnapshot, stderrSnapshot, startTime, reviewCount, callExpectations)
	}
}

// GetIntervalReviewDuration returns the configured interval review duration.
// Returns 0 if interval review is disabled.
func (r *ReAct) GetIntervalReviewDuration() time.Duration {
	if r.config.DisableIntervalReview {
		return 0
	}
	if r.config.IntervalReviewDuration <= 0 {
		return aicommon.DefaultToolCallIntervalReviewDuration
	}
	return r.config.IntervalReviewDuration
}
