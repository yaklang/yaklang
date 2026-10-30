package ssadb

import (
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
)

// InsertIrCodes inserts new rows. The unique (program_name, code_id) index is
// always present, so a duplicate insert fails instead of appending a second
// row for the same instruction.
func InsertIrCodes(db *gorm.DB, items []*IrCode, batchSize int) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if batchSize < 1 {
		batchSize = len(items)
	}
	// The batched insert bypasses the GORM create callbacks, so record it
	// here or db-stats silently loses every bulk IR write.
	started := time.Now()
	err := db.CreateInBatches(items, batchSize).Error
	RecordDBOp(DBOpCreate, time.Since(started), err != nil)
	if err != nil {
		return err
	}
	irCodeInsertRows.Add(int64(len(items)))
	return nil
}

// irCodeWriteCounters splits IR code writes into fresh rows and upserts of
// rows that already exist. A compile that rewrites a large share of its
// instructions is paying for repeated saves, not new IR.
var (
	irCodeInsertRows       atomic.Int64
	irCodeUpdateRows       atomic.Int64
	irCodeUpdateStatements atomic.Int64
)

// IrCodeWriteBreakdown reports the IR code write mix since process start.
func IrCodeWriteBreakdown() (insertRows, updateRows, updateStatements int64) {
	return irCodeInsertRows.Load(), irCodeUpdateRows.Load(), irCodeUpdateStatements.Load()
}

// ResetIrCodeWriteBreakdown clears the counters; used by tests and runs that
// want a per-compile breakdown.
func ResetIrCodeWriteBreakdown() {
	irCodeInsertRows.Store(0)
	irCodeUpdateRows.Store(0)
	irCodeUpdateStatements.Store(0)
}
