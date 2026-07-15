package ssadb

import (
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
)

// sanitizeStringForPG removes NUL bytes (\x00) from a string.
//
// PostgreSQL rejects NUL bytes at the wire-protocol level — they never reach
// column validation, triggers, or CHECK constraints. Any Go string that
// contains a NUL byte and is sent through libpq (e.g. via GORM tx.Save) will
// fail with: "pq: invalid byte sequence for encoding \"UTF8\": 0x00".
//
// MySQL (utf8mb4) and SQLite tolerate NUL bytes, so this is a PostgreSQL-only
// concern. SSA string constants derived from source code (e.g. binary resource
// files parsed as constants) can carry NUL bytes; without stripping, the
// entire batch write aborts and the scan fails at the compile phase.
//
// NUL bytes have no semantic value in SSA IR (they are never part of valid
// source identifiers, type names, or instruction strings), so stripping is
// lossless from the perspective of vulnerability detection.
func sanitizeStringForPG(s string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// sanitizeStructStringsForPG uses reflection to strip NUL bytes from every
// string field (and every element of []string fields) on a struct. This
// automatically covers newly added string fields without manual maintenance.
//
// It handles:
//   - Top-level string fields
//   - string fields inside embedded structs (e.g. gorm.Model is NOT embedded
//     in IrNamePool, but IrCode embeds gorm.Model — gorm.Model has no string
//     fields, so it is a no-op there)
//   - Custom types whose underlying type is []string (e.g. StringSlice)
//
// It is called from BeforeSave GORM hooks on each SSA DB model.
func sanitizeStructStringsForPG(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return
	}
	sanitizeStructFieldsForPG(rv)
}

// sanitizeStructFieldsForPG walks the struct's fields recursively.
func sanitizeStructFieldsForPG(rv reflect.Value) {
	t := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		if !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.String:
			s := field.String()
			if strings.ContainsRune(s, 0) {
				field.SetString(sanitizeStringForPG(s))
			}
		case reflect.Slice:
			// Handle []string (including StringSlice which is []string)
			if field.Type().Elem().Kind() == reflect.String {
				sanitizeStringSliceForPG(field)
			}
		case reflect.Struct:
			// Recurse into embedded/anonymous structs only (avoid touching
			// non-embedded value structs that we don't own).
			if t.Field(i).Anonymous {
				sanitizeStructFieldsForPG(field)
			}
		case reflect.Ptr:
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				sanitizeStructFieldsForPG(field.Elem())
			}
		}
	}
}

// sanitizeStringSliceForPG strips NUL bytes from every element of a []string
// slice (including the StringSlice custom type).
func sanitizeStringSliceForPG(field reflect.Value) {
	if field.Type().Elem().Kind() != reflect.String {
		return
	}
	n := field.Len()
	for i := 0; i < n; i++ {
		elem := field.Index(i)
		s := elem.String()
		if strings.ContainsRune(s, 0) {
			elem.SetString(sanitizeStringForPG(s))
		}
	}
}

// BeforeSave hooks for SSA DB models.
//
// These hooks fire on every GORM write path: tx.Save, db.Save, db.Create,
// and db.Where().Assign().FirstOrCreate() (used by UpsertIrCode). They run
// before the row is sent to PostgreSQL, stripping NUL bytes that PG would
// reject at the protocol level.

func (r *IrCode) BeforeSave(tx *gorm.DB) error {
	sanitizeStructStringsForPG(r)
	return nil
}

func (t *IrType) BeforeSave(tx *gorm.DB) error {
	sanitizeStructStringsForPG(t)
	return nil
}

func (i *IrNamePool) BeforeSave(tx *gorm.DB) error {
	sanitizeStructStringsForPG(i)
	return nil
}

func (r *IrOffset) BeforeSave(tx *gorm.DB) error {
	sanitizeStructStringsForPG(r)
	return nil
}
