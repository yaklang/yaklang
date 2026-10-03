package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCoordinatorActionFinish(t *testing.T) {
	f := newActionFixture(t, true)
	f.invoke("finish", nil, true)
	require.False(t, f.c.Snapshot().Finished)
	WithPlanningOnly()(f.loop)
	op := f.invoke("finish", nil, false)
	done, err := op.IsTerminated()
	require.NoError(t, err)
	require.True(t, done)
	require.False(t, f.c.Snapshot().Finished, "规划结束不能将未执行任务标成完成")
	require.Equal(t, Pending, f.c.Snapshot().Attempts["a"].State)
	f = newActionFixture(t, true)
	for _, id := range []string{"a", "b"} {
		_, err := f.c.StartTasks([]string{id})
		require.NoError(t, err)
		a := awaitResult(t, f.c, id)
		require.NoError(t, f.c.ReviewTask(id, a.ID, "accept", "actual evidence checked"))
	}
	op = f.invoke("finish", nil, false)
	done, err = op.IsTerminated()
	require.NoError(t, err)
	require.True(t, done)
	require.True(t, f.c.Snapshot().Finished)
}

func TestCoordinatorActionFinishRetainsSharedTodoGate(t *testing.T) {
	f := newActionFixture(t, true)
	WithPlanningOnly()(f.loop)
	scope := aicommon.BuildVerificationTodoScope(f.task)
	f.cfg.ApplyTodoDelta(scope, &aicommon.TodoDelta{Add: []aicommon.TodoAdd{{ID: "check-source", Text: "核对来源"}}})
	f.invoke("finish", nil, true)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "TODO")
	require.Equal(t, Pending, f.c.Snapshot().Attempts["a"].State)
}
func TestCoordinatorActionWorkerFinishRequiresResult(t *testing.T) {
	f := newActionFixture(t, false)
	loop, err := NewWorkerLoop(f.loop.GetInvoker())
	require.NoError(t, err)
	f.loop = loop
	loop.SetCurrentTask(f.task)
	f.invoke("finish", nil, true)
	f.invoke("submit_task_result", map[string]any{"summary": "verified result"}, false)
	h, err := loop.GetActionHandler("finish")
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(f.task)
	h.ActionHandler(loop, actionForTest(t, "finish", nil), op)
	done, err := op.IsTerminated()
	require.NoError(t, err)
	require.True(t, done)
}
