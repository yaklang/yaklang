package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestLoopActionHandoffWaitsForOwnedChildrenAndUnseenResults(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	started, release := make(chan string, 1), make(chan struct{})
	_, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "child"}},
		SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, release, "child evidence")}, "child")
	require.NoError(t, err)
	<-started
	invocations := 0
	factory := ConvertReActLoopFactoryToActionFactory("same-task-focus", func(inv aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
		invocations++
		child := NewMinimalReActLoop(inv.GetConfig(), inv)
		WithInitTask(func(_ *ReActLoop, _ aicommon.AIStatefulTask, op *InitTaskOperator) { op.Done() })(child)
		return child, nil
	})
	action, err := factory(parent.GetInvoker())
	require.NoError(t, err)
	invoke := func() *LoopActionHandlerOperator {
		op := NewActionHandlerOperator(task)
		action.ActionHandler(parent, nil, op)
		return op
	}
	op := invoke()
	require.True(t, op.IsContinued())
	require.Zero(t, invocations, "active child must block before the nested factory has side effects")
	require.NoError(t, task.GetContext().Err())
	close(release)
	jobs := awaitBackgroundTerminal(t, parent.GetSubAgentManager(), nil)
	op = invoke()
	require.True(t, op.IsContinued())
	require.Contains(t, op.GetFeedback().String(), "Read the next iteration")
	require.Zero(t, invocations, "an undelivered child result must also block the same-task handoff")
	require.NoError(t, task.GetContext().Err())
	parent.subAgentModelSeen = jobs[0].ResultRevision
	op = invoke()
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)
	require.Equal(t, 1, invocations, "handoff becomes available once results have been presented")
}
