package schema

import (
	"strings"

	"github.com/jinzhu/gorm"
)

func IsSQLite(db *gorm.DB) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	return strings.Contains(strings.ToLower(db.Dialect().GetName()), "sqlite")
}

// ResetSQLiteSequence 仅重置 SQLite 自增序列（表名需与 SQLITE_SEQUENCE 中一致）。非 SQLite、无表记录或 sqlite_sequence 表不存在时忽略。
func ResetSQLiteSequence(db *gorm.DB, tableName string) error {
	if db == nil || tableName == "" || !IsSQLite(db) {
		return nil
	}
	err := db.Exec("UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME=?", tableName).Error
	if err != nil && (strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "SQLITE_SEQUENCE")) {
		return nil
	}
	return err
}

// DropRecreateTable 删除表、重置 SQLite 自增序列并重新建表，供需要「清空并重建」单表的场景复用。
func DropRecreateTable(db *gorm.DB, model interface{}) error {
	if db == nil {
		return nil
	}
	// 表名从 model 的 struct/TableName() 取得，与表是否已存在无关，需在 Drop 前拿到供后续重置序列用
	tableName := db.NewScope(model).TableName()
	db.DropTableIfExists(model)
	if err := db.Error; err != nil {
		return err
	}
	if err := ResetSQLiteSequence(db, tableName); err != nil {
		return err
	}
	return db.AutoMigrate(model).Error
}
