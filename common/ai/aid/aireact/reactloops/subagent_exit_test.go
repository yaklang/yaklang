package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBackgroundSubAgentCancellationCallbackControlsTerminalStatus(t *testing.T) {
	for _, callbackPanics := range []bool{false, true} {
		name := "callback_sets_skipped"
		if callbackPanics {
			name = "callback_panics"
		}
		t.Run(name, func(t *testing.T) {
			loop, task := backgroundSubAgentFixture(t, 1)
			started := make(chan aicommon.AIStatefulTask, 1)
			builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
				child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
				WithInitTask(func(_ *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
					task.SetAsyncDeferCallback(func(error) {
						if callbackPanics {
							panic("cancel receipt failed")
						}
						task.SetStatus(aicommon.AITaskState_Skipped)
					})
					started <- task
					<-task.GetContext().Done()
					op.Done()
				})(child)
				return child, nil
			})
			_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "cancelled-child"}},
				SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "cancelled-child")
			require.NoError(t, err)
			childTask := <-started
			manager := loop.GetSubAgentManager()
			_, err = manager.Cancel(nil)
			require.NoError(t, err)
			jobs := awaitBackgroundTerminal(t, manager, nil)
			require.Equal(t, "cancelled", jobs[0].State)
			if callbackPanics {
				require.Equal(t, aicommon.AITaskState_Aborted, childTask.GetStatus())
				require.Contains(t, jobs[0].Error, "cancel receipt failed")
			} else {
				require.Equal(t, aicommon.AITaskState_Skipped, childTask.GetStatus())
			}
			released := make(chan struct{})
			close(released)
			receipt, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "next-child"}},
				SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(make(chan string, 1), released, "next result")}, "next-child")
			require.NoError(t, err)
			jobs = awaitBackgroundTerminal(t, manager, []string{receipt.Jobs[0].ID})
			require.Equal(t, "completed", jobs[0].State, "either callback outcome must release the worker slot")
		})
	}
}
