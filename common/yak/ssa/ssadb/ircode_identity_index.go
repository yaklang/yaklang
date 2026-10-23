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
	return db.CreateInBatches(items, batchSize).Error
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
	}
	for _, item := range updates {
		if err := db.Save(item).Error; err != nil {
			return fmt.Errorf("update ir code %d by rowid: %w", item.CodeID, err)
		}
	}
	if _, inTx := db.CommonDB().(*sql.Tx); inTx {
		return nil
	}
	return NoteIrCodeRowIDs(items)
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
