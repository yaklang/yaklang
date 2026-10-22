package ssadb

import (
	"database/sql"
	"time"

	"github.com/yaklang/gorm"
)

func needsDMIRTransaction(db *gorm.DB) bool {
	_, nested := db.CommonDB().(*sql.Tx)
	return db.Dialect().GetName() == "dm" && !nested
}

func inIRTransaction(db *gorm.DB, work func(*gorm.DB) error) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if err := work(tx); err != nil {
		return err
	}
	return tx.Commit().Error
}

// DM does not expose a documented consecutive-ID rule for multi-row INSERTs.
// Keep each generated identity from its own result inside the same transaction.
func createIRBatch[T any](db *gorm.DB, items []*T, size int) error {
	if db.Dialect().GetName() != "dm" {
		return db.CreateInBatches(items, size).Error
	}
	tx := db
	_, nested := db.CommonDB().(*sql.Tx)
	if !nested {
		tx = db.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if err := tx.Create(item).Error; err != nil {
			return err
		}
	}
	if !nested {
		return tx.Commit().Error
	}
	return nil
}

// DM does not implement the SQLite/PostgreSQL ON CONFLICT upsert. Preserve its
// delete/insert fallback atomically across the entire input, including chunks.
func saveDMIRCodeBatch(db *gorm.DB, items []*IrCode) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if needsDMIRTransaction(db) {
		return inIRTransaction(db, func(tx *gorm.DB) error { return saveDMIRCodeBatch(tx, items) })
	}
	clean := make([]*IrCode, 0, len(items))
	for _, it := range items {
		if it != nil {
			clean = append(clean, it)
		}
	}
	for start := 0; start < len(clean); start += irCodeBatchChunk {
		end := start + irCodeBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		if err := bulkUpsertDMIRCode(db, clean[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func bulkUpsertDMIRCode(db *gorm.DB, items []*IrCode) error {
	if len(items) == 0 {
		return nil
	}
	progCodeIDs := make(map[string][]interface{}, 1)
	for _, it := range items {
		progCodeIDs[it.ProgramName] = append(progCodeIDs[it.ProgramName], it.CodeID)
		// Fresh insert after delete: clear primary key / soft-delete state so
		// CreateInBatches allocates a new row instead of colliding on id.
		it.ID = 0
		it.CreatedAt = time.Time{}
		it.UpdatedAt = time.Time{}
		it.DeletedAt = nil
	}
	for prog, ids := range progCodeIDs {
		for i := 0; i < len(ids); i += 999 {
			end := i + 999
			if end > len(ids) {
				end = len(ids)
			}
			if err := db.Where("program_name = ? AND code_id IN (?)", prog, ids[i:end]).
				Unscoped().Delete(&IrCode{}).Error; err != nil {
				return err
			}
		}
	}
	if err := createIRBatch(db, items, irCodeBatchChunk); err != nil {
		return err
	}
	return nil
}

func saveDMIRTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if needsDMIRTransaction(db) {
		return inIRTransaction(db, func(tx *gorm.DB) error { return saveDMIRTypeBatch(tx, items) })
	}
	clean := make([]*IrType, 0, len(items))
	for _, it := range items {
		if it != nil {
			clean = append(clean, it)
		}
	}
	for start := 0; start < len(clean); start += irTypeBatchChunk {
		end := start + irTypeBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		if err := bulkUpsertDMIRType(db, clean[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// bulkUpsertDMIRType replaces logical keys inside the caller's transaction.
// Clear identity state before each insert, including structs saved previously.
func bulkUpsertDMIRType(db *gorm.DB, items []*IrType) error {
	if len(items) == 0 {
		return nil
	}
	// collect distinct (program_name, type_id) pairs to delete. type_id is the
	// logical key within a program; collect ids per program to stay safe if a
	// batch ever spans programs (it does not today, but be correct).
	progTypeIDs := make(map[string][]interface{}, 1)
	for _, it := range items {
		progTypeIDs[it.ProgramName] = append(progTypeIDs[it.ProgramName], it.TypeId)
		it.ID = 0
		it.CreatedAt = time.Time{}
		it.UpdatedAt = time.Time{}
		it.DeletedAt = nil
	}
	for prog, ids := range progTypeIDs {
		for i := 0; i < len(ids); i += 999 {
			end := i + 999
			if end > len(ids) {
				end = len(ids)
			}
			if err := db.Where("program_name = ? AND type_id IN (?)", prog, ids[i:end]).
				Unscoped().Delete(&IrType{}).Error; err != nil {
				return err
			}
		}
	}

	if err := createIRBatch(db, items, irTypeBatchChunk); err != nil {
		return err
	}
	return nil
}
