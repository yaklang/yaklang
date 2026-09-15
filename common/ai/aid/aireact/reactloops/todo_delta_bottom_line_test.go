package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestApplyTodoDeltaBottomLineForNormalToolAction(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("tool-task", "inspect token behavior", context.Background(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)

	action, err := aicommon.ExtractAction(`{
		"@action":"write_file",
		"todo_delta":{
			"add":[{"id":"todo-1","text":"verify token reuse"}],
			"current":"todo-1"
		}
	}`, "write_file")
	require.NoError(t, err)

	applyTodoDeltaBottomLine(loop, task, 3, action)
	open, current, closed := cfg.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Equal(t, "todo-1", current)
	require.Len(t, open, 1)
	require.Equal(t, "todo-1", open[0].ID)
	require.Equal(t, "verify token reuse", open[0].Text)
	require.Equal(t, 1, open[0].CreatedAt)
	require.Equal(t, 1, open[0].UpdatedAt)
	require.NotZero(t, open[0].CreatedTs, "CreatedTs should be set on add")
	require.NotZero(t, open[0].FocusStartedTs, "FocusStartedTs should be set when made current")
	require.Empty(t, closed)
}
