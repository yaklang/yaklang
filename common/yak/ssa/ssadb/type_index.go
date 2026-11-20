package ssadb

import (
	"errors"

	"github.com/yaklang/gorm"
)

// irTypeUniqueIndex is the conflict target every type write upserts on. It is
// UNIQUE, so a replayed flush cannot store one type id twice.
const irTypeUniqueIndex = "ux_ir_types_program_type"

// ensureIrTypeIndexes installs the type conflict target on a bare schema.
func ensureIrTypeIndexes(db *gorm.DB) error {
	if db == nil || db.Dialect() == nil {
		return errors.New("no dialect")
	}
	return EnsureIndex(db, IndexSpec{
		Name:    irTypeUniqueIndex,
		Table:   TableIrTypes,
		Columns: []string{"program_name", "type_id"},
		Unique:  true,
	})
}

// EnsureIrTypeIndexesForTest installs the unique key on a bare test database
// that migrated the model directly.
func EnsureIrTypeIndexesForTest(db *gorm.DB) error {
	return ensureIrTypeIndexes(db)
}
