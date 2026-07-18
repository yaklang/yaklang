package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func TestFinishPostIteration_ResumesForDelegatedStageSummary(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	loop.SetDirectlyAnswerDelegationAllowed(true)
	loop.onPostIteration = []func(*ReActLoop, int, aicommon.AIStatefulTask, bool, any, *OnPostIterationOperator){
		func(loop *ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *OnPostIterationOperator) {
			if isDone {
				require.True(t, loop.RequestStageSummary("总结现有证据", "host reachable"))
			}
		},
	}

	postOp := loop.finishIterationLoopWithError(1, task, nil)
	require.True(t, postOp.ShouldResumeLoop())
	require.True(t, loop.HasPendingStageSummaryRequest())
}

func TestFinishPostIteration_HardErrorDoesNotDelegate(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	loop.SetDirectlyAnswerDelegationAllowed(true)
	loop.onPostIteration = []func(*ReActLoop, int, aicommon.AIStatefulTask, bool, any, *OnPostIterationOperator){
		func(loop *ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *OnPostIterationOperator) {
			if isDone && loop.IsDirectlyAnswerDelegationAllowed() {
				loop.RequestStageSummary("不应委派", "")
			}
		},
	}

	postOp := loop.finishIterationLoopWithError(1, task, utils.Error("hard failure"))
	require.False(t, postOp.ShouldResumeLoop())
	require.False(t, loop.HasPendingStageSummaryRequest())
}
