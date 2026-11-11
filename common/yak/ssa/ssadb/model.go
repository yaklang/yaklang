package ssadb

import "time"

// Model is gorm.Model without the index tag on DeletedAt.
//
// gorm.Model declares `DeletedAt *time.Time \`sql:"index"\“, so AutoMigrate
// builds an idx_<table>_deleted_at B-tree on every model that embeds it. The IR
// tables are never soft-deleted in practice: deleted_at is NULL on every row of
// ir_codes, ir_offsets, ir_indices_v1 and ir_types_v2 after a compile. Those
// indexes therefore have no selectivity, and no IR query can use them: every
// lookup binds program_name (or program_name plus value_id/code_id) first, so
// the planner prefers the composite key. Each one still costs a full B-tree
// build whenever the secondary IR indexes are restored after a compile, and
// about 239MB on a 9.8M-row database.
//
// Field names and order match gorm.Model exactly, so the column layout, the
// native INSERT column lists and GORM's soft-delete handling (which keys on the
// DeletedAt field name) are unchanged. The type must stay exported: GORM sets
// DeletedAt through reflection, and an unexported embedded struct makes that
// fail, silently turning soft deletes into hard deletes.
type Model struct {
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}
