package ssadb

import (
	"sync"
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
	// Pool rows go first: an IR row must never reference a const id the
	// transaction has not written yet. Both share the caller's transaction.
	if err := flushIrConstPools(db, items); err != nil {
		return err
	}
	// Native multi-row INSERT avoids gorm's per-row field reflection. The
	// helper returns handled=false for dialects that keep the GORM path.
	if handled, err := nativeInsertIrCodes(db, items, batchSize); handled {
		if err != nil {
			return err
		}
		return nil
	}
	// The batched insert bypasses the GORM create callbacks, so record it
	// here or db-stats silently loses every bulk IR write.
	started := time.Now()
	err := db.CreateInBatches(items, batchSize).Error
	RecordDBOp(DBOpCreate, time.Since(started), err != nil)
	if err != nil {
		return err
	}
	for program, rows := range irCodeRowsByProgram(items) {
		irCodeWriteCounterFor(program).insertRows.Add(int64(rows))
	}
	return nil
}

// irCodeWriteCounter counts one program's IR code writes. A process can host
// several compile projects at the same time (a long-running backend serves
// concurrent frontends), so the mix is per program; a package-level counter
// only reports the sum of every program that ran in the process.
type irCodeWriteCounter struct {
	insertRows       atomic.Int64
	updateRows       atomic.Int64
	updateStatements atomic.Int64
}

var irCodeWriteCounters sync.Map // program name -> *irCodeWriteCounter

func irCodeWriteCounterFor(program string) *irCodeWriteCounter {
	if counter, ok := irCodeWriteCounters.Load(program); ok {
		return counter.(*irCodeWriteCounter)
	}
	counter, _ := irCodeWriteCounters.LoadOrStore(program, &irCodeWriteCounter{})
	return counter.(*irCodeWriteCounter)
}

func dropIrCodeWriteCounter(program string) {
	irCodeWriteCounters.Delete(program)
}

// irCodeRowsByProgram groups a batch by program. A batch normally belongs to
// one program; grouping keeps a mixed batch attributed to the right ones.
func irCodeRowsByProgram(items []*IrCode) map[string]int {
	counts := make(map[string]int, 1)
	for _, item := range items {
		if item == nil {
			continue
		}
		counts[item.ProgramName]++
	}
	return counts
}

// IrCodeWriteBreakdown reports the IR code write mix of one program since the
// process started: fresh rows, upserted rows, and upsert statements.
func IrCodeWriteBreakdown(program string) (insertRows, updateRows, updateStatements int64) {
	counter, ok := irCodeWriteCounters.Load(program)
	if !ok {
		return 0, 0, 0
	}
	mix := counter.(*irCodeWriteCounter)
	return mix.insertRows.Load(), mix.updateRows.Load(), mix.updateStatements.Load()
}

// ResetIrCodeWriteBreakdown clears one program's counters; used by tests and
// by a compile that wants its own breakdown.
func ResetIrCodeWriteBreakdown(program string) {
	if counter, ok := irCodeWriteCounters.Load(program); ok {
		mix := counter.(*irCodeWriteCounter)
		mix.insertRows.Store(0)
		mix.updateRows.Store(0)
		mix.updateStatements.Store(0)
	}
}
