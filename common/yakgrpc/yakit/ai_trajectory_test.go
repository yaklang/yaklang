package yakit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func trajectoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AIAgentRuntime{}, &schema.AiCheckpoint{},
		&schema.AiOutputEvent{}, &schema.AiProcessAndAiEvent{}, &schema.AISessionPlanAndExec{}, &schema.AITrajectory{}).Error)
	for _, session := range []string{"session_%", "session_other"} {
		require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: session, PersistentSession: session}).Error)
	}
	return db
}

func TestAITrajectorySegmentsAndRevision(t *testing.T) {
	db := trajectoryTestDB(t)
	terminal := &schema.AITrajectory{PersistentSessionID: "session_%", TaskID: "task", Kind: schema.AITrajectoryTerminal, Prompt: "original"}
	changed, err := SaveAITrajectory(db, terminal)
	require.NoError(t, err)
	require.True(t, changed)
	id, hash := terminal.ID, terminal.ContentHash
	for _, key := range []string{"compress-1", "compress-2"} {
		changed, err = SaveAITrajectory(db, &schema.AITrajectory{PersistentSessionID: "session_%", Kind: schema.AITrajectoryCompression,
			TrajectoryKey: key, Prompt: "exact source", Response: "summary"})
		require.NoError(t, err)
		require.True(t, changed)
	}
	claimed, err := ClaimAITrajectoryEvaluation(db, id, hash)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	duplicate, err := ClaimAITrajectoryEvaluation(db, id, hash)
	require.NoError(t, err)
	require.Nil(t, duplicate)
	zero := 0
	updated, err := UpdateAITrajectoryEvaluation(db, id, hash, schema.AITrajectoryEvaluationCompleted, "judge", "low training value", "", &zero)
	require.NoError(t, err)
	require.True(t, updated)
	changed, err = SaveAITrajectory(db, terminal)
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, terminal.ValueScore)
	require.Zero(t, *terminal.ValueScore, "a judged zero is distinct from an unjudged NULL")
	terminal.Prompt = "updated prompt"
	changed, err = SaveAITrajectory(db, terminal)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, id, terminal.ID)
	require.NotEqual(t, hash, terminal.ContentHash)
	require.Nil(t, terminal.ValueScore)
	require.Empty(t, terminal.EvaluationReason)
	require.Equal(t, schema.AITrajectoryEvaluationPending, terminal.EvaluationStatus)
	updated, err = UpdateAITrajectoryEvaluation(db, id, hash, schema.AITrajectoryEvaluationCompleted, "judge", "stale", "", &zero)
	require.NoError(t, err)
	require.False(t, updated)
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	require.Len(t, rows, 3, "two compressions plus one terminal")
	require.Equal(t, schema.AITrajectoryTerminal, rows[0].Kind)
	require.Equal(t, "compress-1", rows[1].TrajectoryKey)
	require.Equal(t, "compress-2", rows[2].TrajectoryKey)
	_, err = SaveAITrajectory(db, &schema.AITrajectory{PersistentSessionID: "session_%", Kind: schema.AITrajectoryCompression,
		TrajectoryKey: "compress-1", Prompt: "edited source"})
	require.ErrorContains(t, err, "immutable")
}

func TestAITrajectoryDeletionAndLateEvaluation(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact_session", true: "all_sessions"}[all], func(t *testing.T) {
			db := trajectoryTestDB(t)
			var active *schema.AITrajectory
			for _, session := range []string{"session_%", "session_other"} {
				for _, kind := range []string{schema.AITrajectoryTerminal, schema.AITrajectoryCompression} {
					row := &schema.AITrajectory{PersistentSessionID: session, TaskID: "task", Kind: kind, TrajectoryKey: "compression", Prompt: "source"}
					_, err := SaveAITrajectory(db, row)
					require.NoError(t, err)
					if session == "session_%" && kind == schema.AITrajectoryTerminal {
						active = row
					}
				}
			}
			_, err := ClaimAITrajectoryEvaluation(db, active.ID, active.ContentHash)
			require.NoError(t, err)
			if all {
				_, _, _, _, err = DeleteAllAISessionData(db)
			} else {
				_, _, err = DeleteAISession(db, "session_%")
			}
			require.NoError(t, err)
			var count int64
			require.NoError(t, db.Unscoped().Model(&schema.AITrajectory{}).Where("persistent_session_id = ?", "session_%").Count(&count).Error)
			require.Zero(t, count, "trajectory payloads must be physically deleted")
			score := 8
			updated, err := UpdateAITrajectoryEvaluation(db, active.ID, active.ContentHash,
				schema.AITrajectoryEvaluationCompleted, "judge", "late result", "", &score)
			require.NoError(t, err)
			require.False(t, updated)
			_, err = SaveAITrajectory(db, active)
			require.ErrorContains(t, err, "no runtime")
			rows, err := QueryAITrajectories(db, "session_other")
			require.NoError(t, err)
			if all {
				require.Empty(t, rows)
			} else {
				require.Len(t, rows, 2)
			}
		})
	}
}

func TestAITrajectoryConcurrentTerminalUpsert(t *testing.T) {
	db := trajectoryTestDB(t)
	var changed atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := SaveAITrajectory(db, &schema.AITrajectory{PersistentSessionID: "session_%", TaskID: "task", Kind: schema.AITrajectoryTerminal, Prompt: "same prompt"})
			if err != nil {
				t.Error(err)
			}
			if ok {
				changed.Add(1)
			}
		}()
	}
	wg.Wait()
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int32(1), changed.Load())
}

func TestAITrajectoryResumesOnlyUnfinishedEvaluations(t *testing.T) {
	db := trajectoryTestDB(t)
	for _, key := range []string{"pending", "skipped", "expired", "active", "completed"} {
		record := &schema.AITrajectory{PersistentSessionID: "session_%", Kind: schema.AITrajectoryCompression, TrajectoryKey: key, Prompt: "large source"}
		_, err := SaveAITrajectory(db, record)
		require.NoError(t, err)
		switch key {
		case "skipped":
			_, err = UpdateAITrajectoryEvaluation(db, record.ID, record.ContentHash, schema.AITrajectoryEvaluationSkipped, "", "", "offline", nil)
		case "expired", "active":
			_, err = ClaimAITrajectoryEvaluation(db, record.ID, record.ContentHash)
			if key == "expired" {
				require.NoError(t, db.Model(record).UpdateColumn("updated_at", time.Now().Add(-2*time.Minute)).Error)
			}
		case "completed":
			score := 7
			_, err = UpdateAITrajectoryEvaluation(db, record.ID, record.ContentHash, schema.AITrajectoryEvaluationCompleted, "judge", "useful evidence", "", &score)
		}
		require.NoError(t, err)
	}
	_, err := SaveAITrajectory(db, &schema.AITrajectory{PersistentSessionID: "session_other", TaskID: "task", Kind: schema.AITrajectoryTerminal})
	require.NoError(t, err)
	receipts, err := ResumeAITrajectoryEvaluations(db, "session_%")
	require.NoError(t, err)
	require.Len(t, receipts, 3)
	for _, receipt := range receipts {
		require.Equal(t, "session_%", receipt.PersistentSessionID)
		require.NotEmpty(t, receipt.ContentHash)
		require.Empty(t, receipt.Prompt, "resume must fetch identities without loading large prompt bodies")
	}
}

func TestAITrajectoryOlderQueuedTerminalCannotOverwriteLatest(t *testing.T) {
	db := trajectoryTestDB(t)
	older := schema.AITrajectory{PersistentSessionID: "session_%", TaskID: "task", Kind: schema.AITrajectoryTerminal,
		CapturedAt: time.Now(), Prompt: "older prompt"}
	newer := older
	newer.CapturedAt = older.CapturedAt.Add(time.Second)
	newer.Prompt = "latest prompt"
	_, err := SaveAITrajectory(db, &newer)
	require.NoError(t, err)
	changed, err := SaveAITrajectory(db, &older)
	require.NoError(t, err)
	require.False(t, changed)
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "latest prompt", rows[0].Prompt)
}

func TestAITrajectoryDeletedRuntimeCannotWriteToRecreatedSession(t *testing.T) {
	db := trajectoryTestDB(t)
	record := &schema.AITrajectory{PersistentSessionID: "session_%", RuntimeID: "session_%", TaskID: "task", Kind: schema.AITrajectoryTerminal}
	_, err := SaveAITrajectory(db, record)
	require.NoError(t, err)
	_, _, err = DeleteAISession(db, "session_%")
	require.NoError(t, err)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "replacement-runtime", PersistentSession: "session_%"}).Error)
	_, err = SaveAITrajectory(db, record)
	require.ErrorContains(t, err, "no runtime")
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestAITrajectoryRepeatedLatestPromptKeepsCaptureWatermark(t *testing.T) {
	db := trajectoryTestDB(t)
	first := schema.AITrajectory{PersistentSessionID: "session_%", TaskID: "task", Kind: schema.AITrajectoryTerminal,
		CapturedAt: time.Now(), Prompt: "latest prompt"}
	_, err := SaveAITrajectory(db, &first)
	require.NoError(t, err)
	latest := first
	latest.CapturedAt = first.CapturedAt.Add(2 * time.Second)
	changed, err := SaveAITrajectory(db, &latest)
	require.NoError(t, err)
	require.False(t, changed, "a repeated prompt must not be scored again")
	delayed := first
	delayed.CapturedAt, delayed.Prompt = first.CapturedAt.Add(time.Second), "delayed intermediate prompt"
	changed, err = SaveAITrajectory(db, &delayed)
	require.NoError(t, err)
	require.False(t, changed)
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "latest prompt", rows[0].Prompt)
	require.WithinDuration(t, latest.CapturedAt, rows[0].CapturedAt, time.Millisecond)
}

func TestAITrajectoryQueryPreservesChronologyAcrossTasks(t *testing.T) {
	db := trajectoryTestDB(t)
	start := time.Now()
	for i, record := range []schema.AITrajectory{
		{Kind: schema.AITrajectoryCompression, TrajectoryKey: "compression-first"},
		{Kind: schema.AITrajectoryTerminal, TaskID: "first"},
		{Kind: schema.AITrajectoryCompression, TrajectoryKey: "compression-second"},
		{Kind: schema.AITrajectoryTerminal, TaskID: "second"},
	} {
		record.PersistentSessionID, record.CapturedAt = "session_%", start.Add(time.Duration(i)*time.Second)
		_, err := SaveAITrajectory(db, &record)
		require.NoError(t, err)
	}
	rows, err := QueryAITrajectories(db, "session_%")
	require.NoError(t, err)
	var keys []string
	for _, row := range rows {
		keys = append(keys, row.TrajectoryKey)
	}
	require.Equal(t, []string{"compression-first", "terminal:first", "compression-second", "terminal:second"}, keys)
}
