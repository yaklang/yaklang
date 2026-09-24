package aicommon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func todoSnapshotTestConfig(t *testing.T, sessionID string) *Config {
	t.Helper()
	previous := consts.GetCurrentProjectDatabasePath()
	require.NoError(t, consts.SetGormProjectDatabase(filepath.Join(t.TempDir(), "snapshot-todo.db")))
	t.Cleanup(func() { require.NoError(t, consts.SetGormProjectDatabase(previous)) })
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)
	require.NoError(t, yakit.RegisterAIAgentSession(db, sessionID))
	return NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
}

func todoSnapshotTestTask(id string) AIStatefulTask {
	return NewStatefulTaskBase(id, "input", context.Background(), nil, true)
}

func TestSessionSnapshotTodo_TaskIsolationAndPersistence(t *testing.T) {
	const sessionID = "snapshot-todo-isolation"
	cfg := todoSnapshotTestConfig(t, sessionID)
	one := todoSnapshotTestTask("task-one")
	two := todoSnapshotTestTask("task-two")
	for _, testCase := range []struct {
		task AIStatefulTask
		text string
	}{
		{one, "first task item"}, {two, "second task item"},
	} {
		BeginSessionSnapshotExecutionForTask(cfg, testCase.task, time.Now())
		cfg.MaterializeSessionSnapshot(testCase.task, &SessionSnapshot{Execution: cfg.BuildSessionSnapshotExecution(testCase.task)})
		scope := BuildVerificationTodoScope(testCase.task)
		results := ApplyTodoDeltaAndEmit(cfg, nil, testCase.task, scope, 1,
			&TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: testCase.text}}}, nil)
		require.Len(t, results, 1)
		require.True(t, results[0].Success, results[0].Reason)
		got := cfg.GetSessionTaskSnapshot(testCase.task.GetId())
		require.NotNil(t, got)
		require.Equal(t, testCase.text, got.Todo.Items[0].Content)
		require.Equal(t, testCase.task.GetId(), got.Todo.Items[0].ScopeTaskID)
	}

	raw, err := yakit.GetAISessionMetaSnapshot(cfg.GetDB(), sessionID)
	require.NoError(t, err)
	var doc SessionSnapshotDocument
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	require.Equal(t, SessionSnapshotDocumentSchemaVersion, doc.SchemaVersion)
	require.Len(t, doc.TodoScopes, 2)
	require.Equal(t, 2, doc.Session.Todo.Stats.Pending)
	require.Len(t, doc.Session.Todo.ByTask, 2)
	require.Equal(t, 1, doc.Tasks["task-one"].Todo.Stats.Pending)
	require.Equal(t, 1, doc.Tasks["task-two"].Todo.Stats.Pending)

	restored := NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
	BeginSessionSnapshotExecutionForTask(restored, two, time.Now())
	require.Len(t, restored.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(one)), 1)
	require.Len(t, restored.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(two)), 1)
	result := ApplyTodoDeltaAndEmit(restored, nil, two, BuildVerificationTodoScope(two), 2,
		&TodoDelta{Add: []TodoAdd{{Text: "generated ID after restart"}}}, nil)
	require.Len(t, result, 1)
	require.Equal(t, "todo-1", restored.GetSessionTaskSnapshot(one.GetId()).Todo.Items[0].ID)
	require.Equal(t, "todo-2", result[0].Operation.ID)
}

func TestSessionSnapshotTodo_FrozenAttemptsAndNoOp(t *testing.T) {
	const sessionID = "snapshot-todo-attempts"
	cfg := todoSnapshotTestConfig(t, sessionID)
	task := todoSnapshotTestTask("repeated-task")
	scope := BuildVerificationTodoScope(task)
	BeginSessionSnapshotExecutionForTask(cfg, task, time.Now())
	ApplyTodoDeltaAndEmit(cfg, nil, task, scope, 1,
		&TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "initial"}}}, nil)
	cfg.FinalizeSessionSnapshotExecution("completed", time.Now())
	cfg.MaterializeSessionSnapshot(task, &SessionSnapshot{Execution: cfg.BuildSessionSnapshotExecution(task)})
	require.True(t, cfg.GetSessionTaskSnapshot(task.GetId()).IsFinal)

	BeginSessionSnapshotExecutionForTask(cfg, task, time.Now())
	ApplyTodoDeltaAndEmit(cfg, nil, task, scope, 2,
		&TodoDelta{Add: []TodoAdd{{ID: "todo-2", Text: "retry"}}}, nil)
	retried := cfg.GetSessionTaskSnapshot(task.GetId())
	require.Equal(t, 2, retried.Attempt)
	require.Len(t, retried.Todo.Items, 2)
	require.Len(t, retried.PreviousAttempts, 1)
	require.Len(t, retried.PreviousAttempts[0].Todo.Items, 1)

	rawBefore, err := yakit.GetAISessionMetaSnapshot(cfg.GetDB(), sessionID)
	require.NoError(t, err)
	ApplyTodoDeltaAndEmit(cfg, nil, task, scope, 3,
		&TodoDelta{Update: []TodoUpdate{{ID: "todo-2", Text: "retry"}}}, nil)
	rawAfter, err := yakit.GetAISessionMetaSnapshot(cfg.GetDB(), sessionID)
	require.NoError(t, err)
	require.Equal(t, rawBefore, rawAfter, "idempotent todo operations should not write another snapshot")
}

func TestSessionSnapshotTodo_SubAgentDoesNotLeakOnRestore(t *testing.T) {
	const sessionID = "snapshot-todo-subagent"
	parent := todoSnapshotTestConfig(t, sessionID)
	rootTask := todoSnapshotTestTask("parent-task")
	childTask := todoSnapshotTestTask("child-task")
	childTask.SetSubAgent(true)
	BeginSessionSnapshotExecutionForTask(parent, rootTask, time.Now())
	ApplyTodoDeltaAndEmit(parent, nil, rootTask, BuildVerificationTodoScope(rootTask), 0,
		&TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "parent only"}}}, nil)
	child := NewConfig(context.Background(), append(ConvertConfigToOptions(parent),
		WithSessionPromptState(parent.SessionPromptState.ForkForSubAgent()))...)
	BeginSessionSnapshotExecutionForTask(child, childTask, time.Now())
	ApplyTodoDeltaAndEmit(child, nil, childTask, BuildVerificationTodoScope(childTask), 0,
		&TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "child only"}}}, nil)
	require.Empty(t, parent.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(childTask)))

	restoredParent := NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
	restoredParent.RestoreSessionSnapshotTodos(rootTask)
	require.Len(t, restoredParent.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(rootTask)), 1)
	require.Empty(t, restoredParent.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(childTask)))
	restoredChild := NewConfig(context.Background(), append(ConvertConfigToOptions(restoredParent),
		WithSessionPromptState(restoredParent.SessionPromptState.ForkForSubAgent()))...)
	restoredChild.RestoreSessionSnapshotTodos(childTask)
	require.Len(t, restoredChild.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(childTask)), 1)
	require.Empty(t, restoredChild.SnapshotVerificationTodoItemsByScope(BuildVerificationTodoScope(rootTask)))
}

func TestSessionSnapshotTodo_LoadsVersionOneWithoutTodos(t *testing.T) {
	const sessionID = "snapshot-todo-legacy"
	cfg := todoSnapshotTestConfig(t, sessionID)
	previous := SessionSnapshotDocument{
		SchemaVersion: 1, Revision: 4,
		Tasks: map[string]*SessionTaskSnapshot{
			"old-task": {TaskID: "old-task", Attempt: 1, Revision: 1, Status: "completed", IsFinal: true,
				Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{Status: "completed"}}},
		},
	}
	raw, err := json.Marshal(previous)
	require.NoError(t, err)
	require.NoError(t, yakit.UpdateAISessionMetaSnapshot(cfg.GetDB(), sessionID, string(raw)))

	restored := NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
	task := restored.GetSessionTaskSnapshot("old-task")
	require.NotNil(t, task)
	require.NotNil(t, task.Todo)
	require.Empty(t, task.Todo.Items)
	require.Equal(t, int64(1), task.Revision, "legacy task revision is unaffected by todo normalization")

	restored.MaterializeSessionSnapshot(nil, &SessionSnapshot{Execution: &SessionSnapshotExecution{Status: "processing"}})
	updated, err := yakit.GetAISessionMetaSnapshot(cfg.GetDB(), sessionID)
	require.NoError(t, err)
	var document SessionSnapshotDocument
	require.NoError(t, json.Unmarshal([]byte(updated), &document))
	require.Equal(t, SessionSnapshotDocumentSchemaVersion, document.SchemaVersion)
}
