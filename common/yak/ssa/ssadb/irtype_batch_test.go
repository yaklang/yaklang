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
	require.NoError(t, ensureIrTypeIndexes(db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func loadIrType(t *testing.T, db *gorm.DB, program string, id uint64) *IrType {
	t.Helper()
	got := GetIrTypeItemById(db, program, int64(id))
	require.NotNil(t, got, "type %s/%d must resolve", program, id)
	return got
}

func countIrTypes(t *testing.T, db *gorm.DB, program string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", program).Count(&count).Error)
	return count
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
	assert.Equal(t, n, countIrTypes(t, db, "prog"), "every type row must be persisted across chunks")

	// spot-check a row from the second chunk + the ExtraInformation text column
	got := loadIrType(t, db, "prog", uint64(irTypeBatchChunk+1))
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
	assert.Zero(t, countIrTypes(t, db, "prog"))
}

func TestSaveIrTypeBatch_NilEntriesSkippedWhenMixed(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{
		nil, {TypeId: 1, ProgramName: "p", String: "a"}, nil, {TypeId: 2, ProgramName: "p", String: "b"},
	}))
	assert.Equal(t, 2, countIrTypes(t, db, "p"))
}

// Re-flushing the same (program_name, type_id) with a new body must overwrite
// the stored row in place: the id is the conflict target, so the row id stays
// stable and no duplicate appears.
func TestSaveIrTypeBatch_UpsertOverwritesExisting(t *testing.T) {
	db := setupIrTypeTestDB(t)
	items := []*IrType{
		{TypeId: 1, Kind: 1, ProgramName: "prog", String: "T", ExtraInformation: `{"fullTypeName":["string"]}`},
	}
	require.NoError(t, SaveIrTypeBatch(db, items))
	var before IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "prog", uint64(1)).First(&before).Error)

	items[0].ExtraInformation = `{"fullTypeName":["string","java.lang.String"]}`
	require.NoError(t, SaveIrTypeBatch(db, items))

	assert.Equal(t, 1, countIrTypes(t, db, "prog"), "re-flush must update the row, not duplicate it")
	var after IrType
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "prog", uint64(1)).First(&after).Error)
	assert.Equal(t, before.ID, after.ID, "the row id must stay stable across flushes")

	got := loadIrType(t, db, "prog", 1)
	assert.Contains(t, got.ExtraInformation, "string")
	assert.Contains(t, got.ExtraInformation, "java.lang.String")
}

// A struct that came back from an earlier flush must be savable again without
// resetting it: the flush keys on (program_name, type_id).
func TestSaveIrTypeBatch_ReflushingTheSameStructKeepsRow(t *testing.T) {
	db := setupIrTypeTestDB(t)
	typ := &IrType{TypeId: 5, ProgramName: "p", String: "v1"}
	other := &IrType{TypeId: 6, ProgramName: "p", String: "other"}
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ, other}))

	typ.String = "v2"
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ}))
	assert.Equal(t, "v2", loadIrType(t, db, "p", 5).String)
	assert.Equal(t, "other", loadIrType(t, db, "p", 6).String, "reflushing one type must not touch another row")
	assert.Equal(t, 2, countIrTypes(t, db, "p"))
}

func TestSaveIrTypeBatch_LateFailurePreservesRowsAndCache(t *testing.T) {
	db := setupIrTypeTestDB(t)
	program := t.Name()
	t.Cleanup(func() { deleteCache(program) })
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{TypeId: 1, ProgramName: program, String: "original"}}))
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_late_ir_type BEFORE INSERT ON "`+TableIrTypes+`"
		WHEN NEW."string" = 'reject' BEGIN SELECT RAISE(ABORT, 'rejected late IR type'); END`).Error)

	items := make([]*IrType, 0, irTypeBatchChunk+2)
	for id := 1; id <= irTypeBatchChunk+1; id++ {
		items = append(items, &IrType{TypeId: uint64(id), ProgramName: program, String: "replacement"})
	}
	items = append(items, &IrType{TypeId: uint64(irTypeBatchChunk + 2), ProgramName: program, String: "reject"})
	require.ErrorContains(t, SaveIrTypeBatch(db, items), "rejected late IR type")

	require.Equal(t, 1, countIrTypes(t, db, program), "the rejected batch must roll back")
	assert.Equal(t, "original", loadIrType(t, db, program, 1).String)
	cached, ok := GetIrTypeCache(program).Get(1)
	require.True(t, ok)
	require.Equal(t, "original", cached.String, "a failed batch must preserve the previous cached snapshot")
	_, ok = GetIrTypeCache(program).Get(2)
	require.False(t, ok, "rolled-back rows must not enter the type cache")
}
