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
	require.NoError(t, db.AutoMigrate(&IrTypeID{}, &IrTypePayload{}).Error)
	require.NoError(t, ensureIrTypeDedupeIndexes(db))
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
	require.NoError(t, db.Model(&IrTypeID{}).Where("program_name = ?", "prog").Count(&count).Error)
	assert.Equal(t, n, count, "every type row must be persisted across chunks")

	// spot-check a row from the second chunk + the ExtraInformation text column
	got := loadTypePayload(t, db, "prog", uint64(irTypeBatchChunk+1))
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
	require.NoError(t, db.Model(&IrTypeID{}).Count(&count).Error)
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
	var before IrTypeID
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "prog", uint64(1)).First(&before).Error)

	// re-flush same type_id with merged value
	items[0].ExtraInformation = `{"fullTypeName":["string","java.lang.String"]}`
	require.NoError(t, SaveIrTypeBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrTypeID{}).Where("program_name = ? AND type_id = ?", "prog", uint64(1)).Count(&count).Error)
	assert.Equal(t, 1, count, "re-flush must repoint the id, not duplicate it")

	var after IrTypeID
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", "prog", uint64(1)).First(&after).Error)
	assert.NotEqual(t, before.PayloadID, after.PayloadID, "a changed body points the id at a new payload")

	var payload IrTypePayload
	require.NoError(t, db.Where("id = ?", after.PayloadID).First(&payload).Error)
	assert.Contains(t, payload.ExtraInformation, "string")
	assert.Contains(t, payload.ExtraInformation, "java.lang.String")
}

func TestSaveIrTypeBatch_NilEntriesSkippedWhenMixed(t *testing.T) {
	db := setupIrTypeTestDB(t)
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{nil, {TypeId: 1, ProgramName: "p", String: "a"}, nil, {TypeId: 2, ProgramName: "p", String: "b"}}))
	var count int
	require.NoError(t, db.Model(&IrTypeID{}).Count(&count).Error)
	assert.Equal(t, 2, count)
}

// A struct that came back from an earlier flush must be savable again without
// resetting it: the flush keys on (program_name, type_id).
func TestSaveIrTypeBatch_ReflushingTheSameStructKeepsRow(t *testing.T) {
	db := setupIrTypeTestDB(t)
	typ := &IrType{TypeId: 5, ProgramName: "p", String: "v1"}
	other := &IrType{TypeId: 6, ProgramName: "p", String: "other"}
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ, other}))
	untouchedBefore := loadTypePayload(t, db, "p", 6)

	typ.String = "v2"
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{typ}))
	assert.Equal(t, "v2", loadTypePayload(t, db, "p", 5).String)
	assert.Equal(t, "other", loadTypePayload(t, db, "p", 6).String)
	assert.Equal(t, untouchedBefore.ID, loadTypePayload(t, db, "p", 6).ID,
		"reflushing one type must not touch another type's payload")
	var ids int
	require.NoError(t, db.Model(&IrTypeID{}).Where("program_name = ?", "p").Count(&ids).Error)
	assert.Equal(t, 2, ids, "a reflush repoints the id, it does not add one")
}

func TestSaveIrTypeBatch_LateFailurePreservesRowsAndCache(t *testing.T) {
	db := setupIrTypeTestDB(t)
	program := t.Name()
	t.Cleanup(func() { deleteCache(program) })
	require.NoError(t, SaveIrTypeBatch(db, []*IrType{{TypeId: 1, ProgramName: program, String: "original"}}))
	var before IrTypeID
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&before).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_late_ir_type BEFORE INSERT ON ir_type_payloads
		WHEN NEW.string = 'reject' BEGIN SELECT RAISE(ABORT, 'rejected late IR type'); END`).Error)

	items := make([]*IrType, 0, irTypeBatchChunk+2)
	for id := 1; id <= irTypeBatchChunk+1; id++ {
		items = append(items, &IrType{TypeId: uint64(id), ProgramName: program, String: "replacement"})
	}
	items = append(items, &IrType{TypeId: uint64(irTypeBatchChunk + 2), ProgramName: program, String: "reject"})
	require.ErrorContains(t, SaveIrTypeBatch(db, items), "rejected late IR type")

	var after IrTypeID
	require.NoError(t, db.Where("program_name = ? AND type_id = ?", program, 1).First(&after).Error)
	require.Equal(t, before.PayloadID, after.PayloadID, "a rejected batch must not repoint the id")
	var count int
	require.NoError(t, db.Model(&IrTypeID{}).Where("program_name = ?", program).Count(&count).Error)
	require.Equal(t, 1, count)
	cached, ok := GetIrTypeCache(program).Get(1)
	require.True(t, ok)
	require.Equal(t, "original", cached.String, "a failed batch must preserve the previous cached snapshot")
	_, ok = GetIrTypeCache(program).Get(2)
	require.False(t, ok, "rolled-back rows must not enter the type cache")
}
