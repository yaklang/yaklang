package aicommon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const (
	// Test timing constants - keep tests fast (total < 10s)
	testShortInterval  = time.Millisecond * 20  // short interval for handler calls
	testMediumInterval = time.Millisecond * 50  // medium interval
	testLongInterval   = time.Millisecond * 100 // long interval for comparison tests
	testWaitTimeout    = time.Millisecond * 500 // timeout for waiting operations
	testQuickTimeout   = time.Millisecond * 200 // quick timeout for fast operations
)

// TestGetIntervalReviewDuration tests the GetIntervalReviewDuration method
func TestGetIntervalReviewDuration(t *testing.T) {
	t.Run("default duration is 10 seconds", func(t *testing.T) {
		tc := &ToolCaller{}
		duration := tc.GetIntervalReviewDuration()
		require.Equal(t, time.Second*10, duration, "default duration should be 10 seconds")
	})

	t.Run("zero duration returns default", func(t *testing.T) {
		tc := &ToolCaller{
			intervalReviewDuration: 0,
		}
		duration := tc.GetIntervalReviewDuration()
		require.Equal(t, time.Second*10, duration, "zero duration should return default 10 seconds")
	})

	t.Run("negative duration returns default", func(t *testing.T) {
		tc := &ToolCaller{
			intervalReviewDuration: -time.Second,
		}
		duration := tc.GetIntervalReviewDuration()
		require.Equal(t, time.Second*10, duration, "negative duration should return default 10 seconds")
	})

	t.Run("custom duration is preserved", func(t *testing.T) {
		tc := &ToolCaller{
			intervalReviewDuration: time.Second * 5,
		}
		duration := tc.GetIntervalReviewDuration()
		require.Equal(t, time.Second*5, duration, "custom duration should be preserved")
	})

	t.Run("very short duration is preserved", func(t *testing.T) {
		tc := &ToolCaller{
			intervalReviewDuration: time.Millisecond * 100,
		}
		duration := tc.GetIntervalReviewDuration()
		require.Equal(t, time.Millisecond*100, duration, "short duration should be preserved")
	})
}

// TestWithToolCallerIntervalReviewDuration tests the WithToolCaller_IntervalReviewDuration option
func TestWithToolCallerIntervalReviewDuration(t *testing.T) {
	t.Run("set custom duration", func(t *testing.T) {
		tc := &ToolCaller{}
		opt := WithToolCaller_IntervalReviewDuration(time.Second * 30)
		opt(tc)
		require.Equal(t, time.Second*30, tc.intervalReviewDuration)
	})

	t.Run("set millisecond duration", func(t *testing.T) {
		tc := &ToolCaller{}
		opt := WithToolCaller_IntervalReviewDuration(time.Millisecond * 500)
		opt(tc)
		require.Equal(t, time.Millisecond*500, tc.intervalReviewDuration)
	})

	t.Run("set minute duration", func(t *testing.T) {
		tc := &ToolCaller{}
		opt := WithToolCaller_IntervalReviewDuration(time.Minute)
		opt(tc)
		require.Equal(t, time.Minute, tc.intervalReviewDuration)
	})
}

// TestWithToolCallerIntervalReviewHandler tests the WithToolCaller_IntervalReviewHandler option
func TestWithToolCallerIntervalReviewHandler(t *testing.T) {
	t.Run("set handler", func(t *testing.T) {
		tc := &ToolCaller{}
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			return true, nil
		}
		opt := WithToolCaller_IntervalReviewHandler(handler)
		opt(tc)
		require.NotNil(t, tc.intervalReviewHandler)
	})

	t.Run("handler is callable", func(t *testing.T) {
		tc := &ToolCaller{}
		called := false
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			called = true
			return true, nil
		}
		opt := WithToolCaller_IntervalReviewHandler(handler)
		opt(tc)

		// Call the handler
		result, err := tc.intervalReviewHandler(context.Background(), nil, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, result)
		require.True(t, called)
	})
}

// TestIntervalReviewContext tests the intervalReviewContext function
func TestIntervalReviewContext(t *testing.T) {
	t.Run("exits immediately when handler is nil", func(t *testing.T) {
		tc := &ToolCaller{}
		ctx, cancel := context.WithTimeout(context.Background(), testQuickTimeout)
		defer cancel()

		// Should return immediately without blocking
		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
			close(done)
		}()

		select {
		case <-done:
			// Expected: should return immediately
		case <-ctx.Done():
			t.Fatal("intervalReviewContext should return immediately when handler is nil")
		}
	})

	t.Run("exits when context is cancelled", func(t *testing.T) {
		tc := &ToolCaller{
			intervalReviewDuration: testMediumInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				return true, nil
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
			close(done)
		}()

		// Cancel the context after a short delay
		time.Sleep(testShortInterval)
		cancel()

		select {
		case <-done:
			// Expected: should return after context cancellation
		case <-time.After(testQuickTimeout):
			t.Fatal("intervalReviewContext should exit when context is cancelled")
		}
	})

	t.Run("calls handler at configured interval", func(t *testing.T) {
		var callCount int32

		tc := &ToolCaller{
			intervalReviewDuration: testShortInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				atomic.AddInt32(&callCount, 1)
				return true, nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
			close(done)
		}()

		// Wait for at least 2 handler calls (2 * 20ms + buffer)
		time.Sleep(testShortInterval * 4)
		cancel()

		select {
		case <-done:
			count := atomic.LoadInt32(&callCount)
			require.GreaterOrEqual(t, count, int32(2), "handler should be called at least 2 times")
		case <-time.After(testQuickTimeout):
			t.Fatal("intervalReviewContext should exit after context cancellation")
		}
	})

	t.Run("cancels when handler returns false", func(t *testing.T) {
		var callCount int32
		var reviewCancelCalled bool
		var userCancelCalled bool
		var userCancelMu sync.Mutex

		tc := &ToolCaller{
			intervalReviewDuration: testShortInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				count := atomic.AddInt32(&callCount, 1)
				if count >= 2 {
					return false, nil // Stop after 2 calls
				}
				return true, nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()

		reviewCancel := func() {
			reviewCancelCalled = true
		}

		userCancel := func(reason any) {
			userCancelMu.Lock()
			userCancelCalled = true
			userCancelMu.Unlock()
		}

		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, reviewCancel, nil, nil, nil, nil, userCancel)
			close(done)
		}()

		select {
		case <-done:
			require.True(t, reviewCancelCalled, "reviewCancel should be called when handler returns false")
			userCancelMu.Lock()
			require.True(t, userCancelCalled, "userCancel should be called when handler returns false")
			userCancelMu.Unlock()
		case <-time.After(testQuickTimeout):
			t.Fatal("intervalReviewContext should exit after handler returns false")
		}
	})

	t.Run("continues when handler returns error", func(t *testing.T) {
		var callCount int32

		tc := &ToolCaller{
			intervalReviewDuration: testShortInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				count := atomic.AddInt32(&callCount, 1)
				if count == 1 {
					return false, context.DeadlineExceeded // Return error on first call
				}
				return true, nil
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, cancel, nil, nil, nil, nil, nil)
			close(done)
		}()

		// Wait for a few handler calls (3 * 20ms + buffer)
		time.Sleep(testShortInterval * 5)
		cancel()

		select {
		case <-done:
			count := atomic.LoadInt32(&callCount)
			require.GreaterOrEqual(t, count, int32(2), "handler should continue after error")
		case <-time.After(testQuickTimeout):
			t.Fatal("intervalReviewContext should exit after context cancellation")
		}
	})

	t.Run("passes correct parameters to handler", func(t *testing.T) {
		var receivedTool *aitool.Tool
		var receivedParams aitool.InvokeParams
		var receivedStdout, receivedStderr []byte
		var mu sync.Mutex

		// Create a simple tool for testing - use a pointer that will be compared by reference
		expectedTool := &aitool.Tool{}
		expectedParams := aitool.InvokeParams{"key": "value"}
		expectedStdout := []byte("stdout content")
		expectedStderr := []byte("stderr content")

		tc := &ToolCaller{
			intervalReviewDuration: testShortInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				mu.Lock()
				receivedTool = tool
				receivedParams = params
				receivedStdout = stdout
				receivedStderr = stderr
				mu.Unlock()
				return false, nil // Stop after first call
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			tc.intervalReviewContext(ctx, cancel, expectedTool, expectedParams, expectedStdout, expectedStderr, nil)
			close(done)
		}()

		select {
		case <-done:
			mu.Lock()
			require.Same(t, expectedTool, receivedTool, "tool pointer should be the same")
			require.Equal(t, expectedParams, receivedParams)
			require.Equal(t, expectedStdout, receivedStdout)
			require.Equal(t, expectedStderr, receivedStderr)
			mu.Unlock()
		case <-time.After(testQuickTimeout):
			t.Fatal("intervalReviewContext should complete")
		}
	})
}

// TestIntervalReviewDurationIntegration tests the integration of duration configuration
func TestIntervalReviewDurationIntegration(t *testing.T) {
	t.Run("shorter duration results in more calls", func(t *testing.T) {
		var shortDurationCalls, longDurationCalls int32

		// Test with short duration (20ms)
		tcShort := &ToolCaller{
			intervalReviewDuration: testShortInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				atomic.AddInt32(&shortDurationCalls, 1)
				return true, nil
			},
		}

		// Test with long duration (100ms)
		tcLong := &ToolCaller{
			intervalReviewDuration: testLongInterval,
			intervalReviewHandler: func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
				atomic.AddInt32(&longDurationCalls, 1)
				return true, nil
			},
		}

		ctx1, cancel1 := context.WithTimeout(context.Background(), testWaitTimeout)
		ctx2, cancel2 := context.WithTimeout(context.Background(), testWaitTimeout)
		defer cancel1()
		defer cancel2()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			tcShort.intervalReviewContext(ctx1, cancel1, nil, nil, nil, nil, nil)
		}()

		go func() {
			defer wg.Done()
			tcLong.intervalReviewContext(ctx2, cancel2, nil, nil, nil, nil, nil)
		}()

		// Run for 150ms (enough for short to call ~7 times, long to call ~1 time)
		time.Sleep(time.Millisecond * 150)
		cancel1()
		cancel2()
		wg.Wait()

		short := atomic.LoadInt32(&shortDurationCalls)
		long := atomic.LoadInt32(&longDurationCalls)

		require.Greater(t, short, long, "shorter duration should result in more handler calls")
	})
}

// TestToolCallerOptionsChaining tests chaining multiple options
func TestToolCallerOptionsChaining(t *testing.T) {
	t.Run("chain handler and duration options", func(t *testing.T) {
		handlerCalled := false
		handler := func(ctx context.Context, tool *aitool.Tool, params aitool.InvokeParams, stdout, stderr []byte) (bool, error) {
			handlerCalled = true
			return true, nil
		}

		tc := &ToolCaller{}
		WithToolCaller_IntervalReviewHandler(handler)(tc)
		WithToolCaller_IntervalReviewDuration(time.Second * 5)(tc)

		require.NotNil(t, tc.intervalReviewHandler)
		require.Equal(t, time.Second*5, tc.intervalReviewDuration)

		// Call the handler to verify it works
		_, err := tc.intervalReviewHandler(context.Background(), nil, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, handlerCalled)
	})
}
