package subagent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildForkTaskID_StableSegment(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("parent-abc", "x", context.Background(), aicommon.NewDummyEmitter(), true)
	id := BuildForkTaskID(task, ForkJob{
		Order:      1,
		Identifier: "sql_injection",
	})
	require.Contains(t, id, "parent-abc-sub-sql_injection-")
}

func TestNormalizeForkConcurrency(t *testing.T) {
	require.Equal(t, 5, normalizeForkConcurrency(0, 8))
	require.Equal(t, 2, normalizeForkConcurrency(0, 2))
	require.Equal(t, 10, normalizeForkConcurrency(99, 20))
}

func TestForkSubTaskCompletionDoesNotCancelJobCtx(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-phase2", "scan", context.Background(), aicommon.NewDummyEmitter(), true)
	jobCtx, jobCancel := context.WithCancel(parent.GetContext())
	defer jobCancel()

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parent,
		"parent-phase2-sub-cmd_injection-test",
		"category scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	)
	require.NotSame(t, jobCtx, subTask.GetContext())

	subTask.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-jobCtx.Done():
		t.Fatal("jobCtx must stay alive when forked sub-task completes; only defer jobCancel should end the worker scope")
	default:
	}
}

func TestNestedSubTaskUsesParentTaskId(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	nested := newNestedSubTask(parent, "fast-context")
	require.NotNil(t, nested)
	require.Equal(t, parent.GetId(), nested.GetId())
	require.Equal(t, parent.GetId(), nested.GetIndex())
	require.NotSame(t, parent.GetContext(), nested.GetContext(), "nested scope must use a derived context")
}

func TestNestedSubTaskContextCancelledOnComplete(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	nested := newNestedSubTask(parent, "fast-context")

	nested.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-parent.GetContext().Done():
		t.Fatal("parent task context must stay alive when nested scope completes")
	default:
	}
	select {
	case <-nested.GetContext().Done():
	default:
		t.Fatal("nested scope context should be cancelled after completion")
	}
}
