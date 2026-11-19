package ssadb

import (
	"errors"

	"github.com/yaklang/gorm"
)

// irTypeUniqueIndex is the conflict target every type write upserts on. It is
// UNIQUE, so a replayed flush cannot store one type id twice.
const irTypeUniqueIndex = "ux_ir_types_program_type"

// ensureIrTypeIndexes installs the type conflict target on a bare schema. The
// key is created as-is: type rows are upserted, so a database that already
// violates it is a bug to report, not to repair by deleting rows.
func ensureIrTypeIndexes(db *gorm.DB) error {
	if db == nil || db.Dialect() == nil {
		return errors.New("no dialect")
	}
	return ensureIndex(db, indexSpec{
		table:  TableIrTypes,
		name:   irTypeUniqueIndex,
		unique: true,
		key:    []indexKeyPart{indexCol("program_name"), indexCol("type_id")},
	})
}

// EnsureIrTypeIndexesForTest installs the unique key on a bare test database
// that migrated the model directly.
func EnsureIrTypeIndexesForTest(db *gorm.DB) error {
	return ensureIrTypeIndexes(db)
}
