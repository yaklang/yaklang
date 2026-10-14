package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type asyncReviewTestInvoker struct {
	*mock.MockInvoker
	answers int
}

func (i *asyncReviewTestInvoker) DirectlyAnswer(context.Context, string, []*aitool.Tool, ...any) (string, error) {
	i.answers++
	return "user-requested answer", nil
}

func TestAsyncReviewStopsRemainingActionsAndAnswersOnce(t *testing.T) {
	for _, alreadyAnswering := range []bool{false, true} {
		t.Run(fmt.Sprint(alreadyAnswering), func(t *testing.T) {
			ctx := context.Background()
			cfg := mock.NewMockedAIConfig(ctx)
			invoker := &asyncReviewTestInvoker{MockInvoker: mock.NewMockInvoker(ctx)}
			invoker.SetConfig(cfg)
			loop := NewMinimalReActLoop(cfg, invoker)
			defer loop.Release()
			task := mock.NewMockStatefulTask(ctx, "owner", "review control")
			loop.SetCurrentTask(task)
			sideEffects := 0
			firstAction := "submit"
			if alreadyAnswering {
				firstAction = loopAction_DirectlyAnswer.ActionType
			}
			calls := []LoopCall{
				{Action: aicommon.NewSimpleAction(firstAction, nil), LoopAction: &LoopAction{ActionType: firstAction, ActionHandler: func(loop *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
					if alreadyAnswering {
						_, err := invoker.DirectlyAnswer(ctx, "foreground answer", nil)
						require.NoError(t, err)
					}
					loop.RequestAsyncDirectAnswer()
					loop.RequestAsyncDirectAnswer()
					op.Continue()
				}}},
				{Action: aicommon.NewSimpleAction("forbidden", nil), LoopAction: &LoopAction{ActionType: "forbidden", ActionHandler: func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) { sideEffects++; op.Continue() }}},
			}
			result := loop.execCalls(calls, 1, task, "", new(utils.Once), nil)
			require.NoError(t, result.err)
			require.Equal(t, loopActionsExit, result.result)
			require.Equal(t, 1, invoker.answers)
			require.Zero(t, sideEffects)
			_, err := loop.GetAsyncExecutionManager()
			require.Error(t, err, "review stop must close later async admission even before manager creation")
		})
	}
}
