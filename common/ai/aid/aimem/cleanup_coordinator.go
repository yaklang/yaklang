package aimem

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// cleanupInterval 两次清理之间的最小间隔
const cleanupInterval = 30 * time.Minute

// cleanupMinBatch 攒批阈值：扫描出的待清理条数不足此值时不做物理清理，
// 避免频繁 HNSW Load/Save
const cleanupMinBatch = 20

// cleanupCoordinator 是 DB 级别的全局清理协调器。
// 因为 AIMemoryTriage 不是单例（同一 sessionID 会被反复 NewAIMemory 创建新实例），
// 清理调度状态必须放在全局级别，而不是实例级别。
//
// 设计要点:
//   - lastCleanupTime: 全局时间戳，atomic 读写，纳秒级检查
//   - cleanupRunning:  全局 CAS 标志，同一时间只有一个清理 goroutine
//   - 惰性触发: 写入和搜索都调 MaybeCleanup，但不阻塞主路径
type cleanupCoordinator struct {
	lastCleanupTime int64 // unix nano, atomic
	cleanupRunning  int32 // 0=空闲, 1=清理中, CAS
}

// globalCoordinator 包级别单例
var globalCoordinator = &cleanupCoordinator{}

// MaybeCleanup 惰性触发清理检查。
// 在写入和搜索路径调用，开销仅一次 atomic.Load + time 比较（纳秒级），完全无感。
// 如果距上次清理不足 cleanupInterval，直接返回；否则异步触发清理。
func MaybeCleanup(db *gorm.DB) {
	if db == nil {
		return
	}

	// 距上次清理不足间隔，跳过
	last := atomic.LoadInt64(&globalCoordinator.lastCleanupTime)
	if last > 0 && time.Since(time.Unix(0, last)) < cleanupInterval {
		return
	}

	// CAS 防并发：只有 0→1 的那个调用才触发
	if !atomic.CompareAndSwapInt32(&globalCoordinator.cleanupRunning, 0, 1) {
		return
	}

	// 记录触发时间
	atomic.StoreInt64(&globalCoordinator.lastCleanupTime, time.Now().UnixNano())

	// 异步执行清理，不阻塞调用方
	go globalCoordinator.runCleanup(db)
}

// runCleanup 执行一次完整的清理流程：扫描所有 session → 攒批判断 → 物理清理。
func (c *cleanupCoordinator) runCleanup(db *gorm.DB) {
	defer atomic.StoreInt32(&c.cleanupRunning, 0)

	config := DefaultCleanupConfig()
	ctx := context.Background()

	// 长期记忆表，不含 midterm
	tableName := "ai_memory_entities_v1"

	// 获取所有需要清理的 sessionID
	// 目前长期记忆基本都走 "default"，但为了覆盖性，扫描所有 distinct session_id
	var sessionIDs []string
	if err := db.Table(tableName).Select("DISTINCT(session_id)").Pluck("session_id", &sessionIDs).Error; err != nil {
		log.Warnf("cleanup: failed to get distinct session_ids: %v", err)
		return
	}

	for _, sid := range sessionIDs {
		c.cleanupSession(ctx, db, tableName, sid, config)
	}
}

// cleanupSession 清理单个 session 的过期/低价值/超量记忆。
func (c *cleanupCoordinator) cleanupSession(ctx context.Context, db *gorm.DB, tableName, sessionID string, config CleanupConfig) {
	// 1. 扫描所有待清理的记忆 ID（过期 + 低价值 + 超量）
	ids, err := ScanAllCleanupMemories(db, tableName, sessionID, config)
	if err != nil {
		log.Warnf("cleanup scan failed for session %s: %v", sessionID, err)
		return
	}

	if len(ids) == 0 {
		return
	}

	// 2. 攒批阈值：待清理数量太少时不做物理清理，攒着下次一起删
	if len(ids) < cleanupMinBatch {
		log.Debugf("cleanup skipped for session %s: only %d candidates (< %d), will batch next time",
			sessionID, len(ids), cleanupMinBatch)
		return
	}

	// 限制单次清理数量
	maxBatch := config.MaxBatchSize
	if maxBatch <= 0 {
		maxBatch = 100
	}
	if len(ids) > maxBatch {
		ids = ids[:maxBatch]
	}

	// 3. 执行物理清理（HNSW + RAG + DB）
	log.Infof("cleanup started for session %s: %d memories to clean", sessionID, len(ids))
	if err := BatchCleanupMemories(ctx, db, sessionID, ids); err != nil {
		log.Warnf("cleanup batch delete failed for session %s: %v", sessionID, err)
		return
	}
	log.Infof("cleanup completed for session %s: %d memories cleaned", sessionID, len(ids))
}

// resetCleanupCoordinatorForTest 重置全局协调器状态，仅供测试使用。
func resetCleanupCoordinatorForTest() {
	atomic.StoreInt64(&globalCoordinator.lastCleanupTime, 0)
	atomic.StoreInt32(&globalCoordinator.cleanupRunning, 0)
}