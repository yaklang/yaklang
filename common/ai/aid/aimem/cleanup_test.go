package aimem

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

// --- Helpers ---

func createCleanupTestMemory(t *testing.T, sessionID string) *AIMemoryTriage {
	t.Helper()
	ctx := context.Background()
	mem, err := CreateTestAIMemory(sessionID, WithInvoker(mock.NewMockInvoker(ctx)))
	require.NoError(t, err)
	require.NotNil(t, mem)
	return mem
}

func saveTestMemoryDirectly(t *testing.T, mem *AIMemoryTriage, id string, tScore float64, createdAt time.Time) {
	t.Helper()
	entity := &aicommon.MemoryEntity{
		Id:        id,
		CreatedAt: createdAt,
		Content:   "test content for " + id,
		Tags:      []string{"test"},
		C_Score:   0.5,
		O_Score:   0.5,
		R_Score:   0.5,
		E_Score:   0.5,
		P_Score:   0.5,
		A_Score:   0.5,
		T_Score:   tScore,
		CorePactVector: []float32{
			float32(0.5), float32(0.5), float32(0.5),
			float32(0.5), float32(0.5), float32(0.5),
			float32(tScore),
		},
	}
	err := mem.SaveMemoryEntities(entity)
	require.NoError(t, err)
}

// --- CalcExpiresAt ---

func TestCalcExpiresAt_Transient(t *testing.T) {
	now := time.Now()
	expires := CalcExpiresAt(0.1, now)
	require.NotNil(t, expires)
	assert.WithinDuration(t, now.Add(7*24*time.Hour), *expires, time.Second)
}

func TestCalcExpiresAt_ShortTerm(t *testing.T) {
	now := time.Now()
	expires := CalcExpiresAt(0.4, now)
	require.NotNil(t, expires)
	assert.WithinDuration(t, now.Add(30*24*time.Hour), *expires, time.Second)
}

func TestCalcExpiresAt_MidTerm(t *testing.T) {
	now := time.Now()
	expires := CalcExpiresAt(0.7, now)
	require.NotNil(t, expires)
	assert.WithinDuration(t, now.Add(90*24*time.Hour), *expires, time.Second)
}

func TestCalcExpiresAt_LongTerm(t *testing.T) {
	now := time.Now()
	expires := CalcExpiresAt(0.9, now)
	assert.Nil(t, expires, "long-term memory should not expire")
}

// --- SaveMemoryEntities auto-calculates ExpiresAt ---

func TestSaveMemoryEntities_AutoExpiresAt(t *testing.T) {
	sessionID := "test-cleanup-expires-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	now := time.Now()
	// Save a transient memory (T=0.1) — should get ExpiresAt = now+7d
	saveTestMemoryDirectly(t, mem, "mem-transient", 0.1, now)
	// Save a long-term memory (T=0.9) — should get ExpiresAt = nil
	saveTestMemoryDirectly(t, mem, "mem-longterm", 0.9, now)

	// Verify DB
	db := mem.GetDB()
	var transientEntity schema.AIMemoryEntity
	err := db.Table(mem.entityTableName()).
		Where("memory_id = ? AND session_id = ?", "mem-transient", mem.GetSessionID()).
		First(&transientEntity).Error
	require.NoError(t, err)
	assert.NotNil(t, transientEntity.ExpiresAt)
	assert.WithinDuration(t, now.Add(7*24*time.Hour), *transientEntity.ExpiresAt, time.Second)

	var longTermEntity schema.AIMemoryEntity
	err = db.Table(mem.entityTableName()).
		Where("memory_id = ? AND session_id = ?", "mem-longterm", mem.GetSessionID()).
		First(&longTermEntity).Error
	require.NoError(t, err)
	assert.Nil(t, longTermEntity.ExpiresAt, "long-term memory should have nil ExpiresAt")
}

// --- ScanExpiredMemories ---

func TestScanExpiredMemories(t *testing.T) {
	sessionID := "test-cleanup-scan-expired-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	// Create an expired memory (ExpiresAt in the past)
	pastTime := time.Now().Add(-10 * 24 * time.Hour)
	entity := &schema.AIMemoryEntity{
		MemoryID:  "expired-mem",
		SessionID: sid,
		Content:   "this is an expired memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.1,
		ExpiresAt: &pastTime,
	}
	require.NoError(t, db.Table(tableName).Create(entity).Error)

	// Create a non-expired memory
	futureTime := time.Now().Add(10 * 24 * time.Hour)
	entity2 := &schema.AIMemoryEntity{
		MemoryID:  "active-mem",
		SessionID: sid,
		Content:   "this is an active memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.4,
		ExpiresAt: &futureTime,
	}
	require.NoError(t, db.Table(tableName).Create(entity2).Error)

	// Create a non-expiring memory (ExpiresAt = nil)
	entity3 := &schema.AIMemoryEntity{
		MemoryID:  "nonexpiring-mem",
		SessionID: sid,
		Content:   "this is a non-expiring memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.9,
	}
	require.NoError(t, db.Table(tableName).Create(entity3).Error)

	// Scan expired
	expiredIDs, err := ScanExpiredMemories(db, tableName, sid, 100)
	require.NoError(t, err)
	assert.Len(t, expiredIDs, 1)
	assert.Equal(t, "expired-mem", expiredIDs[0])
}

// --- ScanLowValueMemories ---

func TestScanLowValueMemories(t *testing.T) {
	sessionID := "test-cleanup-scan-lowvalue-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	config := DefaultCleanupConfig()
	coldThreshold := time.Now().AddDate(0, 0, -config.ColdMemoryDays)

	// 1. Low-value, old → should be scanned
	lowValEntity := &schema.AIMemoryEntity{
		MemoryID:  "lowval-mem",
		SessionID: sid,
		Content:   "low value memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.2, // below 0.8
		C_Score:   0.1,
		O_Score:   0.1,
		R_Score:   0.1,
		E_Score:   0.1,
		P_Score:   0.1,
		A_Score:   0.1,
	}
	lowValEntity.CreatedAt = coldThreshold.Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(lowValEntity).Error)

	// 2. High-value, old → should NOT be scanned
	highValEntity := &schema.AIMemoryEntity{
		MemoryID:  "highval-mem",
		SessionID: sid,
		Content:   "high value memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.5,
		C_Score:   0.9,
		O_Score:   0.9,
		R_Score:   0.9,
		E_Score:   0.9,
		P_Score:   0.9,
		A_Score:   0.9,
	}
	highValEntity.CreatedAt = coldThreshold.Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(highValEntity).Error)

	// 3. Low-value, but long-term (T >= 0.8) → should NOT be scanned
	longTermEntity := &schema.AIMemoryEntity{
		MemoryID:  "longterm-mem",
		SessionID: sid,
		Content:   "long term low value memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.9, // exempt
		C_Score:   0.1,
		O_Score:   0.1,
		R_Score:   0.1,
		E_Score:   0.1,
		P_Score:   0.1,
		A_Score:   0.1,
	}
	longTermEntity.CreatedAt = coldThreshold.Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(longTermEntity).Error)

	// 4. Low-value, but too recent (CreatedAt) → should NOT be scanned
	recentEntity := &schema.AIMemoryEntity{
		MemoryID:  "recent-mem",
		SessionID: sid,
		Content:   "recent low value memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.2,
		C_Score:   0.1,
		O_Score:   0.1,
		R_Score:   0.1,
		E_Score:   0.1,
		P_Score:   0.1,
		A_Score:   0.1,
	}
	recentEntity.CreatedAt = time.Now().Add(-1 * time.Hour) // very recent
	require.NoError(t, db.Table(tableName).Create(recentEntity).Error)

	// Scan low-value
	resultIDs, err := ScanLowValueMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, resultIDs, 1)
	assert.Equal(t, "lowval-mem", resultIDs[0])
}

// --- CalcMemoryValue ---

func TestCalcMemoryValue(t *testing.T) {
	entity := &aicommon.MemoryEntity{
		R_Score: 1.0,
		A_Score: 1.0,
		P_Score: 1.0,
		C_Score: 1.0,
		O_Score: 1.0,
		E_Score: 1.0,
		T_Score: 1.0,
	}
	value := CalcMemoryValue(entity)
	assert.InDelta(t, 1.0, value, 0.001)

	entity2 := &aicommon.MemoryEntity{
		R_Score: 0.0,
		A_Score: 0.0,
		P_Score: 0.0,
		C_Score: 0.0,
		O_Score: 0.0,
		E_Score: 0.0,
		T_Score: 0.0,
	}
	value2 := CalcMemoryValue(entity2)
	assert.InDelta(t, 0.0, value2, 0.001)

	// Weights: R=0.25, A=0.20, P=0.15, C=0.15, O=0.10, E=0.05, T=0.10
	entity3 := &aicommon.MemoryEntity{
		R_Score: 1.0,
		A_Score: 0.0,
		P_Score: 0.0,
		C_Score: 0.0,
		O_Score: 0.0,
		E_Score: 0.0,
		T_Score: 0.0,
	}
	value3 := CalcMemoryValue(entity3)
	assert.InDelta(t, 0.25, value3, 0.001)
}

// --- BatchCleanupMemories ---

func TestBatchCleanupMemories(t *testing.T) {
	sessionID := "test-cleanup-batch-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	// Save 3 memories
	for _, id := range []string{"batch-mem-1", "batch-mem-2", "batch-mem-3"} {
		saveTestMemoryDirectly(t, mem, id, 0.5, time.Now())
	}

	// Verify they exist
	var count int64
	db.Table(tableName).Where("session_id = ?", sid).Count(&count)
	assert.Equal(t, int64(3), count)

	// Verify HNSW has them
	hnswIDs := mem.hnswBackend.ListMemoryIDs()
	assert.Len(t, hnswIDs, 3)

	// Batch cleanup 2 of them
	err := BatchCleanupMemories(context.Background(), db, sid, []string{"batch-mem-1", "batch-mem-2"})
	require.NoError(t, err)

	// Verify DB has 1 left
	db.Table(tableName).Where("session_id = ?", sid).Count(&count)
	assert.Equal(t, int64(1), count)

	// Verify remaining is batch-mem-3
	var remaining schema.AIMemoryEntity
	db.Table(tableName).Where("session_id = ? AND memory_id = ?", sid, "batch-mem-3").First(&remaining)
	assert.Equal(t, "batch-mem-3", remaining.MemoryID)
}

// --- ScanAllCleanupMemories (integration) ---

func TestScanAllCleanupMemories(t *testing.T) {
	sessionID := "test-cleanup-scanall-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()
	config := DefaultCleanupConfig()

	// 1. Expired memory (ExpiresAt in past)
	pastTime := time.Now().Add(-10 * 24 * time.Hour)
	expiredEntity := &schema.AIMemoryEntity{
		MemoryID:  "scan-expired",
		SessionID: sid,
		Content:   "expired",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.1,
		ExpiresAt: &pastTime,
	}
	require.NoError(t, db.Table(tableName).Create(expiredEntity).Error)

	// 2. Low-value cold memory
	coldThreshold := time.Now().AddDate(0, 0, -config.ColdMemoryDays)
	lowValEntity := &schema.AIMemoryEntity{
		MemoryID:  "scan-lowval",
		SessionID: sid,
		Content:   "low value",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.2,
		C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
	}
	lowValEntity.CreatedAt = coldThreshold.Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(lowValEntity).Error)

	// 3. Healthy memory (not expired, not low-value)
	healthyEntity := &schema.AIMemoryEntity{
		MemoryID:  "scan-healthy",
		SessionID: sid,
		Content:   "healthy memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.5,
		C_Score:   0.8, O_Score: 0.8, R_Score: 0.8, E_Score: 0.8, P_Score: 0.8, A_Score: 0.8,
	}
	healthyEntity.CreatedAt = time.Now().Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(healthyEntity).Error)

	// Scan all
	resultIDs, err := ScanAllCleanupMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, resultIDs, 2)

	// Should contain both expired and low-value
	idSet := make(map[string]bool)
	for _, id := range resultIDs {
		idSet[id] = true
	}
	assert.True(t, idSet["scan-expired"])
	assert.True(t, idSet["scan-lowval"])
	assert.False(t, idSet["scan-healthy"])
}
