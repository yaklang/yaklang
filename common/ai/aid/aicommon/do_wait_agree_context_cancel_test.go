package aicommon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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
		c := NewTestConfig(ctx,
			WithAgreePolicy(AgreePolicyAI),
			WithAiAgreeRiskControl(func(ctx context.Context, _ *Config, _ *Endpoint) (*Action, error) {
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

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
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

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
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
		c := NewTestConfig(ctx)

		var processed int32
		c.InputEventManager.RegisterSyncCallback("test_drain_sync", func(event *ypb.AIInputEvent) error {
			atomic.AddInt32(&processed, 1)
			return nil
		})

		c.StartEventLoop(ctx)
		time.Sleep(50 * time.Millisecond)

		c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_drain_sync",
		})
		c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_drain_sync",
		})

		time.Sleep(50 * time.Millisecond)
		cancel()
		time.Sleep(200 * time.Millisecond)

		count := atomic.LoadInt32(&processed)
		assert.GreaterOrEqual(t, count, int32(2),
			"all sync events fed before cancel should be processed (got %d)", count)
	})

	t.Run("events fed concurrently with cancel are handled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := NewTestConfig(ctx)

		var processed int32
		c.InputEventManager.RegisterSyncCallback("test_concurrent_cancel", func(event *ypb.AIInputEvent) error {
			atomic.AddInt32(&processed, 1)
			return nil
		})

		c.StartEventLoop(ctx)
		time.Sleep(50 * time.Millisecond)

		c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_concurrent_cancel",
		})
		cancel()
		c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      "test_concurrent_cancel",
		})

		time.Sleep(300 * time.Millisecond)
		count := atomic.LoadInt32(&processed)
		assert.GreaterOrEqual(t, count, int32(1),
			"at least the event fed before cancel should be processed")
	})

	t.Run("non-sync events are not drained after cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := NewTestConfig(ctx)

		var mirrorCount int32
		c.InputEventManager.RegisterMirrorOfAIInputEvent("test_drain_mirror",
			func(event *ypb.AIInputEvent) {
				atomic.AddInt32(&mirrorCount, 1)
			},
		)

		c.StartEventLoop(ctx)
		time.Sleep(50 * time.Millisecond)

		cancel()
		time.Sleep(50 * time.Millisecond)

		c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "should not be processed after cancel",
		})
		time.Sleep(200 * time.Millisecond)

		count := atomic.LoadInt32(&mirrorCount)
		assert.Equal(t, int32(0), count,
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
