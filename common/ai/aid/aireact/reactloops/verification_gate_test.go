package reactloops

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type verificationGateTestInvoker struct {
	*mockcfg.MockInvoker
	verifyCalls int
	result      *aicommon.VerifySatisfactionResult
	// verifyDelay 用于在测试里模拟 AI 调用耗时. 当 > 0 时, VerifyUserSatisfaction
	// 会先 sleep 这段时间再返回, 让 fire 开始时间与 fire 结束时间能被测试
	// 显式区分, 用于验证 "fire 完成后用真实结束时刻作为新基线" 的清零语义.
	// 关键词: verifyDelay, fire 开始/结束时间差, 基线时刻验证
	verifyDelay time.Duration
	// timelineMu / timelineEntries 记录 AddToTimeline 调用, 供 watchdog
	// nudge 断言使用 (MockInvoker.AddToTimeline 是空实现).
	timelineMu      sync.Mutex
	timelineEntries []string
}

func (i *verificationGateTestInvoker) AddToTimeline(entry, content string) {
	i.timelineMu.Lock()
	i.timelineEntries = append(i.timelineEntries, entry)
	i.timelineMu.Unlock()
	i.MockInvoker.AddToTimeline(entry, content)
}

func (i *verificationGateTestInvoker) hasTimelineEntry(entry string) bool {
	i.timelineMu.Lock()
	defer i.timelineMu.Unlock()
	for _, e := range i.timelineEntries {
		if e == entry {
			return true
		}
	}
	return false
}

func (i *verificationGateTestInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	i.verifyCalls++
	if i.verifyDelay > 0 {
		time.Sleep(i.verifyDelay)
	}
	if i.result != nil {
		return i.result, nil
	}
	return aicommon.NewVerifySatisfactionResult(false, "keep iterating", ""), nil
}

// TestVerificationWatchdog_TriggersWhenSnapshotIsStale 验证看门狗承担时间门:
// baseline 比时间门阈值早 1s, 看门狗 timer 计算剩余时间为 0 → 兜底间隔,
// 应很快触发 verification. 自动门 (token 门) 此时 token delta=0 不触发,
// 确认时间门职责已迁移到 watchdog.
func TestVerificationWatchdog_TriggersWhenSnapshotIsStale(t *testing.T) {
	previousMin := verificationWatchdogMinInterval
	verificationWatchdogMinInterval = 20 * time.Millisecond
	defer func() { verificationWatchdogMinInterval = previousMin }()

	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		result:      aicommon.NewVerifySatisfactionResult(false, "stale", ""),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100 // 非 0: 启用节流, 不再每次 fire
	task := aicommon.NewStatefulTaskBase("watchdog-stale-task", "query", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	// baseline 比时间门阈值早 1s: 剩余时间 ≈ 0 → 兜底间隔
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now().Add(-verificationAutoTriggerMaxSnapshotAge - time.Second),
		IterationIndex:   2,
		LoopPromptTokens: 120,
	})
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120})
	// 自动门: token delta=0, 不触发
	_, triggered, _ := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.False(t, triggered, "自动 token 门不应触发 (token delta=0)")

	loop.startVerificationWatchdog(task)
	defer loop.stopVerificationWatchdogForTask(task)
	require.Eventually(t, func() bool { return invoker.verifyCalls >= 1 }, time.Second, 10*time.Millisecond)
}

func TestMaybeVerifyUserSatisfaction_SkipsOnlyWhenAllSignalsAreWeak(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 3
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   2,
		LoopPromptTokens: 120,
	})
	// token delta 远低于硬门 (5120) 与软门 (10240), 时间未过; iter delta=1
	// 不触发 (旧版 iter 门已删除, 现仅靠时间门 + token 门节流).
	loop.currentIterationIndex = 3
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120 + 100})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 0, invoker.verifyCalls)
}

func TestVerificationWatchdog_TriggersAfterIdle(t *testing.T) {
	previousMin := verificationWatchdogMinInterval
	verificationWatchdogMinInterval = 20 * time.Millisecond
	defer func() {
		verificationWatchdogMinInterval = previousMin
	}()

	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		result:      aicommon.NewVerifySatisfactionResult(true, "done", ""),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("watchdog-task", "query", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	loop.startVerificationWatchdog(task)
	defer loop.stopVerificationWatchdogForTask(task)

	// verification 收缩为纯观测角色后, watchdog 不再自动 task.Finish (退出职责
	// 完全迁移到 AI 主动 finish + maxIter 软中断). watchdog 满意时改为写一条
	// [VERIFICATION_WATCHDOG_SUGGEST_FINISH] timeline nudge, 推动 AI 自己 finish.
	// 因此这里断言: verification 被调用了, 但 task 未被 watchdog 终结, 且
	// timeline 里出现了 SUGGEST_FINISH nudge.
	require.Eventually(t, func() bool {
		return invoker.verifyCalls >= 1
	}, time.Second, 10*time.Millisecond)
	require.False(t, task.IsFinished(),
		"watchdog must NOT auto-finish the task anymore; exit is delegated to the AI's finish action")
	// nudge 应写入 invoker timeline
	require.Eventually(t, func() bool {
		return invoker.hasTimelineEntry("[VERIFICATION_WATCHDOG_SUGGEST_FINISH]")
	}, time.Second, 10*time.Millisecond,
		"watchdog should emit a SUGGEST_FINISH nudge timeline entry when verification observes satisfied")
}

func TestVerificationWatchdog_SuppressedDuringToolBlocking(t *testing.T) {
	previousMin := verificationWatchdogMinInterval
	verificationWatchdogMinInterval = 25 * time.Millisecond
	defer func() {
		verificationWatchdogMinInterval = previousMin
	}()

	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		result:      aicommon.NewVerifySatisfactionResult(false, "idle", ""),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("watchdog-suppress-task", "query", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	loop.startVerificationWatchdog(task)
	defer loop.stopVerificationWatchdogForTask(task)

	loop.BeginVerificationWatchdogToolSuppression()
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, 0, invoker.verifyCalls, "watchdog must not fire while tool blocking suppression is active")
	loop.EndVerificationWatchdogToolSuppression()

	require.Eventually(t, func() bool {
		return invoker.verifyCalls >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestVerificationWatchdog_ResetsWhenMaybeVerifyRuns(t *testing.T) {
	previousMin := verificationWatchdogMinInterval
	verificationWatchdogMinInterval = 40 * time.Millisecond
	defer func() {
		verificationWatchdogMinInterval = previousMin
	}()

	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100
	task := aicommon.NewStatefulTaskBase("watchdog-reset-task", "query", context.Background(), nil, true)
	loop.SetCurrentTask(task)
	loop.startVerificationWatchdog(task)
	defer loop.stopVerificationWatchdogForTask(task)

	time.Sleep(20 * time.Millisecond)
	_, _, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)

	time.Sleep(25 * time.Millisecond)
	require.Equal(t, 0, invoker.verifyCalls)
}

func TestRefreshVerificationRuntimeSnapshot_ReplacesStoredPointer(t *testing.T) {
	invoker := mockcfg.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.currentIterationIndex = 2
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 96})

	first := loop.RefreshVerificationRuntimeSnapshot()
	require.NotNil(t, first)
	require.Same(t, first, loop.GetVerificationRuntimeSnapshot())
	require.Equal(t, 2, first.IterationIndex)
	require.Equal(t, 96, first.LoopPromptTokens)

	time.Sleep(time.Millisecond)
	loop.currentIterationIndex = 3
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 144})

	second := loop.RefreshVerificationRuntimeSnapshot()
	require.NotNil(t, second)
	require.NotSame(t, first, second)
	require.Same(t, second, loop.GetVerificationRuntimeSnapshot())
	require.True(t, second.GeneratedAt.After(first.GeneratedAt))
	require.Equal(t, 3, second.IterationIndex)
	require.Equal(t, 144, second.LoopPromptTokens)
}

func TestRefreshVerificationRuntimeSnapshot_UsesPromptStatusFallback(t *testing.T) {
	invoker := mockcfg.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.currentIterationIndex = 2
	loop.SetLastPromptObservationStatus(&PromptObservationStatus{PromptTokens: 72})

	snapshot := loop.RefreshVerificationRuntimeSnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, 2, snapshot.IterationIndex)
	require.Equal(t, 72, snapshot.LoopPromptTokens)
}

// TestMaybeVerifyUserSatisfaction_BaselineRebuildAfterFire 验证自动路径在 fire
// 完成后, prev snapshot 使用的是 fire **结束时刻** 的真实 snapshot, 而不是
// fire 开始前计算的 currentSnapshot. 这是多门交叉触发场景下"清零公平"
// 的核心修复: AI 调用耗时不应被白送给时间门, 否则下一轮时间门会比期望
// 提前到位.
//
// 测试做法: 模拟 AI 调用延迟 60ms, 记录 fire 开始时间; fire 完成后取
// snapshot.GeneratedAt, 验证它晚于 fire 开始时间至少 50ms (允许 10ms
// 调度抖动), 说明基线时刻是 fire 结束时刻而不是 fire 开始时刻.
//
// 关键词: TestMaybeVerifyUserSatisfaction_BaselineRebuildAfterFire,
//
//	fire 完成后基线时刻验证, AI 调用耗时不白送时间门,
//	自动路径与显式路径一致, 多门交叉触发清零公平
func TestMaybeVerifyUserSatisfaction_BaselineRebuildAfterFire(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		verifyDelay: 60 * time.Millisecond,
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100 // 避免 iter 门干扰, 用硬 token 门 fire
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	// baseline 建立在远早于 fire 的时间点, 让我们关注 fire 完成后 prev 的
	// GeneratedAt 是否被刷新到 fire 结束时刻
	baselineTokens := 200
	baselineCreatedAt := time.Now().Add(-30 * time.Second)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      baselineCreatedAt,
		IterationIndex:   3,
		LoopPromptTokens: baselineTokens,
	})
	// 用硬 token 门豁免冷静期触发 fire, iter delta=1 仍然 < 3 冷静期
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{
		PromptTokens: baselineTokens + verificationAutoTriggerHardPromptDelta,
	})

	beforeFire := time.Now()
	_, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	afterFire := time.Now()
	require.NoError(t, err)
	require.True(t, triggered, "硬 token 门应触发 fire")
	require.Equal(t, 1, invoker.verifyCalls)

	snapshot := loop.GetVerificationRuntimeSnapshot()
	require.NotNil(t, snapshot, "fire 完成后 prev snapshot 必须存在")
	require.Equal(t, 4, snapshot.IterationIndex, "prev.iter 应为 fire 时刻的 currentIterationIndex")
	require.Equal(t, baselineTokens+verificationAutoTriggerHardPromptDelta, snapshot.LoopPromptTokens,
		"prev.tokens 应为 fire 时刻的当前 tokens")

	// 核心断言: GeneratedAt 必须晚于 fire 开始时间至少 50ms (留 10ms 抖动余量),
	// 说明它反映的是 fire 结束时刻, 而不是 fire 开始时刻.
	elapsed := snapshot.GeneratedAt.Sub(beforeFire)
	require.GreaterOrEqual(t, elapsed, 50*time.Millisecond,
		"prev.GeneratedAt 必须反映 fire 结束时刻, 与 fire 开始时刻差 >= AI 调用耗时 (60ms - 10ms 抖动)")
	require.LessOrEqual(t, snapshot.GeneratedAt.Sub(afterFire), 5*time.Millisecond,
		"prev.GeneratedAt 与 fire 实际结束时刻应非常接近")
	// 同时显式验证 GeneratedAt 不再是 baseline 旧时刻 (旧 baseline 是 30s 前)
	require.True(t, snapshot.GeneratedAt.After(baselineCreatedAt.Add(time.Second)),
		"prev.GeneratedAt 必须远晚于旧 baseline, 而不是停留在 baseline 时刻")
}

// TestMaybeVerifyUserSatisfaction_TimeGateRefreshAfterFire 验证: 不论哪条
// 路径触发 fire, fire 完成后 prev.GeneratedAt 都必须被推进到 fire 结束时刻.
// 时间门由看门狗承担, 看门狗的 nextVerificationWatchdogDelay 以
// snapshot.GeneratedAt 为唯一起算点, 因此 fire 完成后基线刷新正确与否直接
// 决定看门狗下次触发是否公平. 这里用硬 token 门触发 fire (时间门已不再在
// 自动门中), 验证 snapshot.GeneratedAt 反映 fire 结束时刻而非开始时刻.
//
// 关键词: TimeGateRefreshAfterFire, 看门狗时间门基线, fire 完成时刻,
//
//	AI 调用耗时不白送给看门狗
func TestMaybeVerifyUserSatisfaction_TimeGateRefreshAfterFire(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		verifyDelay: 60 * time.Millisecond,
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	// baseline 建立在远早于 fire 的时间点
	baselineTokens := 200
	baselineCreatedAt := time.Now().Add(-30 * time.Second)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      baselineCreatedAt,
		IterationIndex:   3,
		LoopPromptTokens: baselineTokens,
	})
	// 用硬 token 门触发 fire
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{
		PromptTokens: baselineTokens + verificationAutoTriggerHardPromptDelta,
	})

	beforeFire := time.Now()
	_, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered, "硬 token 门应触发 fire")
	require.Equal(t, 1, invoker.verifyCalls)

	// 核心断言: prev.GeneratedAt 必须反映 fire 结束时刻, 这样看门狗
	// nextVerificationWatchdogDelay 计算剩余时间时起点正确, 不被 AI 调用
	// 耗时白送.
	snapshot := loop.GetVerificationRuntimeSnapshot()
	require.NotNil(t, snapshot)
	require.True(t, snapshot.GeneratedAt.After(beforeFire.Add(50*time.Millisecond)),
		"prev.GeneratedAt 必须晚于 fire 开始时间 50ms 以上, 反映 fire 真实结束时刻")
	require.True(t, snapshot.GeneratedAt.After(baselineCreatedAt.Add(time.Second)),
		"prev.GeneratedAt 必须远晚于旧 baseline, 而不是停留在 baseline 时刻")
}
