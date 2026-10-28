package ssadb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupIrTypeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrType{}).Error)
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_ir_types_program_type ON ir_types (program_name, type_id)`,
	).Error)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestSaveIrTypeBatch_RoundTrip(t *testing.T) {
	db := setupIrTypeTestDB(t)
	n := irTypeBatchChunk + 23
	items := make([]*IrType, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, &IrType{
			TypeId:           uint64(i + 1),
			Kind:             i % 7,
			ProgramName:      "prog",
			String:           "T",
			ExtraInformation: `{"name":"x"}`,
		})
	}
	require.NoError(t, SaveIrTypeBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", "prog").Count(&count).Error)
	assert.Equal(t, n, count, "every type row must be persisted across chunks")

	// spot-check a row from the second chunk + the ExtraInformation text column
	var got IrType
	require.NoError(t, db.Where("type_id = ?", uint64(irTypeBatchChunk+1)).First(&got).Error)
	assert.Equal(t, "prog", got.ProgramName)
	assert.Equal(t, (irTypeBatchChunk)%7, got.Kind)
	assert.Equal(t, `{"name":"x"}`, got.ExtraInformation)

	// cache side-effect preserved
	c := GetIrTypeCache("prog")
	if c != nil {
		v, ok := c.Get(int64(irTypeBatchChunk + 1))
		require.True(t, ok)
		assert.Equal(t, "prog", v.ProgramName)
	}
}

func TestSaveIrTypeBatch_NilSkipped(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{nil, nil, nil}))
	var count int
	require.NoError(t, db.Model(&IrType{}).Count(&count).Error)
	assert.Equal(t, 0, count)
}

// TestSaveIrTypeBatch_UpsertOverwritesExisting verifies the batched upsert
// preserves the idempotent-update semantics of the old per-row UpsertIrType:
// re-flushing the same (program_name, type_id) with a new value must overwrite
// the existing row, not insert a duplicate. Mirrors
// TestTypeFlushUpsertsExistingTypeRows at the ssadb layer.
func TestSaveIrTypeBatch_UpsertOverwritesExisting(t *testing.T) {
	db := setupIrTypeTestDB(t)
	items := []*IrType{
		{TypeId: 1, Kind: 1, ProgramName: "prog", String: "T", ExtraInformation: `{"fullTypeName":["string"]}`},
	}
	require.NoError(t, SaveIrTypeBatch(db, items))
	var before IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "prog", uint64(1)).First(&before).Error)

	// re-flush same type_id with merged value
	items[0].ExtraInformation = `{"fullTypeName":["string","java.lang.String"]}`
	require.NoError(t, SaveIrTypeBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ? AND type_id = ?", "prog", uint64(1)).Count(&count).Error)
	assert.Equal(t, 1, count, "re-flush must overwrite, not duplicate")

	var got IrType
	require.NoError(t, db.Where("type_id = ?", uint64(1)).First(&got).Error)
	assert.Equal(t, before.ID, got.ID, "updating a type must not delete and reinsert its row")
	assert.Contains(t, got.ExtraInformation, "string")
	assert.Contains(t, got.ExtraInformation, "java.lang.String")
}

func TestSaveIrTypeBatch_NilEntriesSkippedWhenMixed(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{nil, {TypeId: 1, ProgramName: "p", String: "a"}, nil, {TypeId: 2, ProgramName: "p", String: "b"}}))
	var count int
	require.NoError(t, db.Model(&IrType{}).Count(&count).Error)
	assert.Equal(t, 2, count)
}

// A struct that came back from an earlier flush must be savable again without
// resetting it: the flush keys on (program_name, type_id).
func TestSaveIrTypeBatch_ReflushingTheSameStructKeepsRow(t *testing.T) {
	db := setupIrTypeTestDB(t)
	typ := &IrType{TypeId: 5, ProgramName: "p", String: "v1"}
	other := &IrType{TypeId: 6, ProgramName: "p", String: "other"}
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ, other}))
	var first IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "p", 5).First(&first).Error)

	typ.String = "v2"
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ}))
	var second, untouched IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "p", 5).First(&second).Error)
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "p", 6).First(&untouched).Error)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "v2", second.String)
	assert.Equal(t, "other", untouched.String)
}

func TestSaveIrTypeBatch_LateFailurePreservesRowsAndCache(t *testing.T) {
	db := setupIrTypeTestDB(t)
	program := t.Name()
	t.Cleanup(func() { deleteCache(program) })
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{TypeId: 1, ProgramName: program, String: "original"}}))
	var before IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&before).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_late_ir_type BEFORE INSERT ON ir_types
		WHEN NEW.string = 'reject' BEGIN SELECT RAISE(ABORT, 'rejected late IR type'); END`).Error)

	items := make([]*IrType, 0, irTypeBatchChunk+2)
	for id := 1; id <= irTypeBatchChunk+1; id++ {
		items = append(items, &IrType{TypeId: uint64(id), ProgramName: program, String: "replacement"})
	}
	items = append(items, &IrType{TypeId: uint64(irTypeBatchChunk + 2), ProgramName: program, String: "reject"})
	require.ErrorContains(t, SaveIrTypeBatch(db, items), "rejected late IR type")

	var after IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&after).Error)
	require.Equal(t, before.ID, after.ID)
	require.True(t, before.CreatedAt.Equal(after.CreatedAt))
	require.Equal(t, "original", after.String)
	var count int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", program).Count(&count).Error)
	require.Equal(t, 1, count)
	cached, ok := GetIrTypeCache(program).Get(1)
	require.True(t, ok)
	require.Equal(t, "original", cached.String, "a failed batch must preserve the previous cached snapshot")
	_, ok = GetIrTypeCache(program).Get(2)
	require.False(t, ok, "rolled-back rows must not enter the type cache")
}

func TestEnsureUniqueIrTypesIndex_RemovesDuplicatesKeepingOldest(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&IrType{}).Error)
	for _, s := range []string{"first", "second", "third"} {
		require.NoError(t, db.Create(&IrType{TypeId: 9, ProgramName: "p", String: s}).Error)
	}
	require.NoError(t, db.Create(&IrType{TypeId: 9, ProgramName: "other", String: "separate program"}).Error)

	ensureUniqueIrTypesProgramTypeIndex(db)
	ensureUniqueIrTypesProgramTypeIndex(db) // idempotent

	require.True(t, db.New().Dialect().HasIndex(TableIrTypes, "ux_ir_types_program_type"))
	var rows []IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "p", 9).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "first", rows[0].String, "the oldest row survives")
	var other int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", "other").Count(&other).Error)
	assert.Equal(t, 1, other, "another program is not deduplicated")
	require.Error(t, db.Create(&IrType{TypeId: 9, ProgramName: "p", String: "dup"}).Error, "the index must now reject duplicates")
}
