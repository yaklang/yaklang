package yakit

import (
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

const (
	dbSaveBatchMaxSize        = 48
	dbSaveBatchCoalesceWait   = 10 * time.Millisecond
	dbSaveSlowInsertThreshold = 3 * time.Second
)

// drainDBSaveBatch pulls more queued writers when the channel is busy.
func drainDBSaveBatch(first DbExecFunc) []DbExecFunc {
	batch := make([]DbExecFunc, 0, dbSaveBatchMaxSize)
	batch = append(batch, first)

	timer := time.NewTimer(dbSaveBatchCoalesceWait)
	defer timer.Stop()

	for len(batch) < dbSaveBatchMaxSize {
		select {
		case f := <-DBSaveAsyncChannel:
			batch = append(batch, f)
		case <-timer.C:
			return batch
		}
	}
	return batch
}

func execDBSaveFunc(db *gorm.DB, f DbExecFunc) error {
	start := time.Now()
	err := f(db)
	recordSlowDBSaveIfNeeded(time.Since(start), f)
	return err
}

func execDBSaveBatch(db *gorm.DB, batch []DbExecFunc) {
	// Sequential autocommit per item: coalesce scheduling without holding one SQLite write
	// transaction across the whole batch (avoids database is locked under MaxOpenConns(1)).
	for _, f := range batch {
		if err := execDBSaveFunc(db, f); err != nil {
			log.Errorf("Throttle sql exec failed: %s", err)
		}
	}
}

func recordSlowDBSaveIfNeeded(elapsed time.Duration, f DbExecFunc) {
	if elapsed <= dbSaveSlowInsertThreshold {
		return
	}

	sqliteLargeDBTuneThrottle(func() {
		consts.TuneSQLiteByDatabaseFileSize(consts.GetGormProjectDatabase(), consts.GetCurrentProjectDatabasePath())
	})

	ptr := reflect.ValueOf(f).Pointer()
	fn := runtime.FuncForPC(ptr)
	fnName := "<unknown>"
	if fn != nil {
		fnName = fn.Name()
	}
	log.Warnf("SQL execution took too long: %v, func_ptr:%p, func_name:%s, queue_len:%d",
		elapsed, f, fnName, len(DBSaveAsyncChannel))

	now := time.Now()
	slowSQLItem := &LongSQLDescription{
		Duration:      elapsed,
		DurationMs:    elapsed.Milliseconds(),
		DurationStr:   elapsed.String(),
		FuncName:      fnName,
		FuncPtr:       fmt.Sprintf("%p", f),
		QueueLen:      len(DBSaveAsyncChannel),
		LastSQL:       "",
		Timestamp:     now,
		TimestampUnix: now.Unix(),
		DatabasePath:  consts.GetCurrentProjectDatabasePath(),
	}

	slowInsertSQLItemsMutex.Lock()
	slowInsertSQLItems = append(slowInsertSQLItems, slowSQLItem)
	slowInsertSQLItemsMutex.Unlock()

	slowInsertSQLThrottle(func() {
		go triggerSlowInsertSQLCallback()
	})
}
