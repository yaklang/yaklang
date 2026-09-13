package aimem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const cleanupInterval = 30 * time.Minute
const cleanupSessionPageSize = 32
const cleanupRunTimeout = 10 * time.Second
const cleanupStateKey = "aimemory-cleanup-v2"

// A single worker bounds process-wide contention. The cooldown is scoped to
// the database and persisted there; restarting must not reset the interval.
// Only one database pointer is retained, rather than an unbounded DB registry.
type cleanupCoordinator struct {
	lastCleanupTime int64
	cleanupRunning  int32
	lastDB          atomic.Pointer[sql.DB]
}

var globalCoordinator = &cleanupCoordinator{}

func MaybeCleanup(db *gorm.DB) {
	if db == nil {
		return
	}
	rawDB, ok := db.CommonDB().(*sql.DB)
	// Never schedule maintenance on a caller-owned transaction.
	if !ok {
		return
	}
	if globalCoordinator.lastDB.Load() == rawDB {
		last := atomic.LoadInt64(&globalCoordinator.lastCleanupTime)
		if last > 0 && time.Since(time.Unix(0, last)) < cleanupInterval {
			return
		}
	}
	if !atomic.CompareAndSwapInt32(&globalCoordinator.cleanupRunning, 0, 1) {
		return
	}
	globalCoordinator.lastDB.Store(rawDB)
	atomic.StoreInt64(&globalCoordinator.lastCleanupTime, time.Now().UnixNano())
	go globalCoordinator.runCleanup(db)
}

type cleanupState struct {
	StartedAt time.Time `json:"started_at"`
	SessionID string    `json:"session_id"`
}

// Claim before doing expensive work, including on empty databases. A crash
// leaves a durable cooldown; failures retain their entity/document mappings
// for retry after that interval. Concurrent processes cannot both commit a
// claim on the same SQLite database.
func claimCleanup(ctx context.Context, db *gorm.DB, now time.Time) (cleanupState, bool, error) {
	tx := db.BeginTx(ctx, nil)
	if tx.Error != nil {
		return cleanupState{}, false, tx.Error
	}
	defer tx.Rollback()
	key := strconv.Quote(cleanupStateKey)
	var row schema.ProjectGeneralStorage
	var state cleanupState
	err := tx.Where("key = ?", key).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, false, err
	}
	if row.Value != "" {
		value, unquoteErr := strconv.Unquote(row.Value)
		if unquoteErr != nil {
			value = row.Value
		}
		if err := json.Unmarshal([]byte(value), &state); err != nil {
			// A corrupt checkpoint must not disable maintenance permanently.
			state = cleanupState{}
		}
		elapsed := now.Sub(state.StartedAt)
		// Ignore a far-future timestamp after clock correction or a bad import.
		if elapsed > -cleanupInterval && elapsed < cleanupInterval {
			return state, false, nil
		}
	}
	state.StartedAt = now
	value, err := json.Marshal(state)
	if err != nil {
		return state, false, err
	}
	if row.ID == 0 {
		row.Key, row.Value = key, strconv.Quote(string(value))
		err = tx.Create(&row).Error
	} else {
		err = tx.Model(&row).Update("value", strconv.Quote(string(value))).Error
	}
	if err != nil {
		return state, false, err
	}
	if err := tx.Commit().Error; err != nil {
		return state, false, err
	}
	return state, true, nil
}

func (c *cleanupCoordinator) runCleanup(db *gorm.DB) {
	defer atomic.StoreInt32(&c.cleanupRunning, 0)
	ctx, cancel := context.WithTimeout(context.Background(), cleanupRunTimeout)
	defer cancel()
	state, claimed, err := claimCleanup(ctx, db, time.Now())
	if err != nil {
		log.Warnf("cleanup: failed to claim database maintenance: %v", err)
		return
	}
	if !claimed {
		return
	}
	config := DefaultCleanupConfig()
	const tableName = "ai_memory_entities_v1"
	// Keyset pagination bounds session metadata and makes progress through
	// imported databases containing thousands of sessions across runs.
	var sessionIDs []string
	if err := db.Table(tableName).Select("session_id").
		Where("session_id > ? AND deleted_at IS NULL", state.SessionID).
		Group("session_id").Order("session_id").Limit(cleanupSessionPageSize).
		Pluck("session_id", &sessionIDs).Error; err != nil {
		log.Warnf("cleanup: failed to get session page: %v", err)
		return
	}
	completed := true
	for _, sid := range sessionIDs {
		if ctx.Err() != nil {
			completed = false
			break
		}
		c.cleanupSession(ctx, db, tableName, sid, config)
		state.SessionID = sid
	}
	if completed && len(sessionIDs) < cleanupSessionPageSize {
		state.SessionID = ""
	}
	value, _ := json.Marshal(state)
	if err := db.Model(&schema.ProjectGeneralStorage{}).
		Where("key = ?", strconv.Quote(cleanupStateKey)).Update("value", strconv.Quote(string(value))).Error; err != nil {
		log.Warnf("cleanup: failed to checkpoint session page: %v", err)
	}
}

func (c *cleanupCoordinator) cleanupSession(ctx context.Context, db *gorm.DB, tableName, sessionID string, config CleanupConfig) {
	if ctx.Err() != nil {
		return
	}
	ids, err := ScanAllCleanupMemories(db, tableName, sessionID, config)
	if err != nil {
		log.Warnf("cleanup scan failed for session %s: %v", sessionID, err)
		return
	}
	if len(ids) == 0 || ctx.Err() != nil {
		return
	}
	started := time.Now()
	if err := BatchCleanupMemories(ctx, db, sessionID, ids); err != nil {
		log.Warnf("cleanup batch delete failed for session %s: %v", sessionID, err)
		return
	}
	log.Infof("cleanup completed for session %s: %d memories cleaned in %v", sessionID, len(ids), time.Since(started))
}

func resetCleanupCoordinatorForTest() {
	atomic.StoreInt64(&globalCoordinator.lastCleanupTime, 0)
	atomic.StoreInt32(&globalCoordinator.cleanupRunning, 0)
	globalCoordinator.lastDB.Store(nil)
}
