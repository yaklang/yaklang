// Package damenggorm owns the SSA store's naming and field-size policy for DM.
// Generic SQL, batch operations and the optional native driver are supplied by
// github.com/yaklang/gorm/dialects/dameng.
package damenggorm

import (
	"reflect"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/gorm/dialects/dameng"
)

const (
	// Dialect is the connection URL scheme and native driver's name.
	Dialect = dameng.Dialect
	// ORMName keeps SSA policy in place when GORM clones a query or transaction.
	ORMName = "dm_ssa"
)

var genericDialectType reflect.Type

func init() {
	prototype, ok := gorm.GetDialect(Dialect)
	if !ok {
		panic("DM dialect is not registered")
	}
	genericDialectType = reflect.TypeOf(prototype).Elem()
	gorm.RegisterDialect(ORMName, &ssaDialect{})
}

// IsDialect accepts the generic DM dialect and the SSA policy alias.
func IsDialect(name string) bool { return name == Dialect || name == ORMName }

func IsDM(db *gorm.DB) bool { return db != nil && IsDialect(db.Dialect().GetName()) }

type ssaDialect struct{ gorm.Dialect }

var (
	_ gorm.Dialect            = (*ssaDialect)(nil)
	_ gorm.BatchCreateDialect = (*ssaDialect)(nil)
	_ gorm.BatchSaveDialect   = (*ssaDialect)(nil)
)

func (*ssaDialect) GetName() string { return ORMName }

func (d *ssaDialect) SetDB(db gorm.SQLCommon) {
	// Mirror GORM's dialect cloning without opening or pinging another pool.
	d.Dialect = reflect.New(genericDialectType).Interface().(gorm.Dialect)
	d.Dialect.SetDB(db)
}

func (d *ssaDialect) Quote(name string) string {
	return d.Dialect.Quote(strings.ToUpper(name))
}

func (d *ssaDialect) DataTypeOf(field *gorm.StructField) string {
	value, typ, size, _ := gorm.ParseFieldStructForDialect(field, d)
	if typ != "" || value.Kind() != reflect.String {
		return d.Dialect.DataTypeOf(field)
	}

	_, sized := field.TagSettingsGet("SIZE")
	_, indexed := field.TagSettingsGet("INDEX")
	_, uniqueIndex := field.TagSettingsGet("UNIQUE_INDEX")
	_, unique := field.TagSettingsGet("UNIQUE")
	// Composite program indexes are installed by SSA migrations rather than
	// every model tag. Their key must remain a bounded, indexable string.
	indexed = indexed || field.DBName == "program_name"
	if sized && size > 0 {
		return d.Dialect.DataTypeOf(field)
	}
	policy := fieldForDataType(field)
	if !sized && !indexed && !uniqueIndex && !unique && !field.IsPrimaryKey {
		policy.TagSettings["TYPE"] = "text"
	} else {
		policy.TagSettings["SIZE"] = "1024"
	}
	return d.Dialect.DataTypeOf(policy)
}

// StructField contains a private mutex. Copy only metadata used by GORM's type
// parser, reading cached tags through its synchronized accessor. The original
// model remains valid for simultaneous SQLite/PostgreSQL use.
func fieldForDataType(field *gorm.StructField) *gorm.StructField {
	cloned := &gorm.StructField{
		DBName: field.DBName, Name: field.Name, Struct: field.Struct,
		IsPrimaryKey: field.IsPrimaryKey, TagSettings: make(map[string]string),
	}
	for _, key := range []string{"TYPE", "SIZE", "AUTO_INCREMENT", "NOT NULL", "UNIQUE", "DEFAULT", "COMMENT"} {
		if value, ok := field.TagSettingsGet(key); ok {
			cloned.TagSettings[key] = value
		}
	}
	return cloned
}

func (d *ssaDialect) HasTable(table string) bool {
	return d.Dialect.HasTable(strings.ToUpper(table))
}
func (d *ssaDialect) HasColumn(table, column string) bool {
	return d.Dialect.HasColumn(strings.ToUpper(table), strings.ToUpper(column))
}
func (d *ssaDialect) HasIndex(table, index string) bool {
	return d.Dialect.HasIndex(strings.ToUpper(table), strings.ToUpper(index))
}
func (d *ssaDialect) HasForeignKey(table, key string) bool {
	return d.Dialect.HasForeignKey(strings.ToUpper(table), strings.ToUpper(key))
}
func (d *ssaDialect) RemoveIndex(table, index string) error {
	return d.Dialect.RemoveIndex(strings.ToUpper(table), strings.ToUpper(index))
}
func (d *ssaDialect) NormalizeIndexAndColumn(index, column string) (string, string) {
	return d.Dialect.NormalizeIndexAndColumn(strings.ToUpper(index), strings.ToUpper(column))
}
func (d *ssaDialect) BatchCreateMaxRows() int {
	return d.Dialect.(gorm.BatchCreateDialect).BatchCreateMaxRows()
}
func (d *ssaDialect) BatchSaveSQL(table string, columns, conflicts, updates []string, values [][]string) (string, error) {
	return d.Dialect.(gorm.BatchSaveDialect).BatchSaveSQL(table, columns, conflicts, updates, values)
}
