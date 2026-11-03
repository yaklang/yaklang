package ssadb

import (
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/gorm"
)

// sanitizeStringForPG removes bytes that PostgreSQL rejects at the
// wire-protocol level from a string. Two classes of bytes are stripped:
//
//  1. NUL bytes (\x00) — PG aborts with
//     `pq: invalid byte sequence for encoding "UTF8": 0x00`.
//  2. Invalid UTF-8 byte sequences — PG aborts with the same error family,
//     e.g. `pq: invalid byte sequence for encoding "UTF8": 0xa0` or
//     `... 0xe8 0x07 0x10`. A lone continuation byte (0xa0) or a 3-byte
//     leader (0xe8) followed by non-continuation bytes (0x07 0x10) are not
//     valid UTF-8 and PG rejects the whole batch.
//
// Both failures abort the entire dbcache save batch, which on a large project
// compile leaves hundreds of thousands of resident IR items unpersisted and
// fails the scan at the compile phase with `resident items were not persisted
// on close`.
//
// MySQL (utf8mb4) and SQLite tolerate these bytes, so this is a
// PostgreSQL-only concern. SSA string constants derived from source code
// (e.g. binary resource files parsed as constants, non-UTF-8 source files)
// can carry these bytes; stripping them is lossless from the perspective of
// vulnerability detection — invalid bytes are never part of valid source
// identifiers, type names, or instruction strings, and valid multi-byte UTF-8
// content (CJK, emoji) is preserved by strings.ToValidUTF8.
//
// Invalid bytes are dropped (replaced with the empty string, not U+FFFD) to
// keep IR string equality stable across re-compiles.
func sanitizeStringForPG(s string) string {
	if !strings.ContainsRune(s, 0) && utf8.ValidString(s) {
		return s
	}
	// strings.ToValidUTF8 replaces each invalid byte sequence with the given
	// replacement; "" drops the invalid bytes entirely. It does NOT remove
	// NUL bytes (NUL is a valid 1-byte UTF-8 rune), so strip NUL explicitly.
	cleaned := strings.ToValidUTF8(s, "")
	if strings.ContainsRune(cleaned, 0) {
		cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	}
	return cleaned
}

// needsSanitize reports whether a string contains bytes PG would reject
// (NUL or invalid UTF-8). Used as a fast path so clean strings — the common
// case — are not copied.
func needsSanitize(s string) bool {
	return strings.ContainsRune(s, 0) || !utf8.ValidString(s)
}

// sanitizeStructStringsForPG uses reflection to strip PG-rejected bytes from
// every string field (and every element of []string fields) on a struct. This
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
			if needsSanitize(s) {
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

// sanitizeStringSliceForPG strips PG-rejected bytes from every element of a
// []string slice (including the StringSlice custom type).
func sanitizeStringSliceForPG(field reflect.Value) {
	if field.Type().Elem().Kind() != reflect.String {
		return
	}
	n := field.Len()
	for i := 0; i < n; i++ {
		elem := field.Index(i)
		s := elem.String()
		if needsSanitize(s) {
			elem.SetString(sanitizeStringForPG(s))
		}
	}
}

// BeforeSave hooks for SSA DB models.
//
// These hooks fire on every GORM write path: tx.Save, db.Save, db.Create,
// and db.Where().Assign().FirstOrCreate() (used by UpsertIrCode). They run
// before the row is sent to PostgreSQL, stripping bytes that PG would
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
