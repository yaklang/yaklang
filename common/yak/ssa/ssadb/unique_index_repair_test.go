package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func openRepairTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "repair.db"))
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

// TestUniqueIndexPatch_RepairsExactDuplicateIrCodes proves that the patch
// auto-repairs legacy exact-duplicate ir_codes rows (same content, only
// id/timestamps differ) by keeping MIN(id) — the row normal single-read paths
// return — and then creates the unique index.
func TestUniqueIndexPatch_RepairsExactDuplicateIrCodes(t *testing.T) {
	db := openRepairTestDB(t)
	prog := "repair-prog"

	// Three exact copies of code_id=10 and two exact copies of code_id=20,
	// plus one unique row. Content is identical; only id/timestamps differ.
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&IrCode{
			ProgramName: prog, CodeID: 10, Opcode: 12, Name: "UploadFile",
		}).Error)
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&IrCode{
			ProgramName: prog, CodeID: 20, Opcode: 5, Name: "Const",
		}).Error)
	}
	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 30, Opcode: 1, Name: "Unique",
	}).Error)

	patchIrCodeIndex(db)

	var count int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&count).Error)
	require.Equal(t, int64(3), count, "duplicates must be removed, one row per code_id")

	// The kept row must be the first inserted (MIN id), matching normal reads.
	var kept IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 10).First(&kept).Error)
	require.Equal(t, "UploadFile", kept.Name)
	require.Equal(t, int64(12), kept.Opcode)

	require.True(t, uniqueIndexExists(t, db, "ir_codes", "ux_ir_codes_program_code"),
		"unique index must be created after exact-duplicate repair")
}

// TestUniqueIndexPatch_RepairsExactDuplicateIrOffsets proves the same
// auto-repair for ir_offsets, whose composite key already covers every content
// column.
func TestUniqueIndexPatch_RepairsExactDuplicateIrOffsets(t *testing.T) {
	db := openRepairTestDB(t)
	prog := "repair-offset-prog"

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&IrOffset{
			ProgramName: prog, ValueID: 1260, FileHash: "h",
			StartOffset: 3701, EndOffset: 3761, VariableName: "ternary_expression",
		}).Error)
	}

	patchIrCodeIndex(db)

	var count int64
	require.NoError(t, db.Model(&IrOffset{}).Where("program_name = ?", prog).Count(&count).Error)
	require.Equal(t, int64(1), count, "duplicate offsets must be removed, one row per composite key")

	require.True(t, uniqueIndexExists(t, db, "ir_offsets", "ux_ir_offsets_program_value_file_range"),
		"unique index must be created after exact-duplicate repair")
}

// TestUniqueIndexPatch_RepairsConflictingIrCodeDuplicates proves that rows
// sharing (program_name, code_id) are deduplicated even when their content
// differs: the extra row is removed, MIN(id) is kept, and the unique index
// is created.
func TestUniqueIndexPatch_RepairsConflictingIrCodeDuplicates(t *testing.T) {
	db := openRepairTestDB(t)
	prog := "conflict-prog"

	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 10, Opcode: 12, Name: "A",
	}).Error)
	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 10, Opcode: 99, Name: "B",
	}).Error)

	patchIrCodeIndex(db)

	var count int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&count).Error)
	require.Equal(t, int64(1), count, "conflicting duplicates must be deduplicated to one row")

	var kept IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 10).First(&kept).Error)
	require.Equal(t, "A", kept.Name, "MIN(id) row must be kept")
	require.True(t, uniqueIndexExists(t, db, "ir_codes", "ux_ir_codes_program_code"),
		"unique index must be created after dedup")
}

// TestUniqueIndexPatch_RepairsMixedDuplicateGroup proves that a key group
// containing exact copies plus a conflicting row (A, A, B) is deduplicated to
// the MIN(id) row and the unique index is created.
func TestUniqueIndexPatch_RepairsMixedDuplicateGroup(t *testing.T) {
	db := openRepairTestDB(t)
	prog := "mixed-prog"

	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 10, Opcode: 12, Name: "A",
	}).Error)
	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 10, Opcode: 12, Name: "A",
	}).Error)
	require.NoError(t, db.Create(&IrCode{
		ProgramName: prog, CodeID: 10, Opcode: 99, Name: "B",
	}).Error)

	patchIrCodeIndex(db)

	var count int64
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", prog).Count(&count).Error)
	require.Equal(t, int64(1), count, "mixed duplicate group must be deduplicated to one row")

	var kept IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 10).First(&kept).Error)
	require.Equal(t, "A", kept.Name, "MIN(id) row must be kept")
	require.True(t, uniqueIndexExists(t, db, "ir_codes", "ux_ir_codes_program_code"),
		"unique index must be created after dedup")
}
