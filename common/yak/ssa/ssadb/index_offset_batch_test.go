package ssadb

import (
	"testing"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIndexOffsetTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrIndex{}, &IrOffset{}).Error)
	return db
}

// TestSaveIrIndexBatch_RoundTrip verifies the chunked multi-row INSERT writes
// every row (including across the chunk boundary) and that the application
// columns round-trip. Regression guard for the per-row db.Create -> bulk
// multi-row INSERT conversion.
func TestSaveIrIndexBatch_RoundTrip(t *testing.T) {
	db := setupIndexOffsetTestDB(t)
	// irIndexBatchChunk + a few extra to force a second chunk.
	n := irIndexBatchChunk + 37
	items := make([]*IrIndex, 0, n)
	for i := 0; i < n; i++ {
		v := int64(i + 1)
		items = append(items, &IrIndex{
			ProgramName:  "prog",
			ValueID:      v,
			VariableID:   &v,
			ClassID:      nil,
			FieldID:      &v,
			ScopeName:    "scope",
			OwnerValueID: nil,
			VersionID:    v * 2,
		})
	}

	SaveIrIndexBatch(db, items)

	var count int
	require.NoError(t, db.Model(&IrIndex{}).Where("program_name = ?", "prog").Count(&count).Error)
	assert.Equal(t, n, count, "every row must be persisted across chunks")

	// spot-check a row from the second chunk
	var got IrIndex
	require.NoError(t, db.Where("value_id = ?", int64(irIndexBatchChunk+1)).First(&got).Error)
	assert.Equal(t, "prog", got.ProgramName)
	assert.Equal(t, int64(irIndexBatchChunk+1), got.ValueID)
	assert.Equal(t, "scope", got.ScopeName)
	assert.Equal(t, int64(irIndexBatchChunk+1)*2, got.VersionID)
}

// TestSaveIrOffsetBatch_RoundTrip verifies the offset batched INSERT writes
// every row across chunks and round-trips the application columns.
func TestSaveIrOffsetBatch_RoundTrip(t *testing.T) {
	db := setupIndexOffsetTestDB(t)
	n := irOffsetBatchChunk + 13
	items := make([]*IrOffset, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, &IrOffset{
			ProgramName:  "prog",
			FileHash:     "hash",
			StartOffset:  int64(i),
			EndOffset:    int64(i + 1),
			VariableName: "v",
			ValueID:      int64(i + 1),
		})
	}

	require.NoError(t, SaveIrOffsetBatch(db, items))

	var count int
	require.NoError(t, db.Model(&IrOffset{}).Where("program_name = ?", "prog").Count(&count).Error)
	assert.Equal(t, n, count, "every offset row must be persisted across chunks")

	var got IrOffset
	require.NoError(t, db.Where("value_id = ?", int64(irOffsetBatchChunk+1)).First(&got).Error)
	assert.Equal(t, "hash", got.FileHash)
	assert.Equal(t, int64(irOffsetBatchChunk), got.StartOffset)
	assert.Equal(t, "v", got.VariableName)
}

// TestSaveIrOffsetBatch_NilSkipped ensures nil entries are skipped, not stored.
func TestSaveIrOffsetBatch_NilSkipped(t *testing.T) {
	db := setupIndexOffsetTestDB(t)
	require.NoError(t, SaveIrOffsetBatch(db, []*IrOffset{nil, nil, nil}))
	var count int
	require.NoError(t, db.Model(&IrOffset{}).Count(&count).Error)
	assert.Equal(t, 0, count)
}
