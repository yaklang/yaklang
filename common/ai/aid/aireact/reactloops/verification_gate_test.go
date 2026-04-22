package reactloops

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type verificationGateTestInvoker struct {
	*mockcfg.MockInvoker
	verifyCalls int
}

func (i *verificationGateTestInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	i.verifyCalls++
	return aicommon.NewVerifySatisfactionResult(false, "keep iterating", ""), nil
}

func TestShouldTriggerPeriodicCheckpointOnIteration_UsesLoopInterval(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.periodicCheckpointInterval = 4
	loop.perception = newPerceptionController(loop.periodicCheckpointInterval)

	require.False(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(2))
	require.True(t, loop.ShouldTriggerPeriodicCheckpointOnIteration(4))
}

func TestShouldTriggerPeriodicCheckpointOnIteration_FallbackWithoutPerception(t *testing.T) {
	invoker := &verificationGateTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
	}
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
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
	loop.perception = newPerceptionController(loop.periodicCheckpointInterval)
	loop.actionHistoryMutex = new(sync.Mutex)
	loop.historySatisfactionReasons = make([]*SatisfactionRecord, 0)

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
}
