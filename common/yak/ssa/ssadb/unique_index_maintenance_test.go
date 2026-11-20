package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func openIndexMaintenanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "index-maintenance.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}, &IrOffset{}).Error)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueIndexExists(t *testing.T, db *gorm.DB, table, index string) bool {
	t.Helper()
	var exists int64
	require.NoError(t, db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name=? AND name=?`,
		table, index,
	).Row().Scan(&exists))
	return exists > 0
}

// TestUniqueIndexPatch_KeepsDuplicateRows guards the decision to stop
// repairing legacy duplicates during index maintenance. A table that already
// contains duplicate rows must keep every row: CREATE UNIQUE INDEX fails, the
// unique index stays absent, and nobody silently deletes data.
func TestUniqueIndexPatch_KeepsDuplicateRows(t *testing.T) {
	db := openIndexMaintenanceTestDB(t)
	prog := "legacy-prog"

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&IrCode{
			ProgramName: prog, CodeID: 10, Opcode: 12, Name: "UploadFile",
		}).Error)
	}
	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 30, Opcode: 1, Name: "Unique",
	}).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&IrOffset{
			ProgramName: prog, ValueID: 1260, FileHash: "h",
			StartOffset: 3701, EndOffset: 3761, VariableName: "ternary_expression",
		}).Error)
	}

	patchIrCodeIndex(db)

	var codeCount int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&codeCount).Error)
	require.Equal(t, int64(4), codeCount, "legacy duplicate rows must not be deleted")

	var offsetCount int64
	require.NoError(t, db.Model(&IrOffset{}).Where("program_name = ?", prog).Count(&offsetCount).Error)
	require.Equal(t, int64(2), offsetCount, "legacy duplicate offsets must not be deleted")

	require.False(t, uniqueIndexExists(t, db, "ir_codes", "ux_ir_codes_program_code"),
		"the unique index cannot be created while duplicate rows exist")
	require.False(t, uniqueIndexExists(t, db, "ir_offsets", "ux_ir_offsets_program_value_file_range"),
		"the unique offsets index cannot be created while duplicate rows exist")
}
