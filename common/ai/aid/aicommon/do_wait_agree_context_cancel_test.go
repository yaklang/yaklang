package aicommon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	testAsyncTimeout = 2 * time.Second
	testNoSignalWait = 200 * time.Millisecond
)

func waitForSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal(message)
	}
}

func waitForSignals(t *testing.T, ch <-chan struct{}, count int, timeout time.Duration, message string) {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for i := 0; i < count; i++ {
		select {
		case <-ch:
		case <-timer.C:
			t.Fatal(message)
		}
	}
}

func assertNoSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(timeout):
	}
}

func startEventLoopWithSignals(c *Config, ctx context.Context) (<-chan struct{}, <-chan struct{}) {
	started := make(chan struct{}, 1)
	done := make(chan struct{}, 1)

	c.StartEventLoopEx(ctx, func() {
		started <- struct{}{}
	}, func() {
		done <- struct{}{}
	})

	return started, done
}

func newTestEventInputChan() (*chanx.UnlimitedChan[*ypb.AIInputEvent], chan *ypb.AIInputEvent) {
	in := make(chan *ypb.AIInputEvent, 8)
	out := make(chan *ypb.AIInputEvent, 8)
	return chanx.NewUnlimitedChanEx(context.Background(), in, out, 8), out
}

func TestDoWaitAgreeWithPolicy_AI_ContextCancel(t *testing.T) {
	t.Run("AI policy unblocks when RiskControl fails", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyAI),
			WithAiAgreeRiskControl(func(_ context.Context, _ *Config, _ *Endpoint) (*Action, error) {
				return nil, errors.New("simulated review failure")
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyAI, ep)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("DoWaitAgreeWithPolicy blocked forever after RiskControl error")
		}

		params := ep.GetParams()
		assert.Equal(t, "continue", params["suggestion"],
			"endpoint should be auto-released with continue on RiskControl failure")
	})

	t.Run("AI policy unblocks when parent ctx cancelled externally", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		reviewStarted := make(chan struct{}, 1)
		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyAI),
			WithAiAgreeRiskControl(func(ctx context.Context, _ *Config, _ *Endpoint) (*Action, error) {
				reviewStarted <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyAI, ep)
		}()

		waitForSignal(t, reviewStarted, testAsyncTimeout, "AI review callback did not start")
		cancel()

		select {
		case <-done:
		case <-time.After(testAsyncTimeout):
			t.Fatal("DoWaitAgreeWithPolicy should unblock when context is cancelled")
		}
	})

	t.Run("Manual policy respects context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := NewTestConfig(ctx, WithAgreePolicy(AgreePolicyManual))
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyManual, ep)
		}()

		cancel()

		select {
		case <-done:
		case <-time.After(testAsyncTimeout):
			t.Fatal("DoWaitAgreeWithPolicy with Manual should unblock when context cancelled")
		}
	})

	t.Run("AI policy low-score auto-continue works correctly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyAI),
			WithAiAgreeRiskControl(func(_ context.Context, _ *Config, _ *Endpoint) (*Action, error) {
				return NewSimpleAction("risk-check", aitool.InvokeParams{"risk_score": 0.1}), nil
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyAI, ep)
		}()

		select {
		case <-done:
			params := ep.GetParams()
			require.Equal(t, "continue", params["suggestion"])
		case <-time.After(5 * time.Second):
			t.Fatal("low score should auto-continue without blocking")
		}
	})

	t.Run("AI policy high-score does not auto-continue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyAI),
			WithAiAgreeRiskControl(func(_ context.Context, _ *Config, _ *Endpoint) (*Action, error) {
				return NewSimpleAction("risk-check", aitool.InvokeParams{
					"risk_score": 0.9,
					"reason":     "dangerous operation detected",
				}), nil
			}),
		)
		c.StartEventLoop(ctx)

		ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.DoWaitAgreeWithPolicy(ctx, AgreePolicyAI, ep)
		}()

		select {
		case <-done:
			// should only return because ctx was cancelled (3s timeout)
		case <-time.After(5 * time.Second):
			t.Fatal("should have returned after ctx timeout")
		}
	})
}

func TestEventLoop_DrainPendingEvents(t *testing.T) {
	t.Run("sync events are drained after context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		eventInput, outputChan := newTestEventInputChan()
		defer eventInput.CloseForce()

		c := NewTestConfig(ctx, WithEventInputChanx(eventInput))

		processed := make(chan struct{}, 2)
		c.InputEventManager.RegisterSyncCallback("test_drain_sync", func(event *ypb.AIInputEvent) error {
			processed <- struct{}{}
			return nil
		})

		outputChan <- &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_drain_sync",
		}
		outputChan <- &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_drain_sync",
		}

		loopStarted := make(chan struct{}, 1)
		loopDone := make(chan struct{}, 1)
		c.StartEventLoopEx(ctx, func() {
			loopStarted <- struct{}{}
			cancel()
		}, func() {
			loopDone <- struct{}{}
		})

		waitForSignal(t, loopStarted, testAsyncTimeout, "event loop did not start")
		waitForSignals(t, processed, 2, testAsyncTimeout,
			"all sync events fed before cancel should be processed")
		waitForSignal(t, loopDone, testAsyncTimeout, "event loop did not exit after cancel")
	})

	t.Run("events fed concurrently with cancel are handled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		eventInput, outputChan := newTestEventInputChan()
		defer eventInput.CloseForce()

		c := NewTestConfig(ctx, WithEventInputChanx(eventInput))

		processed := make(chan struct{}, 2)
		firstStarted := make(chan struct{}, 1)
		releaseFirst := make(chan struct{})
		c.InputEventManager.RegisterSyncCallback("test_concurrent_cancel_first", func(event *ypb.AIInputEvent) error {
			firstStarted <- struct{}{}
			<-releaseFirst
			processed <- struct{}{}
			return nil
		})
		c.InputEventManager.RegisterSyncCallback("test_concurrent_cancel_second", func(event *ypb.AIInputEvent) error {
			processed <- struct{}{}
			return nil
		})

		outputChan <- &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_concurrent_cancel_first",
		}

		loopStarted := make(chan struct{}, 1)
		loopDone := make(chan struct{}, 1)
		c.StartEventLoopEx(ctx, func() {
			loopStarted <- struct{}{}
			cancel()
		}, func() {
			loopDone <- struct{}{}
		})

		waitForSignal(t, loopStarted, testAsyncTimeout, "event loop did not start")
		waitForSignal(t, firstStarted, testAsyncTimeout,
			"the first sync event did not enter drainPendingEvents")

		outputChan <- &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_concurrent_cancel_second",
		}
		close(releaseFirst)

		waitForSignals(t, processed, 2, testAsyncTimeout,
			"sync events queued around cancel should be processed")
		waitForSignal(t, loopDone, testAsyncTimeout, "event loop did not exit after cancel")
	})

	t.Run("non-sync events are not drained after cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		eventInput, outputChan := newTestEventInputChan()
		defer eventInput.CloseForce()

		c := NewTestConfig(ctx, WithEventInputChanx(eventInput))

		mirrorTriggered := make(chan struct{}, 1)
		c.InputEventManager.RegisterMirrorOfAIInputEvent("test_drain_mirror",
			func(event *ypb.AIInputEvent) {
				mirrorTriggered <- struct{}{}
			},
		)

		loopStarted, loopDone := startEventLoopWithSignals(c, ctx)
		waitForSignal(t, loopStarted, testAsyncTimeout, "event loop did not start")

		cancel()
		waitForSignal(t, loopDone, testAsyncTimeout, "event loop did not exit after cancel")

		outputChan <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "should not be processed after cancel",
		}

		assertNoSignal(t, mirrorTriggered, testNoSignalWait,
			"non-sync events should not be processed after cancel")
	})
}

func TestSyncCallback_RegisterAndUnregister(t *testing.T) {
	t.Run("unregistered callback is not called", func(t *testing.T) {
		processor := NewAIInputEventProcessor()

		var called bool
		processor.RegisterSyncCallback("test_unreg", func(event *ypb.AIInputEvent) error {
			called = true
			return nil
		})
		processor.UnRegisterSyncCallback("test_unreg")

		event := &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_unreg",
		}

		var c Config
		c.InputEventManager = processor
		err := c.processInputEvent(event)
		assert.NoError(t, err)
		assert.False(t, called, "unregistered callback should not be called")
	})

	t.Run("register replaces previous callback", func(t *testing.T) {
		processor := NewAIInputEventProcessor()

		var firstCalled, secondCalled bool
		processor.RegisterSyncCallback("test_replace", func(event *ypb.AIInputEvent) error {
			firstCalled = true
			return nil
		})
		processor.RegisterSyncCallback("test_replace", func(event *ypb.AIInputEvent) error {
			secondCalled = true
			return nil
		})

		event := &ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_replace",
		}

		var c Config
		c.InputEventManager = processor
		err := c.processInputEvent(event)
		assert.NoError(t, err)
		assert.False(t, firstCalled, "first callback should be replaced")
		assert.True(t, secondCalled, "second callback should be the active one")
	})
}
