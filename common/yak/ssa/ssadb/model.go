package ssadb

import "time"

// Model is gorm.Model without the index tag on DeletedAt.
//
// gorm.Model declares `DeletedAt *time.Time \`sql:"index"\“, so AutoMigrate
// builds an idx_<table>_deleted_at B-tree on every model that embeds it. The IR
// tables are never soft-deleted in practice. dbstat on a 9.8M-row core scan
// (ab18) measured 239MB of them: ir_codes 85MB, ir_offsets 76MB,
// ir_indices_v1 73MB, ir_types_v2 5MB. Every row of all four tables had
// deleted_at NULL, and no table held a soft-deleted row, so the indexes have no
// selectivity, and no IR query can use them: every lookup binds program_name
// (or program_name plus value_id/code_id) first, so the planner prefers the
// composite key. Each one still costs a full B-tree build whenever the
// secondary IR indexes are restored after a compile.
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
