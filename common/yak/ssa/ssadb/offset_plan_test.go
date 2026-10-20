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

	// No ANALYZE on this database, matching the scan database. ORDER BY id
	// makes SQLite 3.51/3.54 abandon the (program_name, value_id) prefix.
	plan := sqliteQueryPlan(t, db, query, args...)
	ordered := sqliteQueryPlan(t, db, query+" ORDER BY id", args...)
	t.Logf("sqlite %s\nno-order:\n%sorder-by-id:\n%s", sqliteVersion(t, db), plan, ordered)
	require.Contains(t, plan, "ux_ir_offsets_program_value_file_range")
	require.NotContains(t, plan, "idx_ir_offsets_value_id")
	require.Contains(t, ordered, "idx_ir_offsets_value_id")
}

func sqliteVersion(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var version string
	require.NoError(t, db.Raw("SELECT sqlite_version()").Row().Scan(&version))
	return version
}
