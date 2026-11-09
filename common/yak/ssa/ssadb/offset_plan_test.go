package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

// TestOffsetVariableLookupKeepsFilteredIDOrder checks the rows callers see,
// not a particular index name. id order is preserved in memory because the
// composite index order is not id order.
func TestOffsetVariableLookupKeepsFilteredIDOrder(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	ensureUniqueIrOffsetsIndex(db)

	prog := "offset-plan"
	rows := []*IrOffset{
		{ProgramName: prog, ValueID: 7, VariableName: "later", FileHash: "b", StartOffset: 30, EndOffset: 40},
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "b", StartOffset: 30, EndOffset: 40},
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: "other", ValueID: 7, VariableName: "x", FileHash: "z", StartOffset: 1, EndOffset: 2},
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "gone", StartOffset: 1, EndOffset: 2},
	}
	require.NoError(t, db.CreateInBatches(rows, 10).Error)
	require.NoError(t, db.Delete(rows[len(rows)-1]).Error)

	query, args := offsetVariableQuery("x", 7, prog)
	require.NotContains(t, query, "ORDER BY")
	got, err := nativeGetOffsetsByVariable(db, "x", 7, prog)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Less(t, got[0].ID, got[1].ID)
	require.Equal(t, "b", got[0].FileHash)
	require.Equal(t, "a", got[1].FileHash)
	require.NotContains(t, []string{got[0].FileHash, got[1].FileHash}, "gone")
	require.NotContains(t, []string{got[0].FileHash, got[1].FileHash}, "z")

	// No ANALYZE on this database, matching the scan database. The unary "+"
	// disqualifies the name index, so the composite (program_name, value_id)
	// prefix drives the lookup.
	plan := sqliteQueryPlan(t, db, query, args...)
	t.Logf("sqlite %s\nno-order:\n%s", sqliteVersion(t, db), plan)
	require.Contains(t, plan, "ux_ir_offsets_program_value_file_range")
	require.NotContains(t, plan, "idx_ir_offsets_variable_name")
	require.NotContains(t, plan, "SCAN "+TableIrOffsets)

	// Adding ORDER BY id makes SQLite walk the deleted_at index instead, so
	// production sorts in memory and never emits ORDER BY.
	ordered := sqliteQueryPlan(t, db, query+" ORDER BY id", args...)
	require.NotContains(t, ordered, "ux_ir_offsets_program_value_file_range")
	require.NotContains(t, ordered, "SCAN "+TableIrOffsets)
}

// A production database can reopen with the variable-name index created after
// the composite key. Without statistics, SQLite can then choose the name index
// for every instruction named "this", even without ORDER BY. Keep the lookup
// bounded by the program/value prefix regardless of index creation order.
func TestOffsetVariableLookupKeepsValueIndexAfterIndexRecreation(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	ensureUniqueIrOffsetsIndex(db)
	require.NoError(t, db.Exec(`DROP INDEX idx_ir_offsets_variable_name`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_ir_offsets_variable_name ON ir_offsets(variable_name)`).Error)

	rows := make([]*IrOffset, 4096)
	for i := range rows {
		rows[i] = &IrOffset{
			ProgramName: t.Name(), ValueID: int64(i + 1), VariableName: "this",
			FileHash: "f", StartOffset: int64(i), EndOffset: int64(i + 1),
		}
	}
	require.NoError(t, db.CreateInBatches(rows, 500).Error)

	query, args := offsetVariableQuery("this", 2048, t.Name())
	plan := sqliteQueryPlan(t, db, query, args...)
	t.Logf("sqlite %s, offset lookup plan: %s", sqliteVersion(t, db), plan)
	require.Contains(t, plan, "ux_ir_offsets_program_value_file_range")
	require.Contains(t, plan, "program_name=? AND value_id=?")
	require.NotContains(t, plan, "idx_ir_offsets_variable_name")

	got, err := nativeGetOffsetsByVariable(db, "this", 2048, t.Name())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, rows[2047].ID, got[0].ID)
}

// The disqualified term drops variable_name's TEXT affinity. Names stay exact
// text matches because every caller binds a string; a numeric-looking name
// must not start matching its neighbours.
func TestOffsetVariableLookupKeepsExactTextName(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	ensureUniqueIrOffsetsIndex(db)

	prog := "offset-name-text"
	one := &IrOffset{ProgramName: prog, ValueID: 11, VariableName: "1", FileHash: "f1", StartOffset: 1, EndOffset: 2}
	octal := &IrOffset{ProgramName: prog, ValueID: 11, VariableName: "01", FileHash: "f2", StartOffset: 3, EndOffset: 4}
	require.NoError(t, db.CreateInBatches([]*IrOffset{one, octal}, 10).Error)

	got, err := nativeGetOffsetsByVariable(db, "1", 11, prog)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, one.ID, got[0].ID)

	got, err = nativeGetOffsetsByVariable(db, "01", 11, prog)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, octal.ID, got[0].ID)
}

func sqliteVersion(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var version string
	require.NoError(t, db.Raw("SELECT sqlite_version()").Row().Scan(&version))
	return version
}
