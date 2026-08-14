package aicommon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEndpointSignal_ReleaseBeforeWaitIsLatched(t *testing.T) {
	signal := NewEndpointSignal()
	released := make(chan struct{})
	go func() {
		signal.ActiveAsyncContext(context.Background())
		close(released)
	}()

	select {
	case <-released:
	case <-time.After(testAsyncTimeout):
		t.Fatal("release without an existing waiter must not block or leak its goroutine")
	}
	require.NoError(t, signal.WaitTimeout(testNoSignalWait), "a future waiter must observe the latched release")
}

func TestEndpointSignal_OneReleaseWakesEveryWaiter(t *testing.T) {
	signal := NewEndpointSignal()
	const waiterCount = 8
	ready := make(chan struct{}, waiterCount)
	finished := make(chan struct{}, waiterCount)
	for i := 0; i < waiterCount; i++ {
		go func() {
			ready <- struct{}{}
			signal.Wait()
			finished <- struct{}{}
		}()
	}
	waitForSignals(t, ready, waiterCount, testAsyncTimeout, "waiters did not start")
	signal.ActiveContext(context.Background())
	waitForSignals(t, finished, waiterCount, testAsyncTimeout, "one release must wake every waiter")
}

func TestEndpointSignal_RepeatedConcurrentReleaseIsIdempotent(t *testing.T) {
	signal := NewEndpointSignal()
	const releaserCount = 64
	start := make(chan struct{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(releaserCount)
	for i := 0; i < releaserCount; i++ {
		go func() {
			defer wg.Done()
			<-start
			signal.ActiveAsyncContext(context.Background())
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	close(start)
	select {
	case <-done:
	case <-time.After(testAsyncTimeout):
		t.Fatal("repeated releases must return instead of stranding senders")
	}
	require.NoError(t, signal.WaitTimeout(testNoSignalWait))
}

func TestEndpointSignal_CancelledActivationDoesNotRelease(t *testing.T) {
	signal := NewEndpointSignal()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	signal.ActiveContext(ctx)
	require.ErrorIs(t, signal.WaitTimeout(20*time.Millisecond), context.DeadlineExceeded)
	signal.ActiveContext(context.Background())
	require.NoError(t, signal.WaitTimeout(testNoSignalWait))
}

func TestDoWaitAgreeWithPolicy_ManualCallbackPublishesOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testAsyncTimeout)
	defer cancel()
	var callbackCalls int
	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyManual),
		WithAgreeManualCallback(func(callbackCtx context.Context, _ *Config) (aitool.InvokeParams, error) {
			require.NoError(t, callbackCtx.Err())
			callbackCalls++
			return aitool.InvokeParams{"suggestion": "continue", "source": "assistant"}, nil
		}),
	)
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)

	c.DoWaitAgreeWithPolicy(ctx, AgreePolicyManual, ep)
	require.Equal(t, 1, callbackCalls)
	require.Equal(t, "continue", ep.GetParams().GetString("suggestion"))
	require.Equal(t, "assistant", ep.GetParams().GetString("source"))
	require.Equal(t, ApprovalSourceModelJudge, ep.GetApprovalMeta().Source)
	// The callback released before this second wait began. The result must stay
	// latched, proving the positive path did not depend on repeated retries.
	require.True(t, ep.WaitTimeout(testNoSignalWait))
}

func TestDoWaitAgreeWithPolicy_ManualCallbackCancelledResponseIsIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callbackStarted := make(chan struct{})
	allowLateReturn := make(chan struct{})
	callbackReturned := make(chan struct{})
	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyManual),
		WithAgreeManualCallback(func(_ context.Context, _ *Config) (aitool.InvokeParams, error) {
			close(callbackStarted)
			<-allowLateReturn // deliberately ignore the callback context
			close(callbackReturned)
			return aitool.InvokeParams{"suggestion": "late-continue"}, nil
		}),
	)
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	done := make(chan struct{})
	go func() {
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyManual, ep)
		close(done)
	}()
	waitForSignal(t, callbackStarted, testAsyncTimeout, "manual callback did not start")
	cancel()
	waitForSignal(t, done, testAsyncTimeout, "manual wait did not observe cancellation")
	close(allowLateReturn)
	waitForSignal(t, callbackReturned, testAsyncTimeout, "manual callback did not return")
	time.Sleep(20 * time.Millisecond)
	require.Empty(t, ep.GetParams(), "a late callback must not populate the cancelled endpoint")
	require.Equal(t, ApprovalSourceHuman, ep.GetApprovalMeta().Source,
		"a callback result arriving after cancellation must not replace the pending human decision")
}

func TestDoWaitAgreeWithPolicy_ManualHumanResponseBeatsAssistant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testAsyncTimeout)
	defer cancel()
	callbackStarted := make(chan struct{})
	allowLateReturn := make(chan struct{})
	callbackReturned := make(chan struct{})
	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyManual),
		WithAgreeManualCallback(func(_ context.Context, _ *Config) (aitool.InvokeParams, error) {
			close(callbackStarted)
			<-allowLateReturn
			close(callbackReturned)
			return aitool.InvokeParams{"suggestion": "assistant-late"}, nil
		}),
	)
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	done := make(chan struct{})
	go func() {
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyManual, ep)
		close(done)
	}()
	waitForSignal(t, callbackStarted, testAsyncTimeout, "manual assistant callback did not start")
	c.Epm.Feed(ep.GetId(), aitool.InvokeParams{"suggestion": "human-continue"})
	waitForSignal(t, done, testAsyncTimeout, "human response did not release manual review")
	close(allowLateReturn)
	waitForSignal(t, callbackReturned, testAsyncTimeout, "assistant callback did not return")
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, "human-continue", ep.GetParams().GetString("suggestion"))
	require.Equal(t, ApprovalSourceHuman, ep.GetApprovalMeta().Source)
}

func TestWaitAgreeCountdown_CancelledTimerDoesNotFire(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	require.False(t, waitAgreeCountdown(ctx, time.Second))
	require.Less(t, time.Since(started), testNoSignalWait)
}

func TestDoWaitAgreeWithPolicy_AICountdownCancellationDoesNotWriteLateContinue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	riskChecked := make(chan struct{})
	c := NewTestConfig(ctx,
		WithAgreePolicy(AgreePolicyAI),
		WithAiAgreeRiskControl(func(_ context.Context, _ *Config, _ *Endpoint) (*Action, error) {
			close(riskChecked)
			return NewSimpleAction("risk-check", aitool.InvokeParams{"risk_score": 0.1}), nil
		}),
	)
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
	done := make(chan struct{})
	go func() {
		c.DoWaitAgreeWithPolicy(ctx, AgreePolicyAI, ep)
		close(done)
	}()
	waitForSignal(t, riskChecked, testAsyncTimeout, "AI risk callback did not run")
	cancel()
	waitForSignal(t, done, testAsyncTimeout, "AI review wait did not observe cancellation")
	// The old time.Sleep path wrote and released one second later. Wait beyond
	// that boundary to make a late mutation deterministic if it regresses.
	time.Sleep(1100 * time.Millisecond)
	require.Empty(t, ep.GetParams(), "a cancelled countdown must not write a late continue")
	require.Nil(t, ep.GetApprovalMeta(), "a cancelled countdown must not publish a decision")
}
