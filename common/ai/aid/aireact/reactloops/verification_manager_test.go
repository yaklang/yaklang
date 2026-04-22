package reactloops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// fakeVerifyInvoker is a minimal VerifyInvoker used by the VerificationManager
// unit tests. It records the number of calls, the last received arguments,
// and can optionally replay a sequence of (result, error) pairs to simulate
// multi-call scenarios such as retries. Goroutine-safe.
type fakeVerifyInvoker struct {
	calls int32

	// Default fallback used when the sequence is exhausted (or never set).
	// These keep the historical simple tests working.
	result *aicommon.VerifySatisfactionResult
	err    error

	mu          sync.Mutex
	resultsSeq  []*aicommon.VerifySatisfactionResult
	errsSeq     []error
	seqIdx      int
	lastCtx     context.Context
	lastQuery   string
	lastIsTool  bool
	lastPayload string
}

func (f *fakeVerifyInvoker) VerifyUserSatisfaction(
	ctx context.Context,
	query string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastCtx = ctx
	f.lastQuery = query
	f.lastIsTool = isToolCall
	f.lastPayload = payload

	if f.seqIdx < len(f.resultsSeq) || f.seqIdx < len(f.errsSeq) {
		var r *aicommon.VerifySatisfactionResult
		var e error
		if f.seqIdx < len(f.resultsSeq) {
			r = f.resultsSeq[f.seqIdx]
		}
		if f.seqIdx < len(f.errsSeq) {
			e = f.errsSeq[f.seqIdx]
		}
		f.seqIdx++
		return r, e
	}
	return f.result, f.err
}

func (f *fakeVerifyInvoker) CallCount() int {
	return int(atomic.LoadInt32(&f.calls))
}

func (f *fakeVerifyInvoker) LastArgs() (string, bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastQuery, f.lastIsTool, f.lastPayload
}

// stubAICallerConfigForVerifier is a tiny stub implementing just
// GetTimelineContentSizeLimit (the shape WithAICallerConfig requires).
type stubAICallerConfigForVerifier struct {
	limit int64
}

func (s *stubAICallerConfigForVerifier) GetTimelineContentSizeLimit() int64 {
	return s.limit
}

// virtualClock produces deterministic timestamps that can be advanced from the
// test body. It is handed to the manager via WithNowFunc.
type virtualClock struct {
	t time.Time
}

func (c *virtualClock) now() time.Time {
	return c.t
}

func (c *virtualClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func newFakeInvoker() *fakeVerifyInvoker {
	return &fakeVerifyInvoker{
		result: aicommon.NewVerifySatisfactionResult(true, "ok", ""),
	}
}

// -------- ParseVerifyLevel --------

func TestVerificationManager_ParseVerifyLevel(t *testing.T) {
	cases := []struct {
		raw  string
		want VerifyLevel
	}{
		{"", VerifyLevelMiddle},
		{"none", VerifyLevelNone},
		{"NONE", VerifyLevelNone},
		{"None", VerifyLevelNone},
		{"low", VerifyLevelLow},
		{"LOW", VerifyLevelLow},
		{"middle", VerifyLevelMiddle},
		{"MIDDLE", VerifyLevelMiddle},
		{"force", VerifyLevelForce},
		{"Force", VerifyLevelForce},
		{"unknown-value", VerifyLevelMiddle},
		{"HIGH", VerifyLevelMiddle},
	}
	for _, c := range cases {
		got := ParseVerifyLevel(c.raw)
		require.Equalf(t, c.want, got, "ParseVerifyLevel(%q)", c.raw)
	}
}

// -------- ShouldVerify: throttle window --------

func TestVerificationManager_ShouldVerify_ThrottleWindow(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)

	force, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, force, "first call must not be force but normal")
	require.True(t, should, "first call must trigger normal verify (no lastVerifyAt)")
	require.Contains(t, reason, "normal")

	// Simulate a successful verify so the window starts.
	m.ResetWindow()

	force, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, force)
	require.False(t, should, "within 60s throttle window must skip")

	clock.advance(30 * time.Second)
	_, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should, "30s elapsed still within 60s window")

	clock.advance(31 * time.Second)
	force, should, reason = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, force)
	require.True(t, should, "61s elapsed must exit throttle window")
	require.Contains(t, reason, "normal")
}

// -------- ShouldVerify: verify_level=force bypasses throttle --------

func TestVerificationManager_ShouldVerify_ForceLevel(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	force, should, reason := m.ShouldVerify(VerifyLevelForce, EventToolResult)
	require.True(t, force)
	require.True(t, should)
	require.Contains(t, reason, "force")
	require.Contains(t, reason, "verify_level=force")
}

// -------- ShouldVerify: none/low skip (unless another force condition) --------

func TestVerificationManager_ShouldVerify_NoneLowSkip(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	// Move far beyond the throttle window so the only reason to verify would
	// be the middle-level default; none/low must still skip.
	clock.advance(10 * time.Minute)

	_, should, reason := m.ShouldVerify(VerifyLevelNone, EventToolResult)
	require.False(t, should, "verify_level=none must skip")
	require.Contains(t, reason, "skip")

	_, should, reason = m.ShouldVerify(VerifyLevelLow, EventToolResult)
	require.False(t, should, "verify_level=low must skip")
	require.Contains(t, reason, "skip")
}

// -------- ShouldVerify: none/low still forced by done/action/token --------

func TestVerificationManager_ShouldVerify_NoneLow_StillForcedByHigherPriority(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithActionRoundsThreshold(3),
		WithTimelineTokenThreshold(0),
		WithThrottleWindow(60*time.Second),
	)
	m.ResetWindow()
	// Accumulate enough actions to reach the rounds threshold.
	m.DeltaActionOffset(5)

	force, should, reason := m.ShouldVerify(VerifyLevelNone, EventToolResult)
	require.True(t, force, "action rounds must force even with verify_level=none")
	require.True(t, should)
	require.Contains(t, reason, "action rounds")
}

// -------- ShouldVerify: done/finish flag --------

func TestVerificationManager_ShouldVerify_DoneFinish(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
	)
	m.ResetWindow()

	// (1) Event-based @done triggers force even without MarkDone.
	force, should, reason := m.ShouldVerify(VerifyLevelNone, EventDone)
	require.True(t, force)
	require.True(t, should)
	require.Contains(t, reason, "done")

	// (2) MarkDone then call with a regular tool result event must also force.
	m.MarkDone()
	require.True(t, m.GetDoneFlag())
	force, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, force)
	require.True(t, should)

	// (3) MarkFinish has the same effect as MarkDone.
	m.ResetWindow()
	require.False(t, m.GetDoneFlag(), "ResetWindow clears done flag")
	m.MarkFinish()
	require.True(t, m.GetDoneFlag())
	force, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, force)
	require.True(t, should)
}

// -------- ShouldVerify: action rounds --------

func TestVerificationManager_ShouldVerify_ActionRounds(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(3),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventAction)
	require.False(t, should, "no actions yet, within throttle -> skip")

	m.DeltaActionOffset(1)
	_, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventAction)
	require.False(t, should)

	m.DeltaActionOffset(1)
	_, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventAction)
	require.False(t, should, "count=2 still below threshold=3")

	m.DeltaActionOffset(1)
	force, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventAction)
	require.True(t, should, "count=3 reaches threshold -> force")
	require.True(t, force)
	require.Contains(t, reason, "action rounds")
}

// -------- ShouldVerify: timeline token delta --------

func TestVerificationManager_ShouldVerify_TimelineTokenDelta(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	timeline := aicommon.NewTimeline(nil, nil)

	// Use a small token threshold so we can cross it with a handful of tool
	// results.
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithTimeline(timeline),
		WithTimelineTokenThreshold(32),
		WithActionRoundsThreshold(0),
		WithThrottleWindow(60*time.Second),
	)
	m.ResetWindow()

	// Initially no growth since the baseline captured the empty dump.
	require.Equal(t, 0, m.GetTimelineTokenOffset())
	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should, "fresh baseline within throttle -> skip")

	// Push content substantial enough to cross 32 tokens. Repeat push to be
	// safe across tokenizer variations.
	bigPayload := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 10)
	for i := 0; i < 3; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(1_000 + i),
			Name:        fmt.Sprintf("noisy_tool_%d", i),
			Description: bigPayload,
			Param:       map[string]any{"i": i},
			Success:     true,
			Data:        bigPayload,
		})
	}

	require.Greater(t, m.GetTimelineTokenOffset(), 32, "expected token delta to exceed threshold")

	force, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, force)
	require.True(t, should)
	require.Contains(t, reason, "timeline token delta")
}

// -------- AutoVerify: invoker only called when triggered --------

func TestVerificationManager_AutoVerify_InvokesInvokerOnlyWhenTriggered(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	invoker := newFakeInvoker()
	m := NewVerificationManager(
		invoker,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(5),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	// (1) Skip path: verify_level=none must not hit the invoker.
	result, ran, err := m.AutoVerifyUserSatisfaction(
		context.Background(), "q", true, "payload", VerifyLevelNone, EventToolResult,
	)
	require.NoError(t, err)
	require.False(t, ran)
	require.Nil(t, result)
	require.Equal(t, 0, invoker.CallCount())

	// (2) Trigger path: move past throttle window with middle-level.
	clock.advance(61 * time.Second)
	m.DeltaActionOffset(2)
	before := m.GetLastVerifyAt()

	result, ran, err = m.AutoVerifyUserSatisfaction(
		context.Background(), "q", true, "payload", VerifyLevelMiddle, EventToolResult,
	)
	require.NoError(t, err)
	require.True(t, ran)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.CallCount())

	// Post-verify window must be reset.
	require.Equal(t, 0, m.GetActionOffset(), "action offset should reset")
	require.Equal(t, 0, m.GetTimelineTokenOffset(), "timeline token offset should reset")
	require.True(t, m.GetLastVerifyAt().After(before) || m.GetLastVerifyAt().Equal(clock.now()),
		"lastVerifyAt should refresh")
}

// -------- ForceVerify: always runs and resets --------

func TestVerificationManager_ForceVerify_AlwaysRunsAndResets(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	invoker := newFakeInvoker()
	m := NewVerificationManager(
		invoker,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(100),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	m.DeltaActionOffset(3)

	// Even though throttle would skip a normal middle-level call, force must run.
	result, err := m.ForceVerifyUserSatisfaction(context.Background(), "q", true, "payload")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.CallCount())
	require.Equal(t, 0, m.GetActionOffset())
	require.False(t, m.GetDoneFlag())
}

// -------- ForceVerify: error is propagated and window is NOT reset --------

func TestVerificationManager_ForceVerify_PropagatesError_NoReset(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	invoker := &fakeVerifyInvoker{err: fmt.Errorf("verify backend exploded")}
	m := NewVerificationManager(
		invoker,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	lastBefore := m.GetLastVerifyAt()
	m.DeltaActionOffset(4)

	result, err := m.ForceVerifyUserSatisfaction(context.Background(), "q", true, "payload")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, invoker.CallCount())

	// Window must be preserved: action offset intact, lastVerifyAt unchanged.
	require.Equal(t, 4, m.GetActionOffset(), "failed verify must not reset action offset")
	require.True(t, m.GetLastVerifyAt().Equal(lastBefore), "failed verify must not refresh lastVerifyAt")
}

// -------- AutoVerify: invoker error does not reset window --------

func TestVerificationManager_AutoVerify_ErrorNoReset(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	invoker := &fakeVerifyInvoker{err: fmt.Errorf("boom")}
	m := NewVerificationManager(
		invoker,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	lastBefore := m.GetLastVerifyAt()

	// Force path so we definitely hit the invoker.
	_, ran, err := m.AutoVerifyUserSatisfaction(
		context.Background(), "q", true, "p", VerifyLevelForce, EventToolResult,
	)
	require.Error(t, err)
	require.True(t, ran)
	require.Equal(t, 1, invoker.CallCount())
	require.True(t, m.GetLastVerifyAt().Equal(lastBefore), "error path must not reset lastVerifyAt")
}

// -------- ForceAsyncVerify: placeholder returns a closed channel --------

func TestVerificationManager_ForceAsyncVerify_NotImplemented(t *testing.T) {
	m := NewVerificationManager(newFakeInvoker())
	ch := m.ForceAsyncVerifyUserSatisfaction(context.Background(), "q", false, "p")
	require.NotNil(t, ch)
	// Must be closed immediately; receiving from a closed empty channel yields
	// the zero value (nil *VerifyAsyncResult) with ok=false.
	select {
	case _, ok := <-ch:
		require.False(t, ok, "placeholder channel must already be closed")
	case <-time.After(1 * time.Second):
		t.Fatal("async verify channel should be immediately closed")
	}
}

// -------- Clock injection smoke test --------

func TestVerificationManager_NowInjection(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(30*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	require.Equal(t, clock.t, m.GetLastVerifyAt())

	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should, "no time advanced yet -> within window")

	clock.advance(31 * time.Second)
	_, should, _ = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, should, "advanced 31s -> past 30s window")
}

// -------- Counter helpers --------

func TestVerificationManager_ActionOffsetHelpers(t *testing.T) {
	m := NewVerificationManager(newFakeInvoker())
	require.Equal(t, 0, m.GetActionOffset())

	m.SetActionOffset(7)
	require.Equal(t, 7, m.GetActionOffset())

	require.Equal(t, 10, m.DeltaActionOffset(3))
	require.Equal(t, 10, m.GetActionOffset())

	// Negative delta saturates at zero.
	require.Equal(t, 0, m.DeltaActionOffset(-100))
	require.Equal(t, 0, m.GetActionOffset())

	// NotifyAction increments by 1.
	require.Equal(t, 1, m.NotifyAction(VerifyLevelMiddle))
	require.Equal(t, 1, m.GetActionOffset())
}

func TestVerificationManager_TimelineTokenBaselineHelpers(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	m := NewVerificationManager(
		newFakeInvoker(),
		WithTimeline(timeline),
	)
	require.Equal(t, 0, m.GetTimelineTokenOffset(), "fresh timeline baseline is zero")

	timeline.PushToolResult(&aitool.ToolResult{
		ID:          42,
		Name:        "probe_tool",
		Description: strings.Repeat("alpha ", 50),
		Param:       map[string]any{"a": 1},
		Success:     true,
		Data:        strings.Repeat("beta ", 50),
	})
	offsetAfterPush := m.GetTimelineTokenOffset()
	require.Greater(t, offsetAfterPush, 0, "pushing data should grow offset")

	m.ResetTimelineTokenBaseline()
	require.Equal(t, 0, m.GetTimelineTokenOffset(), "after ResetTimelineTokenBaseline offset must be zero")

	// Explicit override: setting baseline=-100 yields offset = currentTokens + 100.
	m.SetTimelineTokenBaseline(-100)
	require.Equal(t, offsetAfterPush+100, m.GetTimelineTokenOffset())
}

// -------- Nil timeline path --------

func TestVerificationManager_NilTimelineOffsetIsZero(t *testing.T) {
	m := NewVerificationManager(newFakeInvoker())
	require.Equal(t, 0, m.GetTimelineTokenOffset(), "no timeline attached -> zero offset always")
	m.ResetTimelineTokenBaseline() // must not panic
	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	// First call with zero lastVerifyAt always passes the throttle gate.
	require.True(t, should)
}

// ============================================================================
// Section A: Threshold strategy (hybrid: explicit > provider > fallback)
// ============================================================================

// TestVerificationManager_Threshold_ExplicitOverrideWins verifies that when a
// caller supplies both WithTimelineTokenThreshold and WithTimelineLimitProvider,
// the explicit value wins (provider is ignored for default derivation).
func TestVerificationManager_Threshold_ExplicitOverrideWins(t *testing.T) {
	m := NewVerificationManager(
		newFakeInvoker(),
		WithTimelineTokenThreshold(500),
		WithTimelineLimitProvider(func() int64 { return 60000 }),
	)
	require.Equal(t, 500, m.GetTimelineTokenThreshold(),
		"explicit threshold must win over provider-derived value")
}

// TestVerificationManager_Threshold_DynamicFromProvider drives a table of
// provider outputs through the hybrid derivation path and asserts the
// resulting threshold is in the expected range.
func TestVerificationManager_Threshold_DynamicFromProvider(t *testing.T) {
	cases := []struct {
		name        string
		limit       int64
		wantLow     int
		wantHigh    int
		wantExact   int
		useExact    bool
		description string
	}{
		{name: "default_50k", limit: 51200, wantLow: 16000, wantHigh: 18000, description: "50k/3 ~= 17066"},
		{name: "larger_60k", limit: 60000, wantExact: 20000, useExact: true, description: "60000/3 = 20000"},
		{name: "small_3k_clamped_to_min", limit: 3000, wantExact: DefaultVerifyTimelineTokenDeltaMin, useExact: true, description: "3000/3=1000 < min=4096"},
		{name: "zero_falls_back", limit: 0, wantExact: DefaultVerifyTimelineTokenDeltaFallback, useExact: true, description: "no usable limit -> fallback"},
		{name: "negative_falls_back", limit: -1, wantExact: DefaultVerifyTimelineTokenDeltaFallback, useExact: true, description: "negative limit -> fallback"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			limit := c.limit
			m := NewVerificationManager(
				newFakeInvoker(),
				WithTimelineLimitProvider(func() int64 { return limit }),
			)
			got := m.GetTimelineTokenThreshold()
			if c.useExact {
				require.Equal(t, c.wantExact, got, c.description)
				return
			}
			require.GreaterOrEqual(t, got, c.wantLow, c.description)
			require.LessOrEqual(t, got, c.wantHigh, c.description)
		})
	}
}

// TestVerificationManager_Threshold_FallbackWhenNoProvider verifies that
// without any explicit threshold or provider, the manager uses the fallback.
func TestVerificationManager_Threshold_FallbackWhenNoProvider(t *testing.T) {
	m := NewVerificationManager(newFakeInvoker())
	require.Equal(t, DefaultVerifyTimelineTokenDeltaFallback, m.GetTimelineTokenThreshold())
}

// TestVerificationManager_Threshold_ExplicitZeroDisables verifies that an
// explicit threshold of 0 disables the token-delta trigger even when the
// timeline grows by a massive amount.
func TestVerificationManager_Threshold_ExplicitZeroDisables(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	inv := newFakeInvoker()
	m := NewVerificationManager(
		inv,
		WithTimeline(timeline),
		WithTimelineTokenThreshold(0),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
	)
	m.ResetWindow() // put us inside the throttle window
	for i := 0; i < 30; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(1000 + i),
			Name:        "mega_tool",
			Description: strings.Repeat("ALPHA ", 500),
			Param:       map[string]any{"k": i},
			Success:     true,
			Data:        strings.Repeat("BETA ", 500),
		})
	}
	require.Greater(t, m.GetTimelineTokenOffset(), 10000,
		"test pre-condition: pushed enough data for a huge delta")

	_, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should,
		"token threshold=0 must disable the token-delta trigger; got reason=%s", reason)
}

// TestVerificationManager_Threshold_RefreshFromProvider verifies that
// RefreshTimelineTokenThresholdFromProvider picks up a provider value change
// at runtime. The provider closes over a mutable int64 so we can move it.
func TestVerificationManager_Threshold_RefreshFromProvider(t *testing.T) {
	var limit int64 = 30000
	m := NewVerificationManager(
		newFakeInvoker(),
		WithTimelineLimitProvider(func() int64 { return limit }),
	)
	require.Equal(t, 10000, m.GetTimelineTokenThreshold(), "30000/3=10000")

	atomic.StoreInt64(&limit, 60000)
	newThreshold := m.RefreshTimelineTokenThresholdFromProvider()
	require.Equal(t, 20000, newThreshold)
	require.Equal(t, 20000, m.GetTimelineTokenThreshold())

	atomic.StoreInt64(&limit, 0)
	require.Equal(t, DefaultVerifyTimelineTokenDeltaFallback, m.RefreshTimelineTokenThresholdFromProvider())
}

// TestVerificationManager_Threshold_RefreshIgnoredWhenExplicit ensures that
// refresh is a no-op when the threshold was set explicitly.
func TestVerificationManager_Threshold_RefreshIgnoredWhenExplicit(t *testing.T) {
	var limit int64 = 30000
	m := NewVerificationManager(
		newFakeInvoker(),
		WithTimelineTokenThreshold(777),
		WithTimelineLimitProvider(func() int64 { return limit }),
	)
	require.Equal(t, 777, m.GetTimelineTokenThreshold())
	atomic.StoreInt64(&limit, 60000)
	require.Equal(t, 777, m.RefreshTimelineTokenThresholdFromProvider(),
		"refresh must be skipped when the threshold was explicitly set")
	require.Equal(t, 777, m.GetTimelineTokenThreshold())
}

// TestVerificationManager_WithAICallerConfig_Wrapper verifies that
// WithAICallerConfig is equivalent to WithTimelineLimitProvider.
func TestVerificationManager_WithAICallerConfig_Wrapper(t *testing.T) {
	cfg := &stubAICallerConfigForVerifier{limit: 60000}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithAICallerConfig(cfg),
	)
	require.Equal(t, 20000, m.GetTimelineTokenThreshold(),
		"config provides 60000 -> threshold=60000/3=20000")

	// Nil config must be a safe no-op and fall through to fallback.
	m2 := NewVerificationManager(newFakeInvoker(), WithAICallerConfig(nil))
	require.Equal(t, DefaultVerifyTimelineTokenDeltaFallback, m2.GetTimelineTokenThreshold())
}

// ============================================================================
// Section B: ShouldVerify boundary cases
// ============================================================================

func TestShouldVerify_ActionRoundsThreshold_ZeroOrNegative(t *testing.T) {
	cases := []int{0, -1}
	for _, threshold := range cases {
		threshold := threshold
		t.Run(fmt.Sprintf("threshold_%d", threshold), func(t *testing.T) {
			clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
			m := NewVerificationManager(
				newFakeInvoker(),
				WithNowFunc(clock.now),
				WithThrottleWindow(60*time.Second),
				WithActionRoundsThreshold(threshold),
				WithTimelineTokenThreshold(0),
			)
			m.ResetWindow()
			m.SetActionOffset(1_000_000) // absurdly large
			_, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
			require.False(t, should,
				"threshold<=0 must disable action-rounds trigger; reason=%s", reason)
		})
	}
}

func TestShouldVerify_ThrottleWindow_Zero(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(0),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, should, "zero throttle window must always admit normal verify")
}

func TestShouldVerify_ExactBoundary_TokenDelta(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithTimeline(timeline),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(100),
	)
	m.ResetWindow()
	// Baseline at 0. Tweak baseline so that GetTimelineTokenOffset reports
	// exactly 99 and exactly 100 without needing to push real tokens.
	m.SetTimelineTokenBaseline(-99)
	force, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, force, reason)
	require.False(t, should, "delta=99 < threshold=100 must skip inside throttle, reason=%s", reason)

	m.SetTimelineTokenBaseline(-100)
	force, should, reason = m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, force, "delta==threshold must be force, reason=%s", reason)
	require.True(t, should)
}

func TestShouldVerify_ExactBoundary_ActionRounds(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	m := NewVerificationManager(
		newFakeInvoker(),
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(5),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	m.SetActionOffset(4)
	_, should, _ := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should, "action=4 < threshold=5 inside throttle -> skip")

	m.SetActionOffset(5)
	force, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.True(t, force, reason)
	require.True(t, should, "action==threshold must force verify")
}

func TestShouldVerify_PriorityOrder(t *testing.T) {
	type step struct {
		doneFlag      bool
		tokenBaseline int
		actionCount   int
		level         VerifyLevel
		event         VerifyTriggerEvent
		afterReset    bool
		wantForce     bool
		wantShould    bool
		reasonIncl    string
	}
	cases := []struct {
		name string
		s    step
	}{
		{name: "done_beats_all", s: step{doneFlag: true, tokenBaseline: -50, actionCount: 2, level: VerifyLevelNone, event: EventToolResult, afterReset: true, wantForce: true, wantShould: true, reasonIncl: "done/finish"}},
		{name: "token_beats_action", s: step{tokenBaseline: -200, actionCount: 2, level: VerifyLevelNone, event: EventToolResult, afterReset: true, wantForce: true, wantShould: true, reasonIncl: "timeline token"}},
		{name: "action_beats_levelforce", s: step{tokenBaseline: -10, actionCount: 5, level: VerifyLevelForce, event: EventAction, afterReset: true, wantForce: true, wantShould: true, reasonIncl: "action rounds"}},
		{name: "levelforce_inside_throttle", s: step{tokenBaseline: -10, actionCount: 2, level: VerifyLevelForce, event: EventToolResult, afterReset: true, wantForce: true, wantShould: true, reasonIncl: "verify_level=force"}},
		{name: "levelnone_skips_throttle", s: step{tokenBaseline: -10, actionCount: 2, level: VerifyLevelNone, event: EventToolResult, afterReset: true, wantForce: false, wantShould: false, reasonIncl: "skip"}},
		{name: "levellow_skips_throttle", s: step{tokenBaseline: -10, actionCount: 2, level: VerifyLevelLow, event: EventToolResult, afterReset: true, wantForce: false, wantShould: false, reasonIncl: "skip"}},
		{name: "middle_outside_throttle_normal", s: step{tokenBaseline: -10, actionCount: 2, level: VerifyLevelMiddle, event: EventToolResult, afterReset: false, wantForce: false, wantShould: true, reasonIncl: "normal"}},
		{name: "middle_inside_throttle_skip", s: step{tokenBaseline: -10, actionCount: 2, level: VerifyLevelMiddle, event: EventToolResult, afterReset: true, wantForce: false, wantShould: false, reasonIncl: "within throttle"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
			m := NewVerificationManager(
				newFakeInvoker(),
				WithNowFunc(clock.now),
				WithThrottleWindow(60*time.Second),
				WithActionRoundsThreshold(5),
				WithTimelineTokenThreshold(100),
			)
			if c.s.afterReset {
				m.ResetWindow()
			}
			if c.s.doneFlag {
				m.MarkDone()
			}
			m.SetTimelineTokenBaseline(c.s.tokenBaseline)
			m.SetActionOffset(c.s.actionCount)
			force, should, reason := m.ShouldVerify(c.s.level, c.s.event)
			require.Equal(t, c.s.wantForce, force, "reason=%s", reason)
			require.Equal(t, c.s.wantShould, should, "reason=%s", reason)
			require.Contains(t, strings.ToLower(reason), strings.ToLower(c.s.reasonIncl))
		})
	}
}

func TestShouldVerify_DoneFlagConsumption(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	m.MarkDone()
	require.True(t, m.GetDoneFlag())

	// Run Auto verify -> invoker called + doneFlag cleared.
	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", false, "p", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	require.False(t, m.GetDoneFlag(), "doneFlag must be reset after a successful verify")

	// Next ShouldVerify: only throttle remains, and we're inside it.
	_, should, reason := m.ShouldVerify(VerifyLevelMiddle, EventToolResult)
	require.False(t, should, "after done consumed, throttle gate applies; reason=%s", reason)
}

func TestShouldVerify_ForceLevelInThrottle(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()
	// Inside throttle, level=force must still fire and reset.
	result, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", true, "payload", VerifyLevelForce, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	require.NotNil(t, result)
	require.Equal(t, 1, inv.CallCount())
	// Window reset: lastVerifyAt == clock.now
	require.Equal(t, clock.t, m.GetLastVerifyAt())
}

// ============================================================================
// Section C: Scenario tests simulating the 7 future integration points plus
//            a few aggregate policy scenarios (N rounds, done-beats-throttle,
//            error retry, evidence pass-through).
// ============================================================================

// newScenarioManager builds a manager pre-configured the way the real
// ReAct loop will eventually build it: explicit action/window/token knobs and
// a virtual clock.
func newScenarioManager(t *testing.T, inv *fakeVerifyInvoker, clock *virtualClock) *VerificationManager {
	t.Helper()
	return NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(5),
		WithTimelineTokenThreshold(1000),
	)
}

// TestScenario_FinishAction_ForcesVerify mirrors action_buildin.go:17 where
// @finish must trigger verification regardless of throttle.
func TestScenario_FinishAction_ForcesVerify(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)
	m.ResetWindow()
	m.MarkFinish()

	result, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "", false, "", VerifyLevelMiddle, EventFinish)
	require.NoError(t, err)
	require.True(t, ran)
	require.NotNil(t, result)
	require.Equal(t, 1, inv.CallCount())
	_, isTool, payload := inv.LastArgs()
	require.False(t, isTool, "finish path is not a tool call")
	require.Equal(t, "", payload)
	require.False(t, m.GetDoneFlag(), "finish flag must be consumed")
	require.Equal(t, clock.t, m.GetLastVerifyAt(), "window reset to now")
}

// TestScenario_ToolCall_FirstCallPassesThrottle mirrors tool_call_common.go:74
// for a freshly-created manager: the very first decision must let middle pass.
func TestScenario_ToolCall_FirstCallPassesThrottle(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	result, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "query", true, "toolpayload", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran, "first call (no prior verify) must pass throttle")
	require.NotNil(t, result)
	require.Equal(t, 1, inv.CallCount())
}

// TestScenario_ToolCall_WithinThrottle_Skips asserts a second middle call
// within the window is skipped.
func TestScenario_ToolCall_WithinThrottle_Skips(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)
	m.ResetWindow()

	clock.advance(10 * time.Second) // still inside 60s window
	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", true, "p", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.False(t, ran, "must be throttled")
	require.Equal(t, 0, inv.CallCount())
}

// TestScenario_ToolCall_TimelineExplosion_ForceVerify mirrors the case where
// tool results blew up the timeline: even inside throttle, verification must
// fire thanks to the token-delta trigger derived from AICallerConfig.
func TestScenario_ToolCall_TimelineExplosion_ForceVerify(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	timeline := aicommon.NewTimeline(nil, nil)
	cfg := &stubAICallerConfigForVerifier{limit: 30000} // threshold becomes 10000
	m := NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimeline(timeline),
		WithAICallerConfig(cfg),
	)
	require.Equal(t, 10000, m.GetTimelineTokenThreshold())
	m.ResetWindow()

	// Push enough timeline content to exceed 10000 tokens of delta.
	for i := 0; i < 60; i++ {
		timeline.PushToolResult(&aitool.ToolResult{
			ID:          int64(2000 + i),
			Name:        "boom_tool",
			Description: strings.Repeat("gamma ", 1000),
			Success:     true,
			Data:        strings.Repeat("delta ", 1000),
		})
	}
	require.Greater(t, m.GetTimelineTokenOffset(), 10000,
		"pre-condition: timeline growth exceeds threshold")

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", true, "p", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran, "timeline explosion must force verify inside throttle")
	require.Equal(t, 1, inv.CallCount())
	// After reset, baseline snapped to current tokens -> offset back to 0.
	require.Equal(t, 0, m.GetTimelineTokenOffset())
}

// TestScenario_ToolCall_VerifyLevelForceHint asserts that a tool that emits
// verify_level=force triggers verification even within the throttle window.
func TestScenario_ToolCall_VerifyLevelForceHint(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)
	m.ResetWindow()
	clock.advance(5 * time.Second) // still inside throttle

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", true, "p", VerifyLevelForce, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran, "verify_level=force must bypass throttle")
	require.Equal(t, 1, inv.CallCount())
}

// TestScenario_ToolCall_VerifyLevelNoneHint_NoOtherTrigger ensures level=none
// skips when there are no other triggers firing.
func TestScenario_ToolCall_VerifyLevelNoneHint_NoOtherTrigger(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)
	m.ResetWindow()
	clock.advance(10 * time.Second)

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", true, "p", VerifyLevelNone, EventToolResult)
	require.NoError(t, err)
	require.False(t, ran)
	require.Equal(t, 0, inv.CallCount())
}

// TestScenario_ToolCompose_Payload mirrors action_tool_compose.go:291. The
// payload emitted by the AI must be transparently carried through to the
// invoker.
func TestScenario_ToolCompose_Payload(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	const payload = "composeX"
	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "compose-query", true, payload, VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	q, isTool, p := inv.LastArgs()
	require.Equal(t, "compose-query", q)
	require.True(t, isTool)
	require.Equal(t, payload, p, "payload must pass through unchanged")
}

// TestScenario_LoadCapability_IdentifierPayload mirrors
// action_load_capability.go:166.
func TestScenario_LoadCapability_IdentifierPayload(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	const payload = "cap:my-identifier"
	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "load-cap", true, payload, VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	_, _, p := inv.LastArgs()
	require.Equal(t, payload, p)
}

// TestScenario_EnhanceKnowledgeAnswer mirrors
// action_enhance_knowledge_answer.go:117.
func TestScenario_EnhanceKnowledgeAnswer(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "enhance-q", true, "enhance-payload", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	q, isTool, p := inv.LastArgs()
	require.Equal(t, "enhance-q", q)
	require.True(t, isTool)
	require.Equal(t, "enhance-payload", p)
}

// TestScenario_ActionFromTool mirrors action_from_tool.go:205.
func TestScenario_ActionFromTool(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "action-from-tool", true, "aft-payload", VerifyLevelMiddle, EventAction)
	require.NoError(t, err)
	require.True(t, ran)
}

// TestScenario_FuzzTestVerify mirrors loop_http_fuzztest/fuzz_utils.go:1075.
func TestScenario_FuzzTestVerify(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "fuzz-query", true, "fuzz-payload", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran)
	q, isTool, _ := inv.LastArgs()
	require.Equal(t, "fuzz-query", q)
	require.True(t, isTool)
}

// TestScenario_NRoundsForce asserts the action-rounds trigger fires exactly on
// the Nth action even when every individual call uses level=none.
func TestScenario_NRoundsForce(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(5),
		WithTimelineTokenThreshold(0),
	)
	m.ResetWindow()

	var runs int
	for i := 1; i <= 5; i++ {
		m.NotifyAction(VerifyLevelNone)
		_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", false, "p", VerifyLevelNone, EventAction)
		require.NoError(t, err)
		if ran {
			runs++
			require.Equal(t, 5, i, "verify must only run on the 5th round, got round=%d", i)
		}
	}
	require.Equal(t, 1, runs, "exactly one forced verify over 5 rounds")
	require.Equal(t, 1, inv.CallCount())
	require.Equal(t, 0, m.GetActionOffset(), "window reset zeroed the action counter")
}

// TestScenario_DoneBeatsThrottle verifies that MarkDone overrides a fresh
// throttle window.
func TestScenario_DoneBeatsThrottle(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := newFakeInvoker()
	m := newScenarioManager(t, inv, clock)
	m.ResetWindow()          // inside throttle from now on
	clock.advance(1 * time.Second)
	m.MarkDone()

	_, ran, err := m.AutoVerifyUserSatisfaction(context.Background(), "q", false, "p", VerifyLevelMiddle, EventToolResult)
	require.NoError(t, err)
	require.True(t, ran, "done must beat throttle")
	require.Equal(t, 1, inv.CallCount())
}

// TestScenario_ErrorPath_RetryOnNextTrigger asserts that an invoker error
// does NOT reset the window, so the next forced trigger still fires.
func TestScenario_ErrorPath_RetryOnNextTrigger(t *testing.T) {
	clock := &virtualClock{t: time.Unix(1_700_000_000, 0)}
	inv := &fakeVerifyInvoker{
		resultsSeq: []*aicommon.VerifySatisfactionResult{
			nil,
			aicommon.NewVerifySatisfactionResult(true, "ok-on-retry", ""),
		},
		errsSeq: []error{
			fmt.Errorf("transient invoker failure"),
			nil,
		},
	}
	m := NewVerificationManager(
		inv,
		WithNowFunc(clock.now),
		WithThrottleWindow(60*time.Second),
		WithActionRoundsThreshold(0),
		WithTimelineTokenThreshold(0),
	)

	// First force call fails.
	_, err := m.ForceVerifyUserSatisfaction(context.Background(), "q1", false, "p1")
	require.Error(t, err)
	require.True(t, m.GetLastVerifyAt().IsZero(),
		"window must NOT be reset after a failed invocation so retry can happen")

	// Second force call succeeds.
	result, err := m.ForceVerifyUserSatisfaction(context.Background(), "q2", false, "p2")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "ok-on-retry", result.Reasoning)
	require.Equal(t, 2, inv.CallCount())
	require.Equal(t, clock.t, m.GetLastVerifyAt(), "successful call must reset the window")
}

// TestScenario_EvidenceOpsPassThrough verifies the manager returns whatever
// evidence/next-movement/output-file payload the invoker produced, without
// modification.
func TestScenario_EvidenceOpsPassThrough(t *testing.T) {
	richResult := &aicommon.VerifySatisfactionResult{
		Satisfied:          true,
		Reasoning:          "all good",
		CompletedTaskIndex: "1-1",
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "continue", Content: "run next tool", ID: "m1"},
		},
		EvidenceOps: []aicommon.EvidenceOperation{
			{ID: "e1", Op: "add", Content: "proof"},
		},
		OutputFiles: []string{"/tmp/out.txt"},
	}
	inv := &fakeVerifyInvoker{result: richResult}
	m := NewVerificationManager(inv, WithTimelineTokenThreshold(0))

	got, err := m.ForceVerifyUserSatisfaction(context.Background(), "q", false, "p")
	require.NoError(t, err)
	require.Same(t, richResult, got, "manager must pass invoker's result pointer through unchanged")
	require.Equal(t, []string{"/tmp/out.txt"}, got.OutputFiles)
	require.Len(t, got.EvidenceOps, 1)
	require.Len(t, got.NextMovements, 1)
}
