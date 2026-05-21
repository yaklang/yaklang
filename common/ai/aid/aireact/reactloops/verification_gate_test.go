package reactloops

import (
	"context"
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
}

func (i *verificationGateTestInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	i.verifyCalls++
	if i.result != nil {
		return i.result, nil
	}
	return aicommon.NewVerifySatisfactionResult(false, "keep iterating", ""), nil
}

func TestShouldTriggerPeriodicCheckpointOnIteration_UsesLoopInterval(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 4
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)

	require.False(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(2))
	require.True(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(4))
}

func TestShouldTriggerPeriodicCheckpointOnIteration_FallbackWithoutPerception(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = nil
	loop.maxIterations = 5

	require.False(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(1))
	require.True(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(2))
	require.True(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(5))
}

// TestMaybeVerifyUserSatisfaction_UsesFirstFireThreshold 验证 baseline 未建立时
// 走 verificationFirstFireIterationThreshold (=3) 路径: iter < 3 不触发,
// iter >= 3 立即 fire. 旧版用 periodic interval 控制首次, 改造后由首次提前门
// 独立控制, 让 AI 能在 iter=3 就拿到一次反馈建立 baseline.
// 关键词: TestMaybeVerifyUserSatisfaction_UsesFirstFireThreshold, 首次提前门, baseline 早期建立
func TestMaybeVerifyUserSatisfaction_UsesFirstFireThreshold(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100})

	// iter=1 / iter=2 都 < firstFireThreshold (3), 不触发
	loop.currentIterationIndex = 1
	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 0, invoker.verifyCalls)
	require.Len(t, loop.historySatisfactionReasons, 0)

	loop.currentIterationIndex = 2
	result, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 0, invoker.verifyCalls)

	// iter=3 达到 firstFireThreshold, 立即 fire
	loop.currentIterationIndex = 3
	result, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Len(t, loop.historySatisfactionReasons, 1)
	require.NotNil(t, loop.GetVerificationRuntimeSnapshot())
	require.Equal(t, 100, loop.GetVerificationRuntimeSnapshot().LoopPromptTokens)
	require.Equal(t, 3, loop.GetVerificationRuntimeSnapshot().IterationIndex)
}

// TestMaybeVerifyUserSatisfaction_SkipsWhenPromptDeltaIsSmall 验证软 token 门
// 不会被小于阈值的 token 增量打断: 首次 fire 后, iter 差 1 + token 差 1499
// (< 1500 软门) 不触发. 注意冷静期 (=3) 也独立把这种 iter 差 < 3 的尝试
// 全部抑制掉, 所以这里同时验证了"小 delta 软门不通过"和"冷静期内不通过".
// 关键词: TestMaybeVerifyUserSatisfaction_SkipsWhenPromptDeltaIsSmall, 软门小 delta, 冷静期
func TestMaybeVerifyUserSatisfaction_SkipsWhenPromptDeltaIsSmall(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	// iter=3 命中首次提前门, 写入 baseline (iter=3, tokens=100)
	loop.currentIterationIndex = 3
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100})
	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)

	// iter=4, iter delta=1, 处于冷静期 (3) 内; 即便 token delta=1499
	// 也已经被 cooldown 提前 short-circuit, 不会触发软门
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100 + verificationAutoTriggerMinPromptDelta - 1})
	result, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Equal(t, 3, loop.GetVerificationRuntimeSnapshot().IterationIndex)
	require.Equal(t, 100, loop.GetVerificationRuntimeSnapshot().LoopPromptTokens)
}

// TestMaybeVerifyUserSatisfaction_TriggersWhenPromptDeltaIsLarge 验证软 token 门
// 在通过冷静期之后能正常触发: baseline iter=2, currentIter=5, iter delta=3
// (== cooldown), token delta >= 1500 → 触发. 通过把 periodic interval 设
// 为远大于 iter delta 的值, 排除 iter 门干扰, 专门测软 token 门.
// 关键词: TestMaybeVerifyUserSatisfaction_TriggersWhenPromptDeltaIsLarge, 软门冷静期后触发
func TestMaybeVerifyUserSatisfaction_TriggersWhenPromptDeltaIsLarge(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	// 故意把 periodic 设大, 避免 iter 门提前触发污染测试目标
	loop.periodicVerificationInterval = 100
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   2,
		LoopPromptTokens: 120,
	})
	// iter delta = 5-2 = 3, 等于 cooldown, 软门解禁
	loop.currentIterationIndex = 5
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120 + verificationAutoTriggerMinPromptDelta})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Equal(t, 5, loop.GetVerificationRuntimeSnapshot().IterationIndex)
}

func TestMaybeVerifyUserSatisfaction_TriggersWhenIterationDeltaIsLarge(t *testing.T) {
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
	// iter delta = 5-2 = 3, 命中 iter 门 (>= periodic=3), 即便 token delta=0
	loop.currentIterationIndex = 5
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Equal(t, 5, loop.GetVerificationRuntimeSnapshot().IterationIndex)
}

func TestMaybeVerifyUserSatisfaction_TriggersWhenSnapshotIsStale(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now().Add(-verificationAutoTriggerMaxSnapshotAge - time.Second),
		IterationIndex:   2,
		LoopPromptTokens: 120,
	})
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
}

// TestShouldTriggerAutomaticVerification_FirstFireEarlierWithThreshold 在
// shouldTriggerAutomaticVerification 函数层面验证首次提前门: previous == nil
// 时, 当前 iter 必须 >= verificationFirstFireIterationThreshold (3) 才 fire,
// iter < 3 全部 skip. 与上面通过 MaybeVerifyUserSatisfaction 的端到端测试
// 对偶, 排除 MockInvoker / 节流锁等中间状态干扰.
// 关键词: TestShouldTriggerAutomaticVerification_FirstFireEarlierWithThreshold,
//
//	首次提前门函数级测试
func TestShouldTriggerAutomaticVerification_FirstFireEarlierWithThreshold(t *testing.T) {
	invoker := mockcfg.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100 // 把 iter 门设大, 避免污染首次门测试

	for iter := 1; iter < verificationFirstFireIterationThreshold; iter++ {
		snapshot := &VerificationRuntimeSnapshot{
			GeneratedAt:    time.Now(),
			IterationIndex: iter,
		}
		require.False(t, loop.shouldTriggerAutomaticVerification(snapshot),
			"iter=%d 小于 firstFireThreshold, 不应触发", iter)
	}

	// iter >= threshold (3) 应该触发
	snapshot := &VerificationRuntimeSnapshot{
		GeneratedAt:    time.Now(),
		IterationIndex: verificationFirstFireIterationThreshold,
	}
	require.True(t, loop.shouldTriggerAutomaticVerification(snapshot),
		"iter=%d 达到 firstFireThreshold, 应触发", verificationFirstFireIterationThreshold)
}

// TestMaybeVerifyUserSatisfaction_TokenGateSuppressedDuringCooldown 验证数据爆炸
// 场景下的核心修复: fire 后 iter 差 1/2 (< cooldown=3) 时, 即使 token delta
// 远超软门阈值, 也不触发软门, 这是修复 "数据爆炸阶段每个工具调用都 verify"
// 尖峰问题的关键. iter 差到 cooldown 之后才允许软门生效.
// 关键词: TestMaybeVerifyUserSatisfaction_TokenGateSuppressedDuringCooldown,
//
//	冷静期抑制软门, 数据爆炸修复
func TestMaybeVerifyUserSatisfaction_TokenGateSuppressedDuringCooldown(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100 // 避免 iter 门干扰
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	baselineTokens := 100
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   3,
		LoopPromptTokens: baselineTokens,
	})

	// iter delta = 1 (< cooldown 3), token delta 远超软门 1500 但低于硬门 5000
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: baselineTokens + 3000})
	_, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered, "iter delta=1 处于冷静期内, 软门必须被抑制")
	require.Equal(t, 0, invoker.verifyCalls)

	// iter delta = 2 (< cooldown 3), token delta 仍然抑制
	loop.currentIterationIndex = 5
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: baselineTokens + 4500})
	_, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered, "iter delta=2 仍在冷静期内, 软门必须被抑制")
	require.Equal(t, 0, invoker.verifyCalls)

	// iter delta = 3 (== cooldown), 冷静期结束, 软门解禁, token delta=1500 即可触发
	loop.currentIterationIndex = 6
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: baselineTokens + verificationAutoTriggerMinPromptDelta})
	_, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered, "iter delta=3 已到冷静期边界, 软门应正常触发")
	require.Equal(t, 1, invoker.verifyCalls)
}

// TestMaybeVerifyUserSatisfaction_HardTokenGateBypassesCooldown 验证硬 token 门的
// 豁免逻辑: 当单次 prompt token 增量 >= verificationAutoTriggerHardPromptDelta
// (5000) 时, 即便仍处于冷静期内 (iter delta < 3) 也立即 fire. 这是为了不丢
// 单次超大数据爆炸的关键 verify 时机.
// 关键词: TestMaybeVerifyUserSatisfaction_HardTokenGateBypassesCooldown,
//
//	硬门豁免冷静期, 单次爆炸不丢
func TestMaybeVerifyUserSatisfaction_HardTokenGateBypassesCooldown(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 100 // 避免 iter 门干扰
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	baselineTokens := 200
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   3,
		LoopPromptTokens: baselineTokens,
	})

	// iter delta = 1 (深陷冷静期), 但 token delta >= 硬门, 应该立即 fire
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{
		PromptTokens: baselineTokens + verificationAutoTriggerHardPromptDelta,
	})
	_, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered, "硬门必须豁免冷静期, 即使 iter delta=1 也应触发")
	require.Equal(t, 1, invoker.verifyCalls)
}

// TestMaybeVerifyUserSatisfaction_IterGateStillFiresAfterCooldown 验证基础节拍门
// 仍然有效: 即便整个过程 token 完全没变化, 只要 iter 差累积到
// periodicVerificationInterval (6), 也必须触发 iter 兜底 verify, 这是
// loop 长期无 token 增长场景 (例如 AI 一直 directly_answer 没真正 fire 工具)
// 下的最后保险.
// 关键词: TestMaybeVerifyUserSatisfaction_IterGateStillFiresAfterCooldown,
//
//	iter 基础节拍兜底, 无 token 增长场景
func TestMaybeVerifyUserSatisfaction_IterGateStillFiresAfterCooldown(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = verificationIterationTriggerInterval // 默认 6
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	baselineTokens := 300
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   2,
		LoopPromptTokens: baselineTokens,
	})

	// iter delta = 6 (== periodic), token 完全无变化, 应该走 iter 兜底门
	loop.currentIterationIndex = 8
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: baselineTokens})
	_, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered, "iter 差达到 periodic 时即使 token 无增长也必须触发")
	require.Equal(t, 1, invoker.verifyCalls)
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
	loop.currentIterationIndex = 3
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120 + verificationAutoTriggerMinPromptDelta - 1})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 0, invoker.verifyCalls)
}

func TestVerificationWatchdog_TriggersAfterIdle(t *testing.T) {
	previousTimeout := verificationWatchdogIdleTimeout
	verificationWatchdogIdleTimeout = 20 * time.Millisecond
	defer func() {
		verificationWatchdogIdleTimeout = previousTimeout
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

	require.Eventually(t, func() bool {
		return task.IsFinished()
	}, time.Second, 10*time.Millisecond)
	require.GreaterOrEqual(t, invoker.verifyCalls, 1)
	require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
}

func TestVerificationWatchdog_SuppressedDuringToolBlocking(t *testing.T) {
	previousTimeout := verificationWatchdogIdleTimeout
	verificationWatchdogIdleTimeout = 25 * time.Millisecond
	defer func() {
		verificationWatchdogIdleTimeout = previousTimeout
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

	loop.beginVerificationWatchdogToolSuppression()
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, 0, invoker.verifyCalls, "watchdog must not fire while tool blocking suppression is active")
	loop.endVerificationWatchdogToolSuppression()

	require.Eventually(t, func() bool {
		return invoker.verifyCalls >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestVerificationWatchdog_ResetsWhenMaybeVerifyRuns(t *testing.T) {
	previousTimeout := verificationWatchdogIdleTimeout
	verificationWatchdogIdleTimeout = 40 * time.Millisecond
	defer func() {
		verificationWatchdogIdleTimeout = previousTimeout
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
