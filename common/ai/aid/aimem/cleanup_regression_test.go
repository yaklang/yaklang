package aimem

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func cleanupRegressionDB(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "cleanup.db"))
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{},
		&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
	return db
}

func cleanupRegressionBackend(t *testing.T, db *gorm.DB) *AIMemoryHNSWBackend {
	t.Helper()
	backend, err := NewAIMemoryHNSWBackend(WithHNSWDatabase(db), WithHNSWSessionID("default"), WithHNSWAutoSave(false))
	require.NoError(t, err)
	return backend
}

func insertCleanupRegressionMemory(t *testing.T, db *gorm.DB, backend *AIMemoryHNSWBackend, id string) {
	t.Helper()
	vec := []float32{1, 0, 0, 0, 0, 0, 1}
	require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: id, SessionID: "default", Content: id, CorePactVector: schema.FloatArray(vec)}).Error)
	require.NoError(t, backend.Add(&aicommon.MemoryEntity{Id: id, CorePactVector: vec}))
}

func TestMUSTPASS_CleanupEmptyGraphRestart(t *testing.T) {
	db := cleanupRegressionDB(t)
	writer := cleanupRegressionBackend(t, db)
	insertCleanupRegressionMemory(t, db, writer, "deleted")
	require.NoError(t, writer.SaveGraph())
	require.NoError(t, BatchCleanupMemories(context.Background(), db, "default", []string{"deleted"}))
	var collection schema.AIMemoryCollection
	require.NoError(t, db.Where("session_id = ?", "default").First(&collection).Error)
	require.Empty(t, collection.GraphBinary, "deleting the last node must persist an empty graph")
	require.Empty(t, cleanupRegressionBackend(t, db).ListMemoryIDs())
	// An already-open triage closes after the cleanup worker completes.
	require.NoError(t, writer.Close())
	require.Empty(t, cleanupRegressionBackend(t, db).ListMemoryIDs(), "stale writer resurrected a deleted node")
	for i := 0; i < 3; i++ {
		require.NoError(t, BatchCleanupMemories(context.Background(), db, "default", []string{"deleted"}))
		require.Empty(t, cleanupRegressionBackend(t, db).ListMemoryIDs())
	}
}

func TestMUSTPASS_CleanupStaleWriterPreservesNewMemory(t *testing.T) {
	db := cleanupRegressionDB(t)
	writer := cleanupRegressionBackend(t, db)
	for _, id := range []string{"deleted", "survivor"} {
		insertCleanupRegressionMemory(t, db, writer, id)
	}
	require.NoError(t, writer.SaveGraph())
	require.NoError(t, BatchCleanupMemories(context.Background(), db, "default", []string{"deleted"}))
	insertCleanupRegressionMemory(t, db, writer, "new")
	require.NoError(t, writer.SaveGraph())
	require.ElementsMatch(t, []string{"survivor", "new"}, cleanupRegressionBackend(t, db).ListMemoryIDs())
}

func TestMUSTPASS_CleanupMemoryRollbackAndCancellation(t *testing.T) {
	db := cleanupRegressionDB(t)
	writer := cleanupRegressionBackend(t, db)
	insertCleanupRegressionMemory(t, db, writer, "keep")
	require.NoError(t, writer.SaveGraph())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, BatchCleanupMemories(ctx, db, "default", []string{"keep"}), context.Canceled)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_memory_save BEFORE UPDATE ON ai_memory_collections_v1
		BEGIN SELECT RAISE(ABORT, 'injected memory save failure'); END`).Error)
	require.ErrorContains(t, BatchCleanupMemories(context.Background(), db, "default", []string{"keep"}), "injected memory save failure")
	var count int
	require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Count(&count).Error)
	require.Equal(t, 1, count)
	require.Equal(t, []string{"keep"}, cleanupRegressionBackend(t, db).ListMemoryIDs())
	require.NoError(t, db.Exec("DROP TRIGGER fail_memory_save").Error)
	require.NoError(t, BatchCleanupMemories(context.Background(), db, "default", []string{"keep"}))
}

func TestMUSTPASS_CleanupCooldownAcrossRestartAndDatabases(t *testing.T) {
	db := cleanupRegressionDB(t)
	now := time.Now()
	_, claimed, err := claimCleanup(context.Background(), db, now)
	require.NoError(t, err)
	require.True(t, claimed)
	// A fresh coordinator has no in-process timestamp, as on restart.
	coordinator := &cleanupCoordinator{}
	coordinator.runCleanup(db)
	_, claimed, err = claimCleanup(context.Background(), db, now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)
	other := cleanupRegressionDB(t)
	_, claimed, err = claimCleanup(context.Background(), other, now)
	require.NoError(t, err)
	require.True(t, claimed, "one database must not suppress another")
	_, claimed, err = claimCleanup(context.Background(), db, now.Add(cleanupInterval+time.Second))
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestMUSTPASS_CleanupScanBoundedAndNoStarvation(t *testing.T) {
	db := cleanupRegressionDB(t)
	expired := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 500; i++ {
		score := 1.0
		if i >= 450 {
			score = 0.0
		}
		e := &schema.AIMemoryEntity{MemoryID: fmt.Sprint(i), SessionID: "default", Content: "body", C_Score: score, R_Score: score, A_Score: score, P_Score: score, O_Score: score, E_Score: score, T_Score: .1}
		e.CreatedAt = time.Now().Add(-90 * 24 * time.Hour)
		e.ExpiresAt = &expired
		require.NoError(t, db.Create(e).Error)
	}
	config := DefaultCleanupConfig()
	config.MaxBatchSize = 10
	ids, err := ScanLowValueMemories(db, "ai_memory_entities_v1", "default", config)
	require.NoError(t, err)
	require.Len(t, ids, 10, "valuable early rows must not hide later low-value rows")
	ids, err = ScanOverCountMemories(db, "ai_memory_entities_v1", "default", config)
	require.NoError(t, err)
	require.Len(t, ids, 10)
	ids, err = ScanAllCleanupMemories(db, "ai_memory_entities_v1", "default", config)
	require.NoError(t, err)
	require.LessOrEqual(t, len(ids), 10)
	require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Where("session_id = ?", "default").Delete(&schema.AIMemoryEntity{}).Error)
	ids, err = ScanAllCleanupMemories(db, "ai_memory_entities_v1", "default", config)
	require.NoError(t, err)
	require.Empty(t, ids, "soft-deleted rows must not be repeatedly selected")
}

func TestMUSTPASS_CleanupAutosaveBurst(t *testing.T) {
	db := cleanupRegressionDB(t)
	backend, err := NewAIMemoryHNSWBackend(WithHNSWDatabase(db), WithHNSWSessionID("default"))
	require.NoError(t, err)
	// Stall persistence as a contended local SQLite database would, then
	// exercise 1000 changes through the public index API.
	backend.saveMutex.Lock()
	before := runtime.NumGoroutine()
	for i := 0; i < 1000; i++ {
		require.NoError(t, backend.Add(&aicommon.MemoryEntity{Id: "burst", CorePactVector: []float32{1, 0, 0, 0, 0, 0, 1}}))
	}
	growth := runtime.NumGoroutine() - before
	backend.saveMutex.Unlock()
	require.Less(t, growth, 20, "mutations spawned an unbounded queue of saving goroutines")
	require.NoError(t, backend.Close())
	require.Equal(t, []string{"burst"}, cleanupRegressionBackend(t, db).ListMemoryIDs())
}

func TestMUSTPASS_CleanupSessionPagination(t *testing.T) {
	db := cleanupRegressionDB(t)
	past := time.Now().Add(-time.Hour)
	for i := 0; i < 70; i++ {
		require.NoError(t, db.Create(&schema.AIMemoryEntity{MemoryID: fmt.Sprint(i), SessionID: fmt.Sprintf("session-%03d", i), Content: "expired", ExpiresAt: &past}).Error)
	}
	var counts []int
	for pass := 0; pass < 3; pass++ {
		coordinator := &cleanupCoordinator{}
		coordinator.runCleanup(db)
		var count int
		require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Count(&count).Error)
		counts = append(counts, count)
		var row schema.ProjectGeneralStorage
		require.NoError(t, db.Where("key = ?", strconv.Quote(cleanupStateKey)).First(&row).Error)
		value, err := strconv.Unquote(row.Value)
		require.NoError(t, err)
		var state cleanupState
		require.NoError(t, json.Unmarshal([]byte(value), &state))
		state.StartedAt = time.Now().Add(-2 * cleanupInterval)
		data, err := json.Marshal(state)
		require.NoError(t, err)
		require.NoError(t, db.Model(&row).Update("value", strconv.Quote(string(data))).Error)
	}
	require.Equal(t, []int{38, 6, 0}, counts, "each run must visit a bounded, progressing session page")
}

func TestMUSTPASS_CleanupCorruptCheckpointAndTransaction(t *testing.T) {
	db := cleanupRegressionDB(t)
	require.NoError(t, db.Create(&schema.ProjectGeneralStorage{Key: strconv.Quote(cleanupStateKey), Value: "malformed checkpoint"}).Error)
	_, claimed, err := claimCleanup(context.Background(), db, time.Now())
	require.NoError(t, err)
	require.True(t, claimed)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NotPanics(t, func() { MaybeCleanup(tx) })
}

func TestMUSTPASS_CleanupCorruptRAGRetainsRows(t *testing.T) {
	db := cleanupRegressionDB(t)
	backend := cleanupRegressionBackend(t, db)
	insertCleanupRegressionMemory(t, db, backend, "keep")
	require.NoError(t, backend.SaveGraph())
	require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Where("memory_id = ?", "keep").UpdateColumn("potential_questions", `["question"]`).Error)
	collection := &schema.VectorStoreCollection{Name: Session2MemoryName("default"), UUID: "corrupt", Dimension: 1024, M: 16, Ml: .25, EfSearch: 20, GraphBinary: []byte("broken graph")}
	require.NoError(t, db.Create(collection).Error)
	require.Error(t, BatchCleanupMemories(context.Background(), db, "default", []string{"keep"}))
	var memories, collections int
	require.NoError(t, db.Model(&schema.AIMemoryEntity{}).Count(&memories).Error)
	require.NoError(t, db.Model(&schema.VectorStoreCollection{}).Count(&collections).Error)
	require.Equal(t, 1, memories)
	require.Equal(t, 1, collections, "maintenance must not delete a corrupt RAG collection")
}

func TestMUSTPASS_CleanupTableRecreationStaleWriter(t *testing.T) {
	db := cleanupRegressionDB(t)
	stale := cleanupRegressionBackend(t, db)
	insertCleanupRegressionMemory(t, db, stale, "deleted")
	require.NoError(t, stale.SaveGraph())
	// The delete-all endpoint drops and recreates these tables; primary keys
	// can be reused by a newly created session while old backends still live.
	require.NoError(t, db.DropTableIfExists(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}).Error)
	require.NoError(t, db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}).Error)
	fresh := cleanupRegressionBackend(t, db)
	insertCleanupRegressionMemory(t, db, fresh, "new")
	require.NoError(t, fresh.SaveGraph())
	require.NoError(t, stale.Close())
	require.Equal(t, []string{"new"}, cleanupRegressionBackend(t, db).ListMemoryIDs())
}
