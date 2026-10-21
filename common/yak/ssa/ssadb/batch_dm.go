package ssadb

import (
	"database/sql"

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
		if err := tx.Create(item).Error; err != nil {
			return err
		}
	}
	if !nested {
		return tx.Commit().Error
	}
	return nil
}
