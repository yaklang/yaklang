package aicommon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSessionSnapshotDocument_PersistsCumulativeAndTaskViews(t *testing.T) {
	originProjectDBPath := consts.GetCurrentProjectDatabasePath()
	require.NoError(t, consts.SetGormProjectDatabase(filepath.Join(t.TempDir(), "session-snapshot-document.db")))
	t.Cleanup(func() {
		require.NoError(t, consts.SetGormProjectDatabase(originProjectDBPath))
	})

	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISession{}).Error)
	const sessionID = "snapshot-document-session"
	require.NoError(t, yakit.RegisterAIAgentSession(db, sessionID))

	cfg := NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
	cfg.SetHotpatchCurrentTaskIdResolver(func() string { return "task-1" })
	task1 := NewStatefulTaskBase("task-1", "first", context.Background(), nil, true)
	task1.SetName("first task")

	startedAt := time.Unix(1_700_000_000, 0)
	cfg.ResetSessionSnapshotExecution(task1.GetName(), "processing", startedAt)
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{ToolCallID: "call-1", Success: true})
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{ToolCallID: "call-1", Success: true})
	cfg.RecordSessionSnapshotFileWrite("/tmp/a.txt")
	cfg.RecordSessionSnapshotFileWrite("/tmp/a.txt")
	cfg.FinalizeSessionSnapshotExecution("completed", startedAt.Add(2*time.Minute))

	task1Execution := cfg.BuildSessionSnapshotExecution(task1)
	task1Execution.ExecutionRounds = 3
	first := cfg.MaterializeSessionSnapshot(task1, &SessionSnapshot{
		Revision:  1,
		UpdatedAt: startedAt.Unix(),
		Execution: task1Execution,
	})
	require.Equal(t, 1, first.Execution.ToolCallTotal)
	require.Equal(t, 3, first.Execution.ExecutionRounds)
	require.Equal(t, 1, first.Execution.ModifiedFileCount, "session count is unique modified paths")
	require.Len(t, first.Tasks, 1)
	require.True(t, first.Tasks[0].IsFinal)

	cfg.SetHotpatchCurrentTaskIdResolver(func() string { return "task-2" })
	task2 := NewStatefulTaskBase("task-2", "second", context.Background(), nil, true)
	task2.SetName("second task")
	cfg.ResetSessionSnapshotExecution(task2.GetName(), "processing", startedAt.Add(3*time.Minute))
	cfg.RecordSessionSnapshotToolCall(&aitool.ToolResult{ToolCallID: "call-2", Success: false})
	task2Execution := cfg.BuildSessionSnapshotExecution(task2)
	task2Execution.ExecutionRounds = 2
	second := cfg.MaterializeSessionSnapshot(task2, &SessionSnapshot{
		Revision:  1,
		UpdatedAt: startedAt.Add(3 * time.Minute).Unix(),
		Execution: task2Execution,
	})
	require.Equal(t, 2, second.Execution.ToolCallTotal)
	require.Equal(t, 5, second.Execution.ExecutionRounds)
	require.Equal(t, 1, second.Execution.ToolCallSuccess)
	require.Equal(t, 1, second.Execution.ToolCallFailed)
	require.Len(t, second.Tasks, 2)

	raw, err := yakit.GetAISessionMetaSnapshot(db, sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	var persisted SessionSnapshotDocument
	require.NoError(t, json.Unmarshal([]byte(raw), &persisted))
	require.Len(t, persisted.Tasks, 2)
	require.Equal(t, 2, persisted.Session.Execution.ToolCallTotal)
	require.Equal(t, 5, persisted.Session.Execution.ExecutionRounds)

	restored := NewConfig(context.Background(), WithPersistentSessionId(sessionID), WithDisableAutoSkills(true))
	restoredTask := restored.GetSessionTaskSnapshot("task-1")
	require.NotNil(t, restoredTask)
	require.True(t, restoredTask.IsFinal)
	require.Equal(t, 1, restoredTask.Snapshot.Execution.ToolCallTotal)
	require.Equal(t, 3, restoredTask.Snapshot.Execution.ExecutionRounds)

	restored.SetHotpatchCurrentTaskIdResolver(func() string { return "task-1" })
	BeginSessionSnapshotExecutionForTask(restored, task1, startedAt.Add(5*time.Minute))
	retriedSession := restored.MaterializeSessionSnapshot(task1, &SessionSnapshot{
		Execution: restored.BuildSessionSnapshotExecution(task1),
	})
	require.Equal(t, 5, retriedSession.Execution.ExecutionRounds,
		"session rounds retain previous task attempts")
	retried := restored.GetSessionTaskSnapshot("task-1")
	require.NotNil(t, retried)
	require.Equal(t, 2, retried.Attempt)
	require.False(t, retried.IsFinal)
	require.Len(t, retried.PreviousAttempts, 1)
	require.Equal(t, "completed", retried.PreviousAttempts[0].Status)
}

func TestConvertConfigToOptions_SharesSessionSnapshotDocumentStore(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAutoSkills(true))
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.Same(t, parent.getSessionSnapshotDocumentStore(), child.getSessionSnapshotDocumentStore())
}
