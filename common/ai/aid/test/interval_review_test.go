package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestIntervalReviewConfig tests that interval review configuration options work correctly
func TestIntervalReviewConfig(t *testing.T) {
	t.Run("interval_review_enabled_by_default", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background())
		require.False(t, config.DisableIntervalReview, "DisableIntervalReview should be false by default (enabled)")
	})

	t.Run("disable_interval_review_option", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background(),
			aicommon.WithDisableToolCallerIntervalReview(true),
		)
		require.True(t, config.DisableIntervalReview, "DisableIntervalReview should be true when disabled")
	})

	t.Run("interval_review_duration_option", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background(),
			aicommon.WithToolCallerIntervalReviewDuration(time.Second*5),
		)
		require.False(t, config.DisableIntervalReview, "DisableIntervalReview should still be false")
		require.Equal(t, time.Second*5, config.IntervalReviewDuration, "IntervalReviewDuration should be 5 seconds")
	})

	t.Run("default_interval_review_duration", func(t *testing.T) {
		config := aicommon.NewConfig(context.Background())
		// Default should be 0 (will be interpreted as 10 seconds in GetIntervalReviewDuration)
		require.Equal(t, time.Duration(0), config.IntervalReviewDuration, "default IntervalReviewDuration should be 0")
	})
}

// TestToolCallerIntervalReviewHandler tests the ToolCaller interval review handler directly
func TestToolCallerIntervalReviewHandler(t *testing.T) {
	t.Run("handler is called with correct parameters", func(t *testing.T) {
		var handlerCalled bool
		var receivedTool *aitool.Tool
		var receivedParams aitool.InvokeParams
		var receivedStdout, receivedStderr []byte

		expectedTool := &aitool.Tool{}
		expectedParams := aitool.InvokeParams{"key": "value"}
		expectedStdout := []byte("test stdout")
		expectedStderr := []byte("test stderr")

		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			handlerCalled = true
			receivedTool = tool
			receivedParams = params
			receivedStdout = stdout
			receivedStderr = stderr
			return false, nil // Cancel to exit immediately
		}

		tc := &aicommon.ToolCaller{}
		aicommon.WithToolCaller_IntervalReviewHandler(handler)(tc)
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 20)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.IntervalReviewContext(ctx, cancel, expectedTool, expectedParams, expectedStdout, expectedStderr, nil)
			close(done)
		}()

		<-done

		require.True(t, handlerCalled, "handler should be called")
		require.Same(t, expectedTool, receivedTool, "tool should match")
		require.Equal(t, expectedParams, receivedParams, "params should match")
		require.Equal(t, expectedStdout, receivedStdout, "stdout should match")
		require.Equal(t, expectedStderr, receivedStderr, "stderr should match")
	})

	t.Run("handler respects configured duration", func(t *testing.T) {
		var callCount int32

		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			atomic.AddInt32(&callCount, 1)
			return true, nil
		}

		tc := &aicommon.ToolCaller{}
		aicommon.WithToolCaller_IntervalReviewHandler(handler)(tc)
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 30)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*150)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.IntervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
			close(done)
		}()

		<-done

		count := atomic.LoadInt32(&callCount)
		// With 30ms interval and 150ms total, we expect 3-4 calls
		require.GreaterOrEqual(t, count, int32(3), "handler should be called at least 3 times")
		require.LessOrEqual(t, count, int32(6), "handler should not be called more than 6 times")
	})
}

// createLongRunningToolForTest creates a tool that runs for a specified duration with periodic output
func createLongRunningToolForTest(duration time.Duration, outputToken string) *aitool.Tool {
	tool, _ := aitool.New("long_running_task",
		aitool.WithDescription("A tool that runs for a long time and periodically outputs progress"),
		aitool.WithStringParam("task_name", aitool.WithParam_Description("Name of the task")),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			taskName := params.GetString("task_name")

			// Simulate long-running task with periodic output
			iterations := int(duration / (time.Millisecond * 50))
			if iterations < 1 {
				iterations = 1
			}
			for i := 0; i < iterations; i++ {
				fmt.Fprintf(stdout, "Progress: %d/%d - %s\n", i+1, iterations, taskName)
				time.Sleep(time.Millisecond * 50)
			}

			fmt.Fprintf(stdout, "Task completed: %s - %s\n", taskName, outputToken)
			return map[string]any{
				"status":  "completed",
				"message": outputToken,
			}, nil
		}),
	)
	return tool
}

// TestIntervalReviewIntegration_MockedAI tests interval review with mocked AI callback (no external calls)
func TestIntervalReviewIntegration_MockedAI(t *testing.T) {
	t.Run("interval_review_continue_with_mocked_ai", func(t *testing.T) {
		outputToken := uuid.New().String()
		var intervalReviewCount int32

		inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
		outputChan := make(chan *schema.AiOutputEvent, 100)

		// Create a tool that runs for 300ms
		longRunningTool := createLongRunningToolForTest(time.Millisecond*300, outputToken)

		coordinator, err := aid.NewCoordinator(
			"test interval review continue",
			aicommon.WithAgreeYOLO(),
			aicommon.WithTools(longRunningTool),
			aicommon.WithEventInputChanx(inputChan),
			// Interval review is enabled by default, just set the duration
			aicommon.WithToolCallerIntervalReviewDuration(time.Millisecond*80), // Review every 80ms
			aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
				select {
				case outputChan <- event:
				default:
				}
			}),
			// Mock AI callback - no external calls
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := r.GetPrompt()
				rsp := i.NewAIResponse()

				// Handle interval review requests - mock response
				if strings.Contains(prompt, "Interval Review") || strings.Contains(prompt, "interval-toolcall-review") {
					atomic.AddInt32(&intervalReviewCount, 1)
					// Always return continue
					rsp.EmitOutputStream(bytes.NewBufferString(`{
						"@action": "interval-toolcall-review",
						"decision": "continue",
						"reason": "Tool is making progress",
						"progress_summary": "Execution is proceeding normally"
					}`))
					return rsp, nil
				}

				// Handle regular tool calling flow - mock response
				return mockedToolCalling(i, r, "long_running_task", fmt.Sprintf(`{"@action": "call-tool", "tool": "long_running_task", "params": {"task_name": "test_task_%s"}}`, outputToken))
			}),
		)
		require.NoError(t, err, "NewCoordinator should not fail")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		go coordinator.Run()

		// Wait for tool call completion or timeout
		var toolCallCompleted bool
	LOOP:
		for {
			select {
			case <-ctx.Done():
				break LOOP
			case result := <-outputChan:
				if result.Type == schema.EVENT_TOOL_CALL_DONE {
					toolCallCompleted = true
					break LOOP
				}
			}
		}

		// Verify results
		count := atomic.LoadInt32(&intervalReviewCount)
		t.Logf("Interval review was called %d times", count)
		// With 300ms tool duration and 80ms interval, expect at least 2 reviews
		require.GreaterOrEqual(t, count, int32(1), "interval review should be called at least once")
		require.True(t, toolCallCompleted, "tool call should complete when AI returns 'continue'")
	})

	t.Run("interval_review_cancel_with_mocked_ai", func(t *testing.T) {
		outputToken := uuid.New().String()
		var intervalReviewCount int32

		inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
		outputChan := make(chan *schema.AiOutputEvent, 100)

		// Create a tool that runs for 1 second (should be cancelled before completion)
		longRunningTool := createLongRunningToolForTest(time.Second*1, outputToken)

		coordinator, err := aid.NewCoordinator(
			"test interval review cancel",
			aicommon.WithAgreeYOLO(),
			aicommon.WithTools(longRunningTool),
			aicommon.WithEventInputChanx(inputChan),
			// Interval review is enabled by default, just set the duration
			aicommon.WithToolCallerIntervalReviewDuration(time.Millisecond*80), // Review every 80ms
			aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
				select {
				case outputChan <- event:
				default:
				}
			}),
			// Mock AI callback - no external calls
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := r.GetPrompt()
				rsp := i.NewAIResponse()

				// Handle interval review requests - mock response
				if strings.Contains(prompt, "Interval Review") || strings.Contains(prompt, "interval-toolcall-review") {
					count := atomic.AddInt32(&intervalReviewCount, 1)
					// Cancel after 2 reviews
					if count >= 2 {
						rsp.EmitOutputStream(bytes.NewBufferString(`{
							"@action": "interval-toolcall-review",
							"decision": "cancel",
							"reason": "Tool is taking too long, cancelling execution",
							"concerns": ["Execution time exceeded expectations"]
						}`))
					} else {
						rsp.EmitOutputStream(bytes.NewBufferString(`{
							"@action": "interval-toolcall-review",
							"decision": "continue",
							"reason": "Tool is making progress"
						}`))
					}
					return rsp, nil
				}

				// Handle regular tool calling flow - mock response
				return mockedToolCalling(i, r, "long_running_task", fmt.Sprintf(`{"@action": "call-tool", "tool": "long_running_task", "params": {"task_name": "test_task_%s"}}`, outputToken))
			}),
		)
		require.NoError(t, err, "NewCoordinator should not fail")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		go coordinator.Run()

		// Wait for tool call cancel or completion
		var toolCallCancelled bool
	LOOP:
		for {
			select {
			case <-ctx.Done():
				break LOOP
			case result := <-outputChan:
				if result.Type == schema.EVENT_TOOL_CALL_USER_CANCEL {
					toolCallCancelled = true
					break LOOP
				}
				if result.Type == schema.EVENT_TOOL_CALL_DONE {
					// Tool completed normally (not expected in this test)
					break LOOP
				}
			}
		}

		// Verify results
		count := atomic.LoadInt32(&intervalReviewCount)
		t.Logf("Interval review was called %d times", count)
		require.GreaterOrEqual(t, count, int32(2), "interval review should be called at least twice")
		require.True(t, toolCallCancelled, "tool call should be cancelled when AI returns 'cancel'")
	})

	t.Run("interval_review_disabled_with_mocked_ai", func(t *testing.T) {
		outputToken := uuid.New().String()
		var intervalReviewCount int32

		inputChan := chanx.NewUnlimitedChan[*ypb.AIInputEvent](context.Background(), 10)
		outputChan := make(chan *schema.AiOutputEvent, 100)

		// Create a quick tool (runs for 150ms)
		quickTool := createLongRunningToolForTest(time.Millisecond*150, outputToken)

		coordinator, err := aid.NewCoordinator(
			"test interval review disabled",
			aicommon.WithAgreeYOLO(),
			aicommon.WithTools(quickTool),
			aicommon.WithEventInputChanx(inputChan),
			aicommon.WithDisableToolCallerIntervalReview(true), // Explicitly disabled
			aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
				select {
				case outputChan <- event:
				default:
				}
			}),
			// Mock AI callback - no external calls
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				prompt := r.GetPrompt()
				rsp := i.NewAIResponse()

				// Handle interval review requests (should not happen when disabled)
				if strings.Contains(prompt, "Interval Review") || strings.Contains(prompt, "interval-toolcall-review") {
					atomic.AddInt32(&intervalReviewCount, 1)
					rsp.EmitOutputStream(bytes.NewBufferString(`{
						"@action": "interval-toolcall-review",
						"decision": "continue",
						"reason": "This should not be called"
					}`))
					return rsp, nil
				}

				// Handle regular tool calling flow - mock response
				return mockedToolCalling(i, r, "long_running_task", fmt.Sprintf(`{"@action": "call-tool", "tool": "long_running_task", "params": {"task_name": "test_task_%s"}}`, outputToken))
			}),
		)
		require.NoError(t, err, "NewCoordinator should not fail")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		go coordinator.Run()

		// Wait for tool call completion
		var toolCallCompleted bool
	LOOP:
		for {
			select {
			case <-ctx.Done():
				break LOOP
			case result := <-outputChan:
				if result.Type == schema.EVENT_TOOL_CALL_DONE {
					toolCallCompleted = true
					break LOOP
				}
			}
		}

		// Verify results
		count := atomic.LoadInt32(&intervalReviewCount)
		t.Logf("Interval review was called %d times (expected 0)", count)
		require.Equal(t, int32(0), count, "interval review should not be called when disabled")
		require.True(t, toolCallCompleted, "tool call should complete normally")
	})
}

