package aimem

import (
	"context"
	"fmt"
	"sync/atomic"
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

// --- ScanOverCountMemories ---

func TestScanOverCountMemories(t *testing.T) {
	sessionID := "test-cleanup-overcount-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	config := DefaultCleanupConfig()
	config.MaxMemoryCount = 5
	config.OverEvictMargin = 2

	// Create 8 memories, all low-value so they're eligible for eviction
	for i := 0; i < 8; i++ {
		entity := &schema.AIMemoryEntity{
			MemoryID:  fmt.Sprintf("overcount-mem-%d", i),
			SessionID: sid,
			Content:   fmt.Sprintf("memory %d", i),
			Tags:      schema.StringArray{"test"},
			T_Score:   0.2,
			C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
		}
		entity.CreatedAt = time.Now().Add(-time.Duration(i+1) * time.Hour)
		require.NoError(t, db.Table(tableName).Create(entity).Error)
	}

	// totalCount=8, MaxMemoryCount=5, OverEvictMargin=2
	// toEvict = 8 - 5 + 2 = 5
	ids, err := ScanOverCountMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, ids, 5)

	// Create a long-term memory (T >= 0.8) — high value, should be last to evict
	longTermEntity := &schema.AIMemoryEntity{
		MemoryID:  "longterm-protected",
		SessionID: sid,
		Content:   "long term memory",
		Tags:      schema.StringArray{"test"},
		T_Score:   0.9,
		C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
	}
	longTermEntity.CreatedAt = time.Now().Add(-1 * time.Hour)
	require.NoError(t, db.Table(tableName).Create(longTermEntity).Error)

	// totalCount=9, MaxMemoryCount=5, OverEvictMargin=2
	// toEvict = 9 - 5 + 2 = 6
	// longterm-protected value = 0.25*0.1+0.20*0.1+0.15*0.1+0.15*0.1+0.10*0.1+0.05*0.1+0.10*0.9 = 0.19
	// low-value mem value      = 0.25*0.1+0.20*0.1+0.15*0.1+0.15*0.1+0.10*0.1+0.05*0.1+0.10*0.2 = 0.10
	// longterm-protected (0.19) > low-value mem (0.10), so it survives
	ids, err = ScanOverCountMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, ids, 6)

	// Verify long-term memory is NOT in the eviction list (higher value than low-T memories)
	for _, id := range ids {
		assert.NotEqual(t, "longterm-protected", id)
	}

	// Test: under limit → no eviction
	config.MaxMemoryCount = 100
	ids, err = ScanOverCountMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// --- OverCount: no zombie memories (all high-T still over limit) ---

func TestScanOverCountMemories_NoZombieMemories(t *testing.T) {
	sessionID := "test-cleanup-no-zombie-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	config := DefaultCleanupConfig()
	config.MaxMemoryCount = 3
	config.OverEvictMargin = 1

	// Create 5 ALL high-T memories (T >= 0.8)
	for i := 0; i < 5; i++ {
		entity := &schema.AIMemoryEntity{
			MemoryID:  fmt.Sprintf("hight-mem-%d", i),
			SessionID: sid,
			Content:   fmt.Sprintf("high t memory %d", i),
			Tags:      schema.StringArray{"test"},
			T_Score:   0.9,
			C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
		}
		entity.CreatedAt = time.Now().Add(-time.Duration(i+1) * time.Hour)
		require.NoError(t, db.Table(tableName).Create(entity).Error)
	}

	// totalCount=5, MaxMemoryCount=3, OverEvictMargin=1
	// toEvict = 5 - 3 + 1 = 3
	// All are T>=0.8 but there's no exemption now — lowest value ones get evicted
	ids, err := ScanOverCountMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, ids, 3, "high-T memories should still be evicted when over limit")

	// Remaining should be 5 - 3 = 2 <= MaxMemoryCount=3
}

// --- OverCount margin effectiveness ---

func TestScanOverCountMemories_MarginEffectiveness(t *testing.T) {
	sessionID := "test-cleanup-margin-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	config := DefaultCleanupConfig()
	config.MaxMemoryCount = 5
	config.OverEvictMargin = 3

	// Create exactly 6 memories (1 over limit)
	for i := 0; i < 6; i++ {
		entity := &schema.AIMemoryEntity{
			MemoryID:  fmt.Sprintf("margin-mem-%d", i),
			SessionID: sid,
			Content:   fmt.Sprintf("memory %d", i),
			Tags:      schema.StringArray{"test"},
			T_Score:   0.2,
			C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
		}
		entity.CreatedAt = time.Now().Add(-time.Duration(i+1) * time.Hour)
		require.NoError(t, db.Table(tableName).Create(entity).Error)
	}

	// totalCount=6, MaxMemoryCount=5, OverEvictMargin=3
	// toEvict = 6 - 5 + 3 = 4
	ids, err := ScanOverCountMemories(db, tableName, sid, config)
	require.NoError(t, err)
	assert.Len(t, ids, 4)

	// After cleaning these 4, remaining = 6 - 4 = 2, which is < MaxMemoryCount=5
	// So new writes won't immediately re-trigger (need 3 more writes to reach 6 again)
}

// --- Coordinator integration (lazy timer trigger) ---

func TestMaybeCleanup_LazyTimerTrigger(t *testing.T) {
	sessionID := "test-cleanup-coordinator-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()
	tableName := mem.entityTableName()
	sid := mem.GetSessionID()

	// Reset global coordinator state for test
	resetCleanupCoordinatorForTest()

	// Create 53 low-value, expired memories to exceed MaxMemoryCount and be expired
	for i := 0; i < 53; i++ {
		entity := &schema.AIMemoryEntity{
			MemoryID:  fmt.Sprintf("coord-mem-%d", i),
			SessionID: sid,
			Content:   fmt.Sprintf("memory %d", i),
			Tags:      schema.StringArray{"test"},
			T_Score:   0.1, // transient → expires in 7 days
			C_Score:   0.1, O_Score: 0.1, R_Score: 0.1, E_Score: 0.1, P_Score: 0.1, A_Score: 0.1,
		}
		entity.CreatedAt = time.Now().Add(-8 * 24 * time.Hour) // 8 days ago → expired
		expires := time.Now().Add(-1 * time.Hour)                // already expired
		entity.ExpiresAt = &expires
		require.NoError(t, db.Table(tableName).Create(entity).Error)
	}

	// Trigger cleanup via MaybeCleanup (first call → lastCleanupTime=0 → triggers)
	MaybeCleanup(db)

	// Wait for async cleanup to complete
	time.Sleep(3 * time.Second)

	// Verify cleanup happened: totalCount should be reduced
	var totalCount int64
	db.Table(tableName).Where("session_id = ?", sid).Count(&totalCount)

	// All 53 are expired (ExpiresAt < NOW()), so all should be cleaned
	assert.Equal(t, int64(0), totalCount, "cleanup should have evicted all expired memories")
}

func TestMaybeCleanup_RateLimit(t *testing.T) {
	sessionID := "test-cleanup-ratelimit-" + t.Name()
	mem := createCleanupTestMemory(t, sessionID)
	defer mem.Close()

	db := mem.GetDB()

	// Reset and set lastCleanupTime to now → should NOT trigger
	resetCleanupCoordinatorForTest()
	atomicStoreLastCleanupNow()

	// Call MaybeCleanup multiple times — should not trigger because within interval
	for i := 0; i < 10; i++ {
		MaybeCleanup(db)
	}

	time.Sleep(500 * time.Millisecond)

	// cleanupRunning should still be 0 (never triggered)
	assert.Equal(t, int32(0), atomic.LoadInt32(&globalCoordinator.cleanupRunning))
}

func atomicStoreLastCleanupNow() {
	atomic.StoreInt64(&globalCoordinator.lastCleanupTime, time.Now().UnixNano())
}