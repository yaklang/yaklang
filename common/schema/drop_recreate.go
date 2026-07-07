package schema

import (
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func IsSQLite(db *gorm.DB) bool {
	if db == nil || db.Dialector == nil {
		return false
	}
	return db.Dialector.Name() == "sqlite"
}

// GormTableName 通过 GORM V2 的 Statement 解析 model 对应的表名，替代 V1 的 db.NewScope(model).TableName()。
func GormTableName(db *gorm.DB, model interface{}) string {
	if db == nil {
		return ""
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return ""
	}
	return stmt.Table
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
	parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return err
	}
	tableName := parsed.Table
	if err := db.Migrator().DropTable(model); err != nil {
		return err
	}
	if err := ResetSQLiteSequence(db, tableName); err != nil {
		return err
	}
	return db.Migrator().AutoMigrate(model)
}
