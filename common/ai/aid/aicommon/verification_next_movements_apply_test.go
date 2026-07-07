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

	require.Len(t, timelineEntries, 1)
	require.Contains(t, timelineEntries[0], "[NEXT_MOVEMENTS_ERROR]:")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "FAILED DONE[sibling_todo]:")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "another task scope")

	siblingItems := cfg.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(taskTwo))
	require.Len(t, siblingItems, 1)
	require.Equal(t, VerificationTodoStatusPending, siblingItems[0].Status)
}

func TestBuildDoneMovementsForActiveTodos(t *testing.T) {
	movements := BuildDoneMovementsForActiveTodos([]VerificationTodoItem{
		{ID: "a", Status: VerificationTodoStatusDoing},
		{ID: "  ", Status: VerificationTodoStatusPending},
		{ID: "b", Status: VerificationTodoStatusDone},
	})
	require.Equal(t, []VerifyNextMovement{
		{Op: "done", ID: "a"},
	}, movements)
}

func TestMarkActiveTodosDoneOnAsyncHandoff_ClosesOnlyCurrentTaskActiveTodos(t *testing.T) {
	cfg := NewConfig(context.Background())
	taskOne := NewStatefulTaskBase("task-1", "1-1", nil, nil, true)
	taskTwo := NewStatefulTaskBase("task-2", "1-2", nil, nil, true)

	cfg.ApplyVerificationTodoOps(BuildVerificationTodoScope(taskOne), false, []VerifyNextMovement{
		{Op: "add", ID: "main_todo", Content: "主循环待办"},
		{Op: "add", ID: "main_doing", Content: "进行中"},
		{Op: "doing", ID: "main_doing"},
	})
	cfg.ApplyVerificationTodoOps(BuildVerificationTodoScope(taskTwo), false, []VerifyNextMovement{
		{Op: "add", ID: "sibling_todo", Content: "兄弟任务待办"},
	})

	var timelineEntries []string
	timelineHook := func(category, line string) {
		timelineEntries = append(timelineEntries, category+": "+line)
	}

	MarkActiveTodosDoneOnAsyncHandoff(cfg, nil, taskOne, 2, timelineHook)

	scopeOne := BuildVerificationTodoScope(taskOne)
	itemsOne := cfg.SnapshotVerificationTodoItemsByScope(scopeOne)
	require.Len(t, itemsOne, 2)
	for _, item := range itemsOne {
		require.Equal(t, VerificationTodoStatusDone, item.Status)
	}

	siblingItems := cfg.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(taskTwo))
	require.Len(t, siblingItems, 1)
	require.Equal(t, VerificationTodoStatusPending, siblingItems[0].Status)

	require.NotEmpty(t, timelineEntries)
	require.Contains(t, timelineEntries[0], "NEXT_MOVEMENTS:")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "DONE[main_todo]")
	require.Contains(t, strings.Join(timelineEntries, "\n"), "DONE[main_doing]")
}

func TestMarkActiveTodosDoneOnAsyncHandoff_NoOpWhenNoActiveTodos(t *testing.T) {
	cfg := NewConfig(context.Background())
	task := NewStatefulTaskBase("task-1", "1-1", nil, nil, true)

	cfg.ApplyVerificationTodoOps(BuildVerificationTodoScope(task), false, []VerifyNextMovement{
		{Op: "add", ID: "closed", Content: "已完成"},
		{Op: "done", ID: "closed"},
	})

	called := false
	timelineHook := func(category, line string) {
		called = true
	}

	MarkActiveTodosDoneOnAsyncHandoff(cfg, nil, task, 1, timelineHook)
	require.False(t, called)
}
