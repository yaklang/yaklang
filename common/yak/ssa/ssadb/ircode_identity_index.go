package ssadb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
)

// irCodeIdentityIndexDDL is the compile-time lookup key on ir_codes. Keeping it
// while rows arrive out of code_id order rewrites B-tree pages for every
// insert. An empty table can drop it, remember the rowid of each new row, and
// build the index once after the load.
const irCodeIdentityIndexDDL = `CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_codes_program_code" ON "` + TableIrCodes + `" ("program_name", "code_id")`

const irCodeIdentityIndexName = "ux_ir_codes_program_code"

// irCodeIdentityIDLimit refuses a sparse code id that would allocate a huge
// rowid table. Hue's ids are dense and well under this.
const irCodeIdentityIDLimit int64 = 32 << 20

var irCodeIdentitySuspended atomic.Bool

type irCodeRowIDTable struct {
	mu     sync.RWMutex
	rowIDs []int64
}

var irCodeRowIDTables sync.Map // program name -> *irCodeRowIDTable

// IrCodeIdentityIndexSuspended reports whether code_id lookups must use the
// in-memory rowid table. Callers query by rowid until the unique index returns.
func IrCodeIdentityIndexSuspended() bool {
	return irCodeIdentitySuspended.Load()
}

// SuspendIrCodeIdentityIndex drops ux_ir_codes_program_code when ir_codes has
// no rows. A non-empty table keeps the index so other programs can still be
// looked up by code_id. The returned DDL is empty when nothing was suspended.
func SuspendIrCodeIdentityIndex(db *gorm.DB) (string, error) {
	if !isSQLiteDB(db) || db.DB() == nil {
		return "", nil
	}
	var one int
	err := db.Raw(`SELECT 1 FROM ` + quoteSQLIdent(TableIrCodes) + ` LIMIT 1`).Row().Scan(&one)
	if err == nil {
		log.Infof("kept ir_codes identity index: table is not empty")
		return "", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if hasNamedIndex(db, TableIrCodes, irCodeIdentityIndexName) {
		if err := db.Exec(`DROP INDEX ` + quoteSQLIdent(irCodeIdentityIndexName)).Error; err != nil {
			return "", fmt.Errorf("drop %s: %w", irCodeIdentityIndexName, err)
		}
	}
	clearIrCodeRowIDs()
	irCodeIdentitySuspended.Store(true)
	log.Infof("suspended ir_codes identity index for bulk insert")
	return irCodeIdentityIndexDDL, nil
}

// RestoreIrCodeIdentityIndex rebuilds the unique code_id index and drops the
// rowid table. An empty DDL is a no-op.
func RestoreIrCodeIdentityIndex(db *gorm.DB, ddl string) error {
	if strings.TrimSpace(ddl) == "" {
		return nil
	}
	if !isSQLiteDB(db) || db.DB() == nil {
		return fmt.Errorf("restore ir_codes identity index: sqlite connection is closed")
	}
	started := time.Now()
	if err := db.Exec(ddl).Error; err != nil {
		return fmt.Errorf("restore ir_codes identity index: %w", err)
	}
	clearIrCodeRowIDs()
	irCodeIdentitySuspended.Store(false)
	log.Infof("restored ir_codes identity index in %s", time.Since(started))
	return nil
}

// NoteIrCodeRowIDs records sqlite rowids assigned by a committed insert.
// It is a no-op unless the identity index is suspended.
func NoteIrCodeRowIDs(items []*IrCode) error {
	if !irCodeIdentitySuspended.Load() {
		return nil
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.ID == 0 || item.CodeID <= 0 {
			return fmt.Errorf("ir code insert did not return a rowid for code_id %d", item.CodeID)
		}
		if err := rememberIrCodeRowID(item.ProgramName, item.CodeID, int64(item.ID)); err != nil {
			return err
		}
	}
	return nil
}

// InsertIrCodes inserts new rows. The caller notes rowids only after the
// surrounding transaction commits.
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
	return err
}

// irCodeWriteCounters splits IR code writes into fresh rows and rewrites of
// rows the identity rowid table already knows. A compile that rewrites a
// large share of its instructions is paying for repeated saves, not new IR.
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

func saveIrCodesWithoutIdentityIndex(db *gorm.DB, items []*IrCode) error {
	inserts := make([]*IrCode, 0, len(items))
	updates := make([]*IrCode, 0)
	for _, item := range items {
		if item == nil {
			continue
		}
		if rowID, ok := lookupIrCodeRowID(item.ProgramName, item.CodeID); ok {
			item.ID = uint(rowID)
			updates = append(updates, item)
			continue
		}
		inserts = append(inserts, item)
	}
	if len(inserts) > 0 {
		if err := InsertIrCodes(db, inserts, irCodeBatchChunk); err != nil {
			return err
		}
		irCodeInsertRows.Add(int64(len(inserts)))
	}
	if len(updates) > 0 {
		if err := saveIrCodeUpdatesByRowID(db, updates); err != nil {
			return err
		}
	}
	if _, inTx := db.CommonDB().(*sql.Tx); inTx {
		return nil
	}
	return NoteIrCodeRowIDs(items)
}

// saveIrCodeUpdatesByRowID rewrites rows the rowid table already knows, keyed
// by primary key. The old path issued one db.Save per row, so every revisited
// instruction paid its own transaction; on Hue that was 1.7ms per row. The
// identity index is suspended here, which makes the primary key the conflict
// target, so a chunk of rows travels in one upsert statement.
func saveIrCodeUpdatesByRowID(db *gorm.DB, updates []*IrCode) error {
	for start := 0; start < len(updates); start += irCodeBatchChunk {
		end := start + irCodeBatchChunk
		if end > len(updates) {
			end = len(updates)
		}
		chunk := dedupeIrCodeRowIDs(updates[start:end])
		if len(chunk) == 0 {
			continue
		}
		started := time.Now()
		err := db.SaveInBatches(chunk, irCodeBatchChunk).Error
		RecordDBOp(DBOpUpdate, time.Since(started), err != nil)
		irCodeUpdateStatements.Add(1)
		irCodeUpdateRows.Add(int64(len(chunk)))
		if err != nil {
			return fmt.Errorf("update ir code batch by rowid: %w", err)
		}
	}
	return nil
}

// dedupeIrCodeRowIDs keeps the last row for a repeated rowid. A chunk that
// names the same conflict key twice is rejected by PostgreSQL, and SQLite
// applies the later row anyway, so dropping the earlier one preserves the
// sequential-save result.
func dedupeIrCodeRowIDs(items []*IrCode) []*IrCode {
	seen := make(map[uint]int, len(items))
	out := items[:0]
	for _, item := range items {
		if item == nil {
			continue
		}
		if index, ok := seen[item.ID]; ok {
			out[index] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func rememberIrCodeRowID(program string, codeID, rowID int64) error {
	if program == "" || codeID <= 0 || rowID <= 0 {
		return fmt.Errorf("invalid ir code rowid program=%q code_id=%d rowid=%d", program, codeID, rowID)
	}
	if codeID > irCodeIdentityIDLimit {
		return fmt.Errorf("ir code id %d exceeds the identity rowid table", codeID)
	}
	table := irCodeRowIDTableFor(program)
	table.mu.Lock()
	defer table.mu.Unlock()
	if int(codeID) >= len(table.rowIDs) {
		nextLen := len(table.rowIDs)
		if nextLen < 1024 {
			nextLen = 1024
		}
		for nextLen <= int(codeID) {
			nextLen *= 2
		}
		next := make([]int64, nextLen)
		copy(next, table.rowIDs)
		table.rowIDs = next
	}
	table.rowIDs[codeID] = rowID
	return nil
}

func lookupIrCodeRowID(program string, codeID int64) (int64, bool) {
	if program == "" || codeID <= 0 {
		return 0, false
	}
	value, ok := irCodeRowIDTables.Load(program)
	if !ok {
		return 0, false
	}
	table := value.(*irCodeRowIDTable)
	table.mu.RLock()
	defer table.mu.RUnlock()
	if int(codeID) >= len(table.rowIDs) {
		return 0, false
	}
	rowID := table.rowIDs[codeID]
	return rowID, rowID > 0
}

func lookupIrCodeRowIDs(program string, codeIDs []int64) []int64 {
	rowIDs := make([]int64, 0, len(codeIDs))
	for _, codeID := range codeIDs {
		rowID, ok := lookupIrCodeRowID(program, codeID)
		if ok {
			rowIDs = append(rowIDs, rowID)
		}
	}
	return rowIDs
}

func irCodeRowIDTableFor(program string) *irCodeRowIDTable {
	if value, ok := irCodeRowIDTables.Load(program); ok {
		return value.(*irCodeRowIDTable)
	}
	table := &irCodeRowIDTable{}
	actual, _ := irCodeRowIDTables.LoadOrStore(program, table)
	return actual.(*irCodeRowIDTable)
}

func clearIrCodeRowIDs() {
	irCodeRowIDTables.Range(func(key, _ any) bool {
		irCodeRowIDTables.Delete(key)
		return true
	})
}
