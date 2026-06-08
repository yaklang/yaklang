package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyVerificationNextMovementsAndEmit_WritesApplyErrorsToTimeline(t *testing.T) {
	cfg := NewConfig(context.Background())
	taskOne := NewStatefulTaskBase("task-1", "1-1", nil, nil, true)
	taskTwo := NewStatefulTaskBase("task-2", "1-2", nil, nil, true)

	cfg.ApplyVerificationTodoOps(BuildVerificationTodoScope(taskTwo), false, []VerifyNextMovement{
		{Op: "add", ID: "sibling_todo", Content: "兄弟任务 TODO"},
	})

	var timelineEntries []string
	timelineHook := func(category, line string) {
		timelineEntries = append(timelineEntries, category+": "+line)
	}

	ApplyVerificationNextMovementsAndEmit(
		cfg,
		nil,
		taskOne,
		BuildVerificationTodoScope(taskOne),
		3,
		false,
		[]VerifyNextMovement{
			{Op: "done", ID: "sibling_todo"},
		},
		timelineHook,
	)

	require.Len(t, timelineEntries, 2)
	require.Contains(t, timelineEntries[0], "NEXT_MOVEMENTS:")
	require.Contains(t, timelineEntries[1], "[NEXT_MOVEMENTS_ERROR]:")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "FAILED DONE[sibling_todo]:")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "another task scope")

	siblingItems := cfg.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(taskTwo))
	require.Len(t, siblingItems, 1)
	require.Equal(t, VerificationTodoStatusPending, siblingItems[0].Status)
}
