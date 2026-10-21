// Package damenggorm supplies an explicit DM8 dialect for the engine's GORM v1
// SSA store. It does not change the SQLite or PostgreSQL runtime paths.
package damenggorm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

const Dialect = "dm"

func init() { gorm.RegisterDialect(Dialect, &dmDialect{}) }

type dmDialect struct {
	db gorm.SQLCommon
	gorm.DefaultForeignKeyNamer
}

func (*dmDialect) GetName() string           { return Dialect }
func (d *dmDialect) SetDB(db gorm.SQLCommon) { d.db = db }
func (*dmDialect) BindVar(int) string        { return "?" }
func (*dmDialect) Quote(name string) string {
	return `"` + strings.ReplaceAll(strings.ToUpper(name), `"`, `""`) + `"`
}

func (d *dmDialect) DataTypeOf(field *gorm.StructField) string {
	value, typ, size, extra := gorm.ParseFieldStructForDialect(field, d)
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "text", "longtext", "mediumtext", "json", "jsonb":
		typ = "CLOB"
	case "blob", "bytea":
		typ = "BLOB"
	case "bool", "boolean":
		typ = "BIT"
	}
	if typ == "" {
		switch value.Kind() {
		case reflect.Bool:
			typ = "BIT"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			typ = "BIGINT"
			increment, explicit := field.TagSettingsGet("AUTO_INCREMENT")
			if (field.IsPrimaryKey && !explicit) || (explicit && !strings.EqualFold(increment, "false")) {
				typ += " IDENTITY(1,1)"
			}
		case reflect.Float32, reflect.Float64:
			typ = "DOUBLE"
		case reflect.String:
			_, sized := field.TagSettingsGet("SIZE")
			_, indexed := field.TagSettingsGet("INDEX")
			_, uniqueIndex := field.TagSettingsGet("UNIQUE_INDEX")
			_, unique := field.TagSettingsGet("UNIQUE")
			// Every payload table is keyed and cleaned by program_name. Some
			// composite indexes are installed by the SSA schema patch.
			if field.DBName == "program_name" {
				indexed = true
			}
			if !sized && !indexed && !uniqueIndex && !unique && !field.IsPrimaryKey {
				typ = "CLOB"
			} else {
				if !sized || size <= 0 {
					size = 1024
				}
				typ = fmt.Sprintf("VARCHAR(%d)", size)
			}
		case reflect.Struct:
			if value.Type() == reflect.TypeOf(time.Time{}) {
				typ = "TIMESTAMP(6)"
			}
		case reflect.Slice, reflect.Array:
			if value.Type().Elem().Kind() == reflect.Uint8 {
				typ = "BLOB"
			}
		}
	}
	if typ == "" {
		panic(fmt.Sprintf("unsupported DM SSA field %s (%s)", field.Name, value.Kind()))
	}
	return strings.TrimSpace(typ + " " + extra)
}

func (d *dmDialect) count(query string, args ...interface{}) bool {
	var count int64
	return d.db.QueryRow(query, args...).Scan(&count) == nil && count > 0
}
func (d *dmDialect) HasTable(table string) bool {
	return d.count(`SELECT COUNT(*) FROM ALL_TABLES WHERE OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND TABLE_NAME=?`, strings.ToUpper(table))
}
func (d *dmDialect) HasColumn(table, column string) bool {
	return d.count(`SELECT COUNT(*) FROM ALL_TAB_COLUMNS WHERE OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND TABLE_NAME=? AND COLUMN_NAME=?`, strings.ToUpper(table), strings.ToUpper(column))
}
func (d *dmDialect) HasIndex(table, index string) bool {
	return d.count(`SELECT COUNT(*) FROM ALL_INDEXES WHERE OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND TABLE_NAME=? AND INDEX_NAME=?`, strings.ToUpper(table), strings.ToUpper(index))
}
func (d *dmDialect) HasForeignKey(table, key string) bool {
	return d.count(`SELECT COUNT(*) FROM ALL_CONSTRAINTS WHERE OWNER=SYS_CONTEXT('USERENV','CURRENT_SCHEMA') AND TABLE_NAME=? AND CONSTRAINT_NAME=? AND CONSTRAINT_TYPE='R'`, strings.ToUpper(table), strings.ToUpper(key))
}
func (d *dmDialect) RemoveIndex(_ string, index string) error {
	_, err := d.db.Exec("DROP INDEX " + d.Quote(index))
	return err
}
func (d *dmDialect) ModifyColumn(table, column, typ string) error {
	_, err := d.db.Exec("ALTER TABLE " + d.Quote(table) + " MODIFY " + d.Quote(column) + " " + typ)
	return err
}
func (d *dmDialect) CurrentDatabase() string {
	var schema string
	_ = d.db.QueryRow(`SELECT SYS_CONTEXT('USERENV','CURRENT_SCHEMA') FROM DUAL`).Scan(&schema)
	return schema
}
func (*dmDialect) LimitAndOffsetSQL(limit, offset interface{}) (string, error) {
	var sql string
	for _, entry := range []struct {
		name  string
		value interface{}
	}{{"LIMIT", limit}, {"OFFSET", offset}} {
		if entry.value == nil {
			continue
		}
		n, err := strconv.ParseInt(fmt.Sprint(entry.value), 10, 64)
		if err != nil {
			return "", err
		}
		if n >= 0 {
			sql += fmt.Sprintf(" %s %d", entry.name, n)
		}
	}
	return sql, nil
}
func (*dmDialect) SelectFromDummyTable() string                                   { return " FROM DUAL" }
func (*dmDialect) LastInsertIDOutputInterstitial(string, string, []string) string { return "" }
func (*dmDialect) LastInsertIDReturningSuffix(string, string) string              { return "" }
func (*dmDialect) DefaultValueStr() string                                        { return "DEFAULT VALUES" }
func (*dmDialect) NormalizeIndexAndColumn(index, column string) (string, string) {
	return strings.ToUpper(index), strings.ToUpper(column)
}
