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
	t.Cleanup(func() { require.NoError(t, db.Close()) })
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

func TestSessionSnapshotDocument_CumulativeExecutionExcludesIdleBetweenTasks(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	startedAt := time.Unix(1_700_000_000, 0)
	task1 := NewStatefulTaskBase("task-1", "first", context.Background(), nil, true)
	task2 := NewStatefulTaskBase("task-2", "second", context.Background(), nil, true)

	cfg.MaterializeSessionSnapshot(task1, &SessionSnapshot{
		Execution: &SessionSnapshotExecution{
			TaskName:  task1.GetName(),
			Status:    "completed",
			StartedAt: startedAt.Unix(),
			EndedAt:   startedAt.Add(2 * time.Minute).Unix(),
		},
	})
	snapshot := cfg.MaterializeSessionSnapshot(task2, &SessionSnapshot{
		Execution: &SessionSnapshotExecution{
			TaskName:  task2.GetName(),
			Status:    "completed",
			StartedAt: startedAt.Add(10 * time.Minute).Unix(),
			EndedAt:   startedAt.Add(13*time.Minute + 25*time.Second).Unix(),
		},
	})

	require.NotNil(t, snapshot.Execution)
	require.Equal(t, startedAt.Unix(), snapshot.Execution.StartedAt)
	require.Equal(t, startedAt.Add(13*time.Minute+25*time.Second).Unix(), snapshot.Execution.EndedAt)
	require.EqualValues(t, 325, snapshot.Execution.ExecutionSeconds)
	require.Equal(t, 5, snapshot.Execution.ExecutionMinutes)
}

func TestCumulativeSessionSnapshotExecutionSeconds_MergesOverlappingIntervals(t *testing.T) {
	seconds := cumulativeSessionSnapshotExecutionSeconds([]sessionSnapshotExecutionInterval{
		{startedAt: 10, endedAt: 130},
		{startedAt: 70, endedAt: 190},
		{startedAt: 250, endedAt: 310},
	})

	require.EqualValues(t, 240, seconds)
}

func TestSessionSnapshotDocument_CumulativeExecutionIncludesPreviousAttempts(t *testing.T) {
	cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
	store := cfg.getSessionSnapshotDocumentStore()
	store.document.Tasks = map[string]*SessionTaskSnapshot{
		"task-1": {
			TaskID:  "task-1",
			Status:  "completed",
			IsFinal: true,
			Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{
				Status:          "completed",
				StartedAt:       100,
				EndedAt:         200,
				ExecutionRounds: 2,
			}},
			PreviousAttempts: []SessionTaskSnapshotAttempt{
				{
					Attempt: 1,
					Status:  "aborted",
					Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{
						Status:          "aborted",
						StartedAt:       10,
						EndedAt:         40,
						ExecutionRounds: 3,
					}},
				},
			},
		},
	}

	execution := store.buildCumulativeExecutionLocked(cfg, nil)

	require.EqualValues(t, 10, execution.StartedAt)
	require.EqualValues(t, 200, execution.EndedAt)
	require.EqualValues(t, 130, execution.ExecutionSeconds)
	require.Equal(t, 2, execution.ExecutionMinutes)
	require.Equal(t, 5, execution.ExecutionRounds)
}

func TestNormalizeSessionSnapshotDocument_RepairsLegacyExecutionSeconds(t *testing.T) {
	doc := SessionSnapshotDocument{
		SchemaVersion: 2,
		Revision:      3,
		Session: &SessionSnapshot{
			Execution: &SessionSnapshotExecution{
				Status:           "completed",
				StartedAt:        100,
				EndedAt:          1000,
				ExecutionMinutes: 15,
			},
		},
		Tasks: map[string]*SessionTaskSnapshot{
			"task-1": {
				TaskID: "task-1",
				Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{
					StartedAt: 100,
					EndedAt:   220,
				}},
			},
			"task-2": {
				TaskID: "task-2",
				Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{
					StartedAt: 700,
					EndedAt:   1000,
				}},
			},
		},
	}

	normalizeSessionSnapshotDocument(&doc)

	require.Equal(t, SessionSnapshotDocumentSchemaVersion, doc.SchemaVersion)
	require.EqualValues(t, 420, doc.Session.Execution.ExecutionSeconds,
		"12 minutes of task time must be counted instead of the 15-minute wall-clock span")
	require.Equal(t, 7, doc.Session.Execution.ExecutionMinutes)
	require.EqualValues(t, 120, doc.Tasks["task-1"].Snapshot.Execution.ExecutionSeconds)
	require.Equal(t, 2, doc.Tasks["task-1"].Snapshot.Execution.ExecutionMinutes)
	require.EqualValues(t, 300, doc.Tasks["task-2"].Snapshot.Execution.ExecutionSeconds)
	require.Equal(t, 5, doc.Tasks["task-2"].Snapshot.Execution.ExecutionMinutes)
}

func TestNormalizeSessionSnapshotDocument_PreservesExecutionSecondsWithoutIntervals(t *testing.T) {
	doc := SessionSnapshotDocument{
		SchemaVersion: SessionSnapshotDocumentSchemaVersion,
		Session: &SessionSnapshot{
			Execution: &SessionSnapshotExecution{
				Status:           "completed",
				ExecutionMinutes: 8,
			},
		},
		Tasks: map[string]*SessionTaskSnapshot{
			"legacy-task": {
				TaskID:   "legacy-task",
				Snapshot: &SessionSnapshot{Execution: &SessionSnapshotExecution{Status: "completed"}},
			},
		},
	}

	normalizeSessionSnapshotDocument(&doc)

	require.EqualValues(t, 480, doc.Session.Execution.ExecutionSeconds)
	require.Equal(t, 8, doc.Session.Execution.ExecutionMinutes)
}

func TestConvertConfigToOptions_SharesSessionSnapshotDocumentStore(t *testing.T) {
	parent := NewConfig(context.Background(), WithDisableAutoSkills(true))
	child := NewConfig(context.Background(), ConvertConfigToOptions(parent)...)
	require.Same(t, parent.getSessionSnapshotDocumentStore(), child.getSessionSnapshotDocumentStore())
}
