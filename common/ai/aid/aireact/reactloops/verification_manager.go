package reactloops

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/log"
)

// VerifyLevel describes the suggested verification strength attached to a tool
// parameter or event. The AI is expected to hint at how aggressively the system
// should verify the resulting state after executing the tool.
//
//   - none   / low: the tool result does not need verification unless forced by
//     other signals (timeline token growth, action rounds, done/finish).
//   - middle:       follow system policy: check throttle window / action rounds /
//     timeline token delta; this is the default when the field is omitted.
//   - force:        verify immediately after the tool returns, bypassing the
//     throttle window.
type VerifyLevel string

const (
	VerifyLevelNone   VerifyLevel = "none"
	VerifyLevelLow    VerifyLevel = "low"
	VerifyLevelMiddle VerifyLevel = "middle"
	VerifyLevelForce  VerifyLevel = "force"
)

// ParseVerifyLevel normalizes a raw string (typically sourced from an AI
// generated tool parameter named "verify_level") into a VerifyLevel. Empty
// string or unknown values fall back to VerifyLevelMiddle so callers always
// receive a safe default.
func ParseVerifyLevel(raw string) VerifyLevel {
	switch {
	case len(raw) == 0:
		return VerifyLevelMiddle
	}
	// Normalize in a single pass.
	lower := make([]byte, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		lower[i] = c
	}
	switch string(lower) {
	case string(VerifyLevelNone):
		return VerifyLevelNone
	case string(VerifyLevelLow):
		return VerifyLevelLow
	case string(VerifyLevelMiddle):
		return VerifyLevelMiddle
	case string(VerifyLevelForce):
		return VerifyLevelForce
	default:
		return VerifyLevelMiddle
	}
}

// VerifyTriggerEvent identifies the source of a verification decision request.
// It is used both for logging and for explicit @done / @finish handling.
type VerifyTriggerEvent string

const (
	EventToolResult VerifyTriggerEvent = "tool_result"
	EventAction     VerifyTriggerEvent = "action"
	EventDone       VerifyTriggerEvent = "done"
	EventFinish     VerifyTriggerEvent = "finish"
)

// Default thresholds for the verification manager. These are exported so
// external callers can reference the same defaults when constructing their
// own configurations.
//
// The timeline token delta default follows a hybrid strategy:
//
//  1. If the caller supplies an explicit threshold via
//     WithTimelineTokenThreshold, that value wins (including 0 / negative
//     which disables the trigger).
//  2. Else, if a TimelineLimitProvider is supplied (directly or via
//     WithAICallerConfig), the threshold is computed as
//     max(limit / DefaultVerifyTimelineLimitDivisor, DefaultVerifyTimelineTokenDeltaMin).
//     This targets a verification roughly after the timeline has grown by
//     one third of its compression cap, giving us a chance to verify before
//     the timeline is compressed and evidence gets rewritten.
//  3. Else, fall back to DefaultVerifyTimelineTokenDeltaFallback.
const (
	DefaultVerifyThrottleWindow             = 60 * time.Second
	DefaultVerifyActionRoundsThreshold      = 5
	DefaultVerifyTimelineTokenDeltaFallback = 16384 // ~ 50*1024 / 3, used when no provider is attached
	DefaultVerifyTimelineTokenDeltaMin      = 8192  // lower clamp when limit/3 is too small / 8K
	DefaultVerifyTimelineLimitDivisor       = 3     // threshold = limit / divisor
)

// VerifyInvoker is the narrow interface that VerificationManager depends on to
// actually execute a verification. It intentionally contains only the single
// method so unit tests can provide a lightweight mock without implementing the
// entire aicommon.AIInvokeRuntime surface. aicommon.AIInvokeRuntime already
// satisfies this interface.
type VerifyInvoker interface {
	VerifyUserSatisfaction(
		ctx context.Context,
		query string,
		isToolCall bool,
		payload string,
	) (*aicommon.VerifySatisfactionResult, error)
}

// VerifyAsyncResult wraps the outcome of an asynchronous verification. It is
// kept exported for future callers even though ForceAsyncVerifyUserSatisfaction
// is currently a placeholder.
type VerifyAsyncResult struct {
	Result *aicommon.VerifySatisfactionResult
	Err    error
}

// TimelineLimitProvider returns the current timeline token limit, typically
// obtained from aicommon.AICallerConfigIf.GetTimelineContentSizeLimit. A
// return value of zero or a negative number is treated as "no limit known".
type TimelineLimitProvider func() int64

// timelineLimitFromConfig is the narrow interface we need to derive a limit
// provider from a config object, avoiding a hard dependency on the full
// aicommon.AICallerConfigIf surface inside this file's helpers.
type timelineLimitFromConfig interface {
	GetTimelineContentSizeLimit() int64
}

// VerifyManagerOption configures a VerificationManager at construction time.
type VerifyManagerOption func(*VerificationManager)

// WithThrottleWindow overrides the minimum duration between two successive
// (non-forced) verification runs. A value of zero disables throttling entirely.
func WithThrottleWindow(window time.Duration) VerifyManagerOption {
	return func(m *VerificationManager) {
		m.throttleWindow = window
	}
}

// WithActionRoundsThreshold overrides the action rounds threshold. Set to zero
// or a negative value to disable the action-rounds based forced verification.
func WithActionRoundsThreshold(threshold int) VerifyManagerOption {
	return func(m *VerificationManager) {
		m.actionRoundsThreshold = threshold
	}
}

// WithTimelineTokenThreshold overrides the timeline token delta threshold. Set
// to zero or a negative value to disable the timeline-token based forced
// verification. When this option is supplied (with any value), the hybrid
// default-derivation based on TimelineLimitProvider is skipped.
func WithTimelineTokenThreshold(threshold int) VerifyManagerOption {
	return func(m *VerificationManager) {
		m.timelineTokenThreshold = threshold
		m.thresholdExplicitlySet = true
	}
}

// WithTimelineLimitProvider attaches a dynamic provider from which the default
// timeline token delta threshold is derived (limit / DefaultVerifyTimelineLimitDivisor,
// clamped to DefaultVerifyTimelineTokenDeltaMin). The provider is ignored if
// WithTimelineTokenThreshold is also supplied.
func WithTimelineLimitProvider(p TimelineLimitProvider) VerifyManagerOption {
	return func(m *VerificationManager) {
		m.limitProvider = p
	}
}

// WithAICallerConfig is a convenience wrapper equivalent to
// WithTimelineLimitProvider(func() int64 { return cfg.GetTimelineContentSizeLimit() }).
// A nil cfg is a no-op so callers can pass the config directly without
// defensive checks.
func WithAICallerConfig(cfg timelineLimitFromConfig) VerifyManagerOption {
	return func(m *VerificationManager) {
		if cfg == nil {
			return
		}
		m.limitProvider = func() int64 {
			return cfg.GetTimelineContentSizeLimit()
		}
	}
}

// WithTimeline attaches a Timeline instance the manager will inspect when
// computing the token delta since the last verification. If omitted (nil) the
// token delta is always reported as zero, effectively disabling that trigger.
func WithTimeline(timeline *aicommon.Timeline) VerifyManagerOption {
	return func(m *VerificationManager) {
		m.timeline = timeline
	}
}

// WithNowFunc injects a virtual clock, primarily useful for deterministic unit
// tests of the throttle window.
func WithNowFunc(now func() time.Time) VerifyManagerOption {
	return func(m *VerificationManager) {
		if now != nil {
			m.now = now
		}
	}
}

// VerificationManager centralizes all verification policy for a ReAct loop. It
// tracks throttling, action-rounds counters, timeline token growth, explicit
// done/finish markers, and the verify_level hint that may arrive on tool
// parameters. Callers can either ask for a pure decision via ShouldVerify or
// combine decision and execution via AutoVerifyUserSatisfaction /
// ForceVerifyUserSatisfaction.
//
// The manager does not own any prompt logic. It is a thin policy layer that
// delegates the real AI call to the injected VerifyInvoker.
type VerificationManager struct {
	invoker  VerifyInvoker
	timeline *aicommon.Timeline

	throttleWindow time.Duration
	lastVerifyAt   time.Time

	actionRoundsThreshold  int
	actionCountSinceVerify int

	timelineTokenThreshold int
	timelineTokenBaseline  int
	thresholdExplicitlySet bool
	limitProvider          TimelineLimitProvider

	doneFlag bool

	now func() time.Time
	mu  sync.Mutex
}

// NewVerificationManager constructs a VerificationManager with sane defaults.
// The invoker must be non-nil for Auto / Force execution paths; ShouldVerify
// itself will still work without it.
//
// The timeline token delta threshold follows the hybrid strategy documented on
// the default constants: explicit override via WithTimelineTokenThreshold wins;
// otherwise the manager derives the threshold from a TimelineLimitProvider (or
// WithAICallerConfig) at construction time; otherwise it falls back to
// DefaultVerifyTimelineTokenDeltaFallback.
func NewVerificationManager(invoker VerifyInvoker, opts ...VerifyManagerOption) *VerificationManager {
	m := &VerificationManager{
		invoker:               invoker,
		throttleWindow:        DefaultVerifyThrottleWindow,
		actionRoundsThreshold: DefaultVerifyActionRoundsThreshold,
		now:                   time.Now,
	}
	// Apply all options first so the explicit-vs-default decision is made with
	// complete information (WithTimelineTokenThreshold order-insensitive vs.
	// WithTimelineLimitProvider).
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	if !m.thresholdExplicitlySet {
		m.timelineTokenThreshold = deriveTimelineTokenThreshold(m.limitProvider)
	}
	return m
}

// deriveTimelineTokenThreshold implements the hybrid default computation. It
// is a pure function so it can be reused by RefreshTimelineTokenThresholdFromProvider.
func deriveTimelineTokenThreshold(p TimelineLimitProvider) int {
	if p == nil {
		return DefaultVerifyTimelineTokenDeltaFallback
	}
	limit := p()
	if limit <= 0 {
		return DefaultVerifyTimelineTokenDeltaFallback
	}
	derived := int(limit / int64(DefaultVerifyTimelineLimitDivisor))
	if derived < DefaultVerifyTimelineTokenDeltaMin {
		return DefaultVerifyTimelineTokenDeltaMin
	}
	return derived
}

// RefreshTimelineTokenThresholdFromProvider recomputes the timeline token
// delta threshold from the configured TimelineLimitProvider and applies it.
// It is a no-op when the threshold was explicitly set via
// WithTimelineTokenThreshold. Returns the effective threshold after the
// refresh (which equals the current timelineTokenThreshold if no refresh
// happened).
func (m *VerificationManager) RefreshTimelineTokenThresholdFromProvider() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.thresholdExplicitlySet {
		log.Debugf("verification manager: refresh skipped, threshold was explicitly set (value=%d)", m.timelineTokenThreshold)
		return m.timelineTokenThreshold
	}
	newThreshold := deriveTimelineTokenThreshold(m.limitProvider)
	if newThreshold != m.timelineTokenThreshold {
		log.Infof("verification manager: refresh timeline token threshold %d -> %d", m.timelineTokenThreshold, newThreshold)
	}
	m.timelineTokenThreshold = newThreshold
	return m.timelineTokenThreshold
}

// ShouldVerify returns the verification decision for the given level + event
// without mutating counters or performing any AI call. The three returned
// values are:
//
//   - force:        the decision is a *forced* verification (bypasses throttle)
//   - shouldVerify: whether to run any verification at all
//   - reason:       an English human-readable reason for the decision, useful
//     for logging/observability.
//
// The decision order follows the product spec:
//  1. done / finish flag set (or the event itself is done/finish)
//  2. timeline token delta >= threshold
//  3. action rounds since last verify >= threshold
//  4. explicit verify_level == force
//  5. explicit verify_level == none/low -> skip
//  6. throttle window elapsed -> normal verify, otherwise skip
func (m *VerificationManager) ShouldVerify(level VerifyLevel, event VerifyTriggerEvent) (bool, bool, string) {
	// Compute the current token count outside of our mutex so we do not hold
	// two locks across the (potentially heavy) timeline Dump.
	currentTokens := m.currentTimelineTokens()

	m.mu.Lock()
	defer m.mu.Unlock()

	tokenDelta := currentTokens - m.timelineTokenBaseline
	actionCount := m.actionCountSinceVerify
	nowTime := m.nowLocked()
	var elapsed time.Duration
	if m.lastVerifyAt.IsZero() {
		// Never verified: treat elapsed as "infinite" so the throttle window
		// is always considered passed.
		elapsed = 1<<62 - 1
	} else {
		elapsed = nowTime.Sub(m.lastVerifyAt)
	}

	if m.doneFlag || event == EventDone || event == EventFinish {
		reason := fmt.Sprintf("force: done/finish flag (event=%s)", event)
		return true, true, reason
	}
	if m.timelineTokenThreshold > 0 && tokenDelta >= m.timelineTokenThreshold {
		reason := fmt.Sprintf("force: timeline token delta %d >= threshold %d", tokenDelta, m.timelineTokenThreshold)
		return true, true, reason
	}
	if m.actionRoundsThreshold > 0 && actionCount >= m.actionRoundsThreshold {
		reason := fmt.Sprintf("force: action rounds %d >= threshold %d", actionCount, m.actionRoundsThreshold)
		return true, true, reason
	}
	if level == VerifyLevelForce {
		return true, true, "force: verify_level=force"
	}
	if level == VerifyLevelNone || level == VerifyLevelLow {
		reason := fmt.Sprintf("skip: verify_level=%s", level)
		return false, false, reason
	}
	// middle or unset -> throttle window check
	if m.throttleWindow <= 0 || elapsed >= m.throttleWindow {
		reason := fmt.Sprintf("normal: throttle window passed (elapsed=%s, window=%s)", elapsed, m.throttleWindow)
		return false, true, reason
	}
	reason := fmt.Sprintf("skip: within throttle window (elapsed=%s < window=%s)", elapsed, m.throttleWindow)
	return false, false, reason
}

// AutoVerifyUserSatisfaction consults ShouldVerify and, if a verification is
// required, calls the underlying invoker. The third return value `ran`
// indicates whether the invoker was actually called.
//
// On a successful invocation the internal window is reset (last verify
// timestamp refreshed, action counter zeroed, timeline baseline re-snapshot,
// done flag cleared). On an invoker error the window is NOT reset so callers
// can retry.
func (m *VerificationManager) AutoVerifyUserSatisfaction(
	ctx context.Context,
	query string,
	isToolCall bool,
	payload string,
	level VerifyLevel,
	event VerifyTriggerEvent,
) (*aicommon.VerifySatisfactionResult, bool, error) {
	force, shouldVerify, reason := m.ShouldVerify(level, event)
	if !shouldVerify {
		log.Infof("verification manager: skip auto verify (level=%s, event=%s, reason=%s)", level, event, reason)
		return nil, false, nil
	}
	log.Infof("verification manager: run auto verify (force=%v, level=%s, event=%s, reason=%s)", force, level, event, reason)
	if m.invoker == nil {
		return nil, true, fmt.Errorf("verification manager: invoker is nil")
	}
	result, err := m.invoker.VerifyUserSatisfaction(ctx, query, isToolCall, payload)
	if err != nil {
		log.Errorf("verification manager: auto verify invoker failed: %v", err)
		return nil, true, err
	}
	m.ResetWindow()
	return result, true, nil
}

// ForceVerifyUserSatisfaction unconditionally runs verification via the
// invoker, ignoring throttle / level / counters. On success the window is
// reset so downstream policy resumes from a clean baseline. On error the
// window is preserved.
func (m *VerificationManager) ForceVerifyUserSatisfaction(
	ctx context.Context,
	query string,
	isToolCall bool,
	payload string,
) (*aicommon.VerifySatisfactionResult, error) {
	log.Infof("verification manager: force verify requested")
	if m.invoker == nil {
		return nil, fmt.Errorf("verification manager: invoker is nil")
	}
	result, err := m.invoker.VerifyUserSatisfaction(ctx, query, isToolCall, payload)
	if err != nil {
		log.Errorf("verification manager: force verify invoker failed: %v", err)
		return nil, err
	}
	m.ResetWindow()
	return result, nil
}

// ForceAsyncVerifyUserSatisfaction is a placeholder signature reserved for a
// future background-verification capability. For now it returns an
// already-closed channel carrying a single nil result and logs a warning. The
// signature is stable so callers can be wired up ahead of the real
// implementation.
func (m *VerificationManager) ForceAsyncVerifyUserSatisfaction(
	ctx context.Context,
	query string,
	isToolCall bool,
	payload string,
) <-chan *VerifyAsyncResult {
	log.Warnf("verification manager: async verify not implemented yet, returning closed channel")
	ch := make(chan *VerifyAsyncResult, 1)
	close(ch)
	return ch
}

// NotifyAction is a convenience helper that increments the action counter by
// one. The level argument is currently only logged, but is kept in the
// signature so future policy can take it into account without breaking
// callers.
func (m *VerificationManager) NotifyAction(level VerifyLevel) int {
	newCount := m.DeltaActionOffset(1)
	log.Debugf("verification manager: notify action (level=%s, count=%d)", level, newCount)
	return newCount
}

// GetActionOffset returns the current action count since the last verify.
func (m *VerificationManager) GetActionOffset() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.actionCountSinceVerify
}

// SetActionOffset replaces the current action count.
func (m *VerificationManager) SetActionOffset(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionCountSinceVerify = n
}

// DeltaActionOffset adjusts the action counter by delta (can be negative) and
// returns the new value.
func (m *VerificationManager) DeltaActionOffset(delta int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionCountSinceVerify += delta
	if m.actionCountSinceVerify < 0 {
		m.actionCountSinceVerify = 0
	}
	return m.actionCountSinceVerify
}

// GetTimelineTokenOffset returns the token delta between the current timeline
// dump and the stored baseline. Returns 0 when no timeline is attached.
func (m *VerificationManager) GetTimelineTokenOffset() int {
	currentTokens := m.currentTimelineTokens()
	m.mu.Lock()
	defer m.mu.Unlock()
	return currentTokens - m.timelineTokenBaseline
}

// SetTimelineTokenBaseline overrides the baseline used for delta calculation.
func (m *VerificationManager) SetTimelineTokenBaseline(baseline int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timelineTokenBaseline = baseline
}

// ResetTimelineTokenBaseline snapshots the current timeline token count as the
// new baseline. Useful right after an out-of-band verification.
func (m *VerificationManager) ResetTimelineTokenBaseline() {
	currentTokens := m.currentTimelineTokens()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timelineTokenBaseline = currentTokens
}

// GetLastVerifyAt returns the timestamp of the last successful verification.
// A zero Time is returned if no verification has ever run.
func (m *VerificationManager) GetLastVerifyAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastVerifyAt
}

// GetDoneFlag reports whether an explicit done/finish has been requested and
// not yet consumed by a verification.
func (m *VerificationManager) GetDoneFlag() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doneFlag
}

// MarkDone signals that the AI has emitted an @done directive. The next
// verification decision will treat this as a forced verification.
func (m *VerificationManager) MarkDone() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doneFlag = true
	log.Infof("verification manager: done flag marked")
}

// MarkFinish signals that the AI has emitted an @finish directive. Semantics
// match MarkDone; two methods are kept to make call sites self-documenting.
func (m *VerificationManager) MarkFinish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doneFlag = true
	log.Infof("verification manager: finish flag marked")
}

// ResetWindow is the public form of the post-verification reset. Callers that
// execute a verification out-of-band (bypassing the manager entirely) can call
// this to sync the policy state.
func (m *VerificationManager) ResetWindow() {
	currentTokens := m.currentTimelineTokens()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastVerifyAt = m.nowLocked()
	m.actionCountSinceVerify = 0
	m.timelineTokenBaseline = currentTokens
	m.doneFlag = false
	log.Debugf("verification manager: window reset (baseline_tokens=%d)", currentTokens)
}

// GetThrottleWindow / GetActionRoundsThreshold / GetTimelineTokenThreshold
// expose configuration, mainly for introspection/logging purposes.
func (m *VerificationManager) GetThrottleWindow() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.throttleWindow
}

func (m *VerificationManager) GetActionRoundsThreshold() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.actionRoundsThreshold
}

func (m *VerificationManager) GetTimelineTokenThreshold() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timelineTokenThreshold
}

// currentTimelineTokens performs the (potentially heavy) Timeline.Dump +
// CalcTokenCount outside of the manager mutex. Safe to call with nil timeline.
func (m *VerificationManager) currentTimelineTokens() int {
	m.mu.Lock()
	tl := m.timeline
	m.mu.Unlock()
	if tl == nil {
		return 0
	}
	return ytoken.CalcTokenCount(tl.Dump())
}

// nowLocked must be called with m.mu held. It exists purely to make unit
// testing with a virtual clock easier.
func (m *VerificationManager) nowLocked() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return m.now()
}
