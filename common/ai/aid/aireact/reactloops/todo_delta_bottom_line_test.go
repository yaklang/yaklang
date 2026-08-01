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
	require.Equal(t, []aicommon.TodoOpenItem{{ID: "todo-1", Text: "verify token reuse", CreatedAt: 1, UpdatedAt: 1}}, open)
	require.Empty(t, closed)
}
