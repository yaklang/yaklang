package test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"

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
		// Default should be 0 (will be interpreted as 20 seconds in GetIntervalReviewDuration)
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

// TestIntervalReviewIntegration_MockedHandler tests interval review with mocked handler (no external AI calls)
// This test directly tests the ToolCaller interval review mechanism without depending on full Coordinator flow
func TestIntervalReviewIntegration_MockedHandler(t *testing.T) {
	t.Run("handler_continues_execution", func(t *testing.T) {
		var handlerCallCount int32

		// Create handler that always returns continue
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			atomic.AddInt32(&handlerCallCount, 1)
			return true, nil // continue
		}

		tc := &aicommon.ToolCaller{}
		aicommon.WithToolCaller_IntervalReviewHandler(handler)(tc)
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 30)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
		defer cancel()

		// Run interval review in background
		done := make(chan struct{})
		go func() {
			tc.IntervalReviewContext(ctx, cancel, &aitool.Tool{}, nil, []byte("test stdout"), []byte("test stderr"), nil)
			close(done)
		}()

		// Wait for context timeout
		<-done

		count := atomic.LoadInt32(&handlerCallCount)
		t.Logf("Handler was called %d times", count)
		// With 30ms interval and 200ms timeout, expect 5-6 calls
		require.GreaterOrEqual(t, count, int32(4), "handler should be called at least 4 times")
		require.LessOrEqual(t, count, int32(8), "handler should not be called too many times")
	})

	t.Run("handler_cancels_execution", func(t *testing.T) {
		var handlerCallCount int32
		var cancelCalled bool

		// Create handler that cancels after 2 calls
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			count := atomic.AddInt32(&handlerCallCount, 1)
			if count >= 2 {
				return false, nil // cancel
			}
			return true, nil // continue
		}

		tc := &aicommon.ToolCaller{}
		aicommon.WithToolCaller_IntervalReviewHandler(handler)(tc)
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 30)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
		defer cancel()

		// Run interval review in background
		done := make(chan struct{})
		go func() {
			tc.IntervalReviewContext(ctx, func() {
				cancelCalled = true
				cancel()
			}, &aitool.Tool{}, nil, nil, nil, nil)
			close(done)
		}()

		// Wait for completion
		<-done

		count := atomic.LoadInt32(&handlerCallCount)
		t.Logf("Handler was called %d times before cancel", count)
		require.Equal(t, int32(2), count, "handler should be called exactly 2 times before cancel")
		require.True(t, cancelCalled, "cancel should be called when handler returns false")
	})

	t.Run("handler_receives_correct_parameters", func(t *testing.T) {
		var receivedTool *aitool.Tool
		var receivedParams aitool.InvokeParams
		var receivedStdout, receivedStderr []byte

		expectedTool := &aitool.Tool{}
		expectedParams := aitool.InvokeParams{"key": "value", "number": 42}
		expectedStdout := []byte("stdout content here")
		expectedStderr := []byte("stderr content here")

		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			receivedTool = tool
			receivedParams = params
			receivedStdout = stdout
			receivedStderr = stderr
			return false, nil // cancel immediately to verify parameters
		}

		tc := &aicommon.ToolCaller{}
		aicommon.WithToolCaller_IntervalReviewHandler(handler)(tc)
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 20)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.IntervalReviewContext(ctx, cancel, expectedTool, expectedParams, expectedStdout, expectedStderr, nil)
			close(done)
		}()

		<-done

		require.Same(t, expectedTool, receivedTool, "tool should be passed correctly")
		require.Equal(t, expectedParams, receivedParams, "params should be passed correctly")
		require.Equal(t, expectedStdout, receivedStdout, "stdout should be passed correctly")
		require.Equal(t, expectedStderr, receivedStderr, "stderr should be passed correctly")
	})

	t.Run("no_handler_means_no_review", func(t *testing.T) {
		tc := &aicommon.ToolCaller{}
		// Intentionally NOT setting a handler
		aicommon.WithToolCaller_IntervalReviewDuration(time.Millisecond * 20)(tc)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		start := time.Now()
		tc.IntervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
		elapsed := time.Since(start)

		// Should return immediately when handler is nil
		require.Less(t, elapsed, time.Millisecond*50, "should return immediately when handler is nil")
	})

	t.Run("handler_error_continues_execution", func(t *testing.T) {
		var handlerCallCount int32

		// Create handler that returns error
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			atomic.AddInt32(&handlerCallCount, 1)
			return true, context.DeadlineExceeded // return error but continue
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

		count := atomic.LoadInt32(&handlerCallCount)
		t.Logf("Handler was called %d times despite errors", count)
		// Should continue calling despite errors
		require.GreaterOrEqual(t, count, int32(3), "handler should continue being called despite errors")
	})
}
