package ssadb

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIrTypeTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrType{}).Error)
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

	// re-flush same type_id with merged value
	items[0].ExtraInformation = `{"fullTypeName":["string","java.lang.String"]}`
	require.NoError(t, SaveIrTypeBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ? AND type_id = ?", "prog", uint64(1)).Count(&count).Error)
	assert.Equal(t, 1, count, "re-flush must overwrite, not duplicate")

	var got IrType
	require.NoError(t, db.Where("type_id = ?", uint64(1)).First(&got).Error)
	assert.Contains(t, got.ExtraInformation, "string")
	assert.Contains(t, got.ExtraInformation, "java.lang.String")
}
