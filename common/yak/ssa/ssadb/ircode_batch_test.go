package ssadb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/utils"
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
	t.Cleanup(func() { require.NoError(t, db.Close()) })
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

func TestSaveIrCodeBatch_UpsertFailurePreservesExisting(t *testing.T) {
	db := setupIrCodeTestDB(t)
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{CodeID: 7, ProgramName: "prog", Name: "old"}}))
	// The upsert is INSERT ... ON CONFLICT DO UPDATE, so an existing row is
	// rejected by the UPDATE trigger, not by a BEFORE INSERT trigger.
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_ir_update BEFORE UPDATE ON ir_codes
		WHEN NEW.name = 'reject' BEGIN SELECT RAISE(ABORT, 'rejected IR'); END`).Error)

	err := SaveIrCodeBatch(db, []*IrCode{{CodeID: 7, ProgramName: "prog", Name: "reject"}})
	require.ErrorContains(t, err, "rejected IR")

	var got IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 7).First(&got).Error,
		"a failed replacement must preserve the previously persisted IR")
	require.Equal(t, "old", got.Name)
}

func TestSaveIrCodeBatch_UpsertRestoresDeletedRowInPlace(t *testing.T) {
	db := setupIrCodeTestDB(t)
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{
		{CodeID: 7, ProgramName: "prog", Name: "old", IsExternal: true, Users: Int64Slice{1, 2}},
		{CodeID: 7, ProgramName: "other", Name: "untouched"},
	}))
	var before IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 7).First(&before).Error)
	require.NoError(t, db.Delete(&before).Error)

	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{{CodeID: 7, ProgramName: "prog", Name: "new"}}))
	var got IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 7).First(&got).Error)
	require.Equal(t, before.ID, got.ID, "updating an existing IR must not delete and reinsert its row")
	require.Equal(t, "new", got.Name)
	require.False(t, got.IsExternal, "zero-valued fields must overwrite the previous snapshot")
	require.Empty(t, got.Users)
	require.Nil(t, got.DeletedAt)

	var other IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "other", 7).First(&other).Error)
	require.Equal(t, "untouched", other.Name)
}

func TestSaveIrCodeBatch_ProductionScopedTransaction(t *testing.T) {
	program := uuid.NewString()
	db := GetDB().Where("program_name = ?", program)
	t.Cleanup(func() { DeleteProgram(GetDB(), program) })
	for _, name := range []string{"before", "after"} {
		require.NoError(t, utils.GormTransaction(db, func(tx *gorm.DB) error {
			return SaveIrCodeBatch(tx, []*IrCode{{CodeID: 1, ProgramName: program, Name: name}})
		}))
	}
	var got IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", program, 1).First(&got).Error)
	require.Equal(t, "after", got.Name)
}

// The flush used to clear ID/CreatedAt/UpdatedAt/DeletedAt on every struct before
// saving. SaveInBatches keys on (program_name, code_id) and never writes the
// primary key, so a struct that still carries state from an earlier load or save
// lands on the right row and leaves the others alone.
func TestSaveIrCodeBatch_StructStateFromEarlierSaveIsHarmless(t *testing.T) {
	db := setupIrCodeTestDB(t)
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{
		{CodeID: 1, ProgramName: "prog", Name: "one"},
		{CodeID: 2, ProgramName: "prog", Name: "two"},
	}))
	var one, two IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 1).First(&one).Error)
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 2).First(&two).Error)

	// A loaded row for code 1 that was re-pointed at the primary key of code 2.
	reused := one
	reused.ID = two.ID
	reused.Name = "one-renewed"
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{&reused}))

	var got1, got2 IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 1).First(&got1).Error)
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", "prog", 2).First(&got2).Error)
	require.Equal(t, one.ID, got1.ID, "the row keeps its id whatever id the struct carries")
	require.Equal(t, "one-renewed", got1.Name)
	require.Equal(t, two.ID, got2.ID)
	require.Equal(t, "two", got2.Name, "another row must not be touched")
	require.True(t, got1.CreatedAt.Equal(one.CreatedAt), "created_at is kept")
}

// A flush may be handed a sparse slice; nil entries are skipped on both paths.
func TestSaveIrCodeBatch_NilEntriesSkipped(t *testing.T) {
	db := setupIrCodeTestDB(t)
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{nil, {CodeID: 1, ProgramName: "p", Name: "a"}, nil, {CodeID: 2, ProgramName: "p", Name: "b"}}))
	var count int
	require.NoError(t, db.Model(&IrCode{}).Count(&count).Error)
	require.Equal(t, 2, count)
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{nil, nil}))
}
