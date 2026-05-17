package reactloops

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// timelineCapturingInvoker 把 AddToTimeline 调用全部记下来供断言.
// 关键词: timelineCapturingInvoker, 测试用 timeline 捕获 invoker
type timelineCapturingInvoker struct {
	*mockcfg.MockInvoker

	mu      sync.Mutex
	entries []timelineEntry

	verifyEntry  chan struct{}
	verifyRelease chan struct{}
	verifyCalls  atomic.Int64
	customResult *aicommon.VerifySatisfactionResult
}

type timelineEntry struct {
	Tag     string
	Content string
}

func newTimelineCapturingInvoker(ctx context.Context) *timelineCapturingInvoker {
	return &timelineCapturingInvoker{
		MockInvoker:   mockcfg.NewMockInvoker(ctx),
		verifyEntry:   make(chan struct{}, 8),
		verifyRelease: make(chan struct{}),
	}
}

func (i *timelineCapturingInvoker) AddToTimeline(tag, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.entries = append(i.entries, timelineEntry{Tag: tag, Content: content})
}

func (i *timelineCapturingInvoker) Entries() []timelineEntry {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]timelineEntry, len(i.entries))
	copy(out, i.entries)
	return out
}

func (i *timelineCapturingInvoker) hasTag(tag string) bool {
	for _, e := range i.Entries() {
		if e.Tag == tag {
			return true
		}
	}
	return false
}

// VerifyUserSatisfaction 默认会阻塞直到测试主动 release, 用于模拟"AI 流卡死"
// 场景下 verification 长时间不返回的状态.
func (i *timelineCapturingInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	i.verifyCalls.Add(1)
	select {
	case i.verifyEntry <- struct{}{}:
	default:
	}
	select {
	case <-i.verifyRelease:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if i.customResult != nil {
		return i.customResult, nil
	}
	return aicommon.NewVerifySatisfactionResult(false, "released", ""), nil
}

// TestTriggerVerificationWatchdog_BusyShortCircuit 验证: 当 verificationInFlight
// 为 true 时, watchdog 应立刻短路返回, 写入 [ASYNC_VERIFICATION_WATCHDOG_BUSY]
// timeline, 而不去抢同一把锁导致跟着一起阻塞.
//
// 关键词: triggerVerificationWatchdog 解锁, [ASYNC_VERIFICATION_WATCHDOG_BUSY]
func TestTriggerVerificationWatchdog_BusyShortCircuit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-busy-watchdog", "trigger watchdog while in-flight", ctx, nil, true)

	// 主动置位 inFlight, 模拟 "verification 正在飞行".
	loop.verificationInFlight.Store(true)

	start := time.Now()
	loop.triggerVerificationWatchdog(task)
	elapsed := time.Since(start)
	require.Less(t, elapsed, 200*time.Millisecond, "watchdog must short-circuit instantly")
	require.True(t, invoker.hasTag("[ASYNC_VERIFICATION_WATCHDOG_BUSY]"), "expected busy timeline breadcrumb")
	require.Equal(t, int64(0), invoker.verifyCalls.Load(), "VerifyUserSatisfaction must not be invoked while busy")
}

// TestMaybeVerifyUserSatisfaction_BusyShortCircuit 验证: 当 verificationInFlight
// 已经被另一个调用持有时, MaybeVerifyUserSatisfaction 应直接放行 (返回 false)
// 而不阻塞.
// 关键词: MaybeVerifyUserSatisfaction CAS 让位
func TestMaybeVerifyUserSatisfaction_BusyShortCircuit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 1
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)

	loop.verificationInFlight.Store(true)

	start := time.Now()
	res, ran, err := loop.MaybeVerifyUserSatisfaction(ctx, "query", false, "payload")
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Nil(t, res)
	require.False(t, ran)
	require.Less(t, elapsed, 200*time.Millisecond)
	require.Equal(t, int64(0), invoker.verifyCalls.Load())
}

// TestVerifyUserSatisfactionNow_BusyReentrySkip 验证: 当 inFlight 时,
// VerifyUserSatisfactionNow 也走 reentry 让位路径并写一条 timeline.
// 关键词: VerifyUserSatisfactionNow reentry skip
func TestVerifyUserSatisfactionNow_BusyReentrySkip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)

	loop.verificationInFlight.Store(true)
	res, err := loop.VerifyUserSatisfactionNow(ctx, "query", false, "payload")
	require.NoError(t, err)
	require.Nil(t, res)
	require.True(t, invoker.hasTag("[VERIFICATION_REENTRY_SKIP]"))
	require.Equal(t, int64(0), invoker.verifyCalls.Load())
}

// TestVerifyUserSatisfaction_InFlightDoesNotBlockWatchdog 端到端验证: 让 invoker
// 的 VerifyUserSatisfaction 持续阻塞, 主线程同时发起 watchdog 触发. 修复前
// watchdog 会跟着 verification 卡死 (因为它要抢 verificationMutex); 修复后
// watchdog 立刻短路 + 写 busy timeline.
//
// 关键词: 端到端 watchdog 不被阻塞, post-action 卡死不传染
func TestVerifyUserSatisfaction_InFlightDoesNotBlockWatchdog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stuck-verify", "stuck verify, watchdog must still progress", ctx, nil, true)

	verifyDone := make(chan struct{})
	go func() {
		defer close(verifyDone)
		_, _ = loop.VerifyUserSatisfactionNow(ctx, "query", false, "payload")
	}()

	// 等 verifier 真正进入阻塞段, 此时 verificationInFlight 应为 true.
	select {
	case <-invoker.verifyEntry:
	case <-time.After(3 * time.Second):
		t.Fatalf("VerifyUserSatisfaction did not enter blocking state in time")
	}
	require.True(t, loop.verificationInFlight.Load(), "in-flight flag must be set during AI call")

	// watchdog 必须立刻短路.
	start := time.Now()
	loop.triggerVerificationWatchdog(task)
	elapsed := time.Since(start)
	require.Less(t, elapsed, 200*time.Millisecond, "watchdog blocked by in-flight verification")

	busy := false
	for _, e := range invoker.Entries() {
		if e.Tag == "[ASYNC_VERIFICATION_WATCHDOG_BUSY]" && strings.Contains(e.Content, "still in flight") {
			busy = true
			break
		}
	}
	require.True(t, busy, "missing [ASYNC_VERIFICATION_WATCHDOG_BUSY] timeline")

	// 释放 verifier, 收尾 goroutine.
	close(invoker.verifyRelease)
	select {
	case <-verifyDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("verifier did not unwind after release")
	}
}
