package ssadb

import (
	"testing"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIrCodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)
	// Mirror production UNIQUE key so upsert overwrite is constrained.
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_ir_codes_program_code ON ir_codes (program_name, code_id)`,
	).Error)
	return db
}

func TestSaveIrCodeBatch_RoundTrip(t *testing.T) {
	db := setupIrCodeTestDB(t)
	n := irCodeBatchChunk + 17
	items := make([]*IrCode, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, &IrCode{
			CodeID:      int64(i + 1),
			ProgramName: "prog",
			Name:        "n",
			Opcode:      int64(i % 5),
		})
	}
	require.NoError(t, SaveIrCodeBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ?", "prog").Count(&count).Error)
	assert.Equal(t, n, count)
}

func TestSaveIrCodeBatch_UpsertOverwritesExisting(t *testing.T) {
	db := setupIrCodeTestDB(t)
	first := []*IrCode{{CodeID: 7, ProgramName: "prog", Name: "old", Opcode: 1}}
	require.NoError(t, SaveIrCodeBatch(db, first))

	second := []*IrCode{{CodeID: 7, ProgramName: "prog", Name: "new", Opcode: 2}}
	require.NoError(t, SaveIrCodeBatch(db, second))

	var count int
	require.NoError(t, db.Model(&IrCode{}).Where("program_name = ? AND code_id = ?", "prog", 7).Count(&count).Error)
	assert.Equal(t, 1, count)

	var got IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 7).First(&got).Error)
	assert.Equal(t, "new", got.Name)
	assert.Equal(t, int64(2), got.Opcode)
}

func TestUpsertIrCode_UsesBatchPath(t *testing.T) {
	db := setupIrCodeTestDB(t)
	require.NoError(t, UpsertIrCode(db, &IrCode{CodeID: 1, ProgramName: "p", Name: "a"}))
	require.NoError(t, UpsertIrCode(db, &IrCode{CodeID: 1, ProgramName: "p", Name: "b"}))

	var got IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "p", 1).First(&got).Error)
	assert.Equal(t, "b", got.Name)
}
