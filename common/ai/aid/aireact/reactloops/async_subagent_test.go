package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

func TestSubAgentOwnsIndependentAsyncExecutions(t *testing.T) {
	parent, task := backgroundSubAgentFixture(t, 1)
	parentManager, err := parent.GetAsyncExecutionManager()
	require.NoError(t, err)
	resource := func() asyncexecmanager.Resource {
		r, err := asyncexecmanager.NewResource(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(context.Context) (asyncexecmanager.Observation, error) {
			return asyncexecmanager.Observation{}, nil
		})
		require.NoError(t, err)
		return r
	}
	parentReceipt, err := parentManager.Start(asyncexecmanager.Request{ID: "parent-tool", Resource: resource()})
	require.NoError(t, err)
	type childState struct {
		manager *asyncexecmanager.Manager
		id      string
	}
	ready := make(chan childState, 1)
	builder := managedLoopBuilder(func(p *PreparedSubAgent) (*ReActLoop, error) {
		child := NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		WithInitTask(func(loop *ReActLoop, childTask aicommon.AIStatefulTask, op *InitTaskOperator) {
			manager, err := loop.GetAsyncExecutionManager()
			if err != nil {
				t.Error(err)
				op.Done()
				return
			}
			receipt, err := manager.Start(asyncexecmanager.Request{ID: "child-tool", Resource: resource()})
			if err != nil {
				t.Error(err)
				op.Done()
				return
			}
			ready <- childState{manager: manager, id: receipt.ID}
			<-childTask.GetContext().Done()
			op.Done()
		})(child)
		return child, nil
	})
	receipt, err := parent.SubmitSubAgents(task, []SubAgentJob{{Identifier: "independent-tools"}}, SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: builder}, "async-tool-owner-test")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var child childState
	select {
	case child = <-ready:
	case <-ctx.Done():
		t.Fatal("subagent did not start")
	}
	require.NotSame(t, parentManager, child.manager)
	_, err = parentManager.Get(child.id)
	require.ErrorIs(t, err, asyncexecmanager.ErrNotFound)
	_, err = child.manager.Get(parentReceipt.ID)
	require.ErrorIs(t, err, asyncexecmanager.ErrNotFound)
	// Cancelling the real subagent job cascades into its tool manager only.
	_, err = parent.GetSubAgentManager().Cancel([]string{receipt.Jobs[0].ID})
	require.NoError(t, err)
	jobs := awaitBackgroundTerminal(t, parent.GetSubAgentManager(), []string{receipt.Jobs[0].ID})
	require.Equal(t, "cancelled", jobs[0].State)
	ended, err := child.manager.Wait(ctx, child.id)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, ended.State)
	alive, err := parentManager.Get(parentReceipt.ID)
	require.NoError(t, err)
	require.False(t, alive.State.IsTerminal())
	parent.Release()
	ended, err = parentManager.Wait(ctx, parentReceipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, ended.State)
}
