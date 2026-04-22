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

func TestMaybeVerifyUserSatisfaction_UsesSharedCheckpointRule(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100})

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
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Len(t, loop.historySatisfactionReasons, 1)
	require.NotNil(t, loop.GetVerificationRuntimeSnapshot())
	require.Equal(t, 100, loop.GetVerificationRuntimeSnapshot().LoopPromptTokens)
}

func TestMaybeVerifyUserSatisfaction_SkipsWhenPromptDeltaIsSmall(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

	loop.currentIterationIndex = 2
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100})
	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)

	loop.currentIterationIndex = 3
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 100 + verificationAutoTriggerMinPromptDelta - 1})
	result, triggered, err = loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.False(t, triggered)
	require.Nil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Equal(t, 2, loop.GetVerificationRuntimeSnapshot().IterationIndex)
	require.Equal(t, 100, loop.GetVerificationRuntimeSnapshot().LoopPromptTokens)
}

func TestMaybeVerifyUserSatisfaction_TriggersWhenPromptDeltaIsLarge(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicVerificationInterval = 2
	loop.perception = newPerceptionController(loop.periodicVerificationInterval)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)
	loop.setVerificationRuntimeSnapshot(&VerificationRuntimeSnapshot{
		GeneratedAt:      time.Now(),
		IterationIndex:   2,
		LoopPromptTokens: 120,
	})
	loop.currentIterationIndex = 4
	loop.SetLastPromptObservation(&PromptObservation{PromptTokens: 120 + verificationAutoTriggerMinPromptDelta})

	result, triggered, err := loop.MaybeVerifyUserSatisfaction(context.Background(), "query", true, "tool")
	require.NoError(t, err)
	require.True(t, triggered)
	require.NotNil(t, result)
	require.Equal(t, 1, invoker.verifyCalls)
	require.Equal(t, 4, loop.GetVerificationRuntimeSnapshot().IterationIndex)
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
