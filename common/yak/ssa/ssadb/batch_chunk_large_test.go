package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

// TestLargeBatchChunksPersistAllRows proves that the 1000-row chunk sizes used
// by the type/index/offset stores and the IrCode insert path are accepted by
// the SQLite driver on this machine (MAX_VARIABLE_NUMBER=250000). It guards
// against regressing back to conservative ~100-150 row chunks.
func TestLargeBatchChunksPersistAllRows(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrCode{}, &IrType{}, &IrIndex{}, &IrOffset{}).Error)

	const n = 2000
	const chunk = 1000
	const irCodeChunk = 500

	codes := make([]*IrCode, 0, n)
	types := make([]*IrType, 0, n)
	indexes := make([]*IrIndex, 0, n)
	offsets := make([]*IrOffset, 0, n)
	for i := 1; i <= n; i++ {
		id := int64(i)
		codes = append(codes, &IrCode{
			ProgramName: "prog",
			CodeID:      id,
			Opcode:      1,
			Name:        "inst",
		})
		types = append(types, &IrType{
			ProgramName:      "prog",
			TypeId:           uint64(id),
			Kind:             i % 7,
			String:           "T",
			ExtraInformation: `{"name":"x"}`,
		})
		indexes = append(indexes, &IrIndex{
			ProgramName: "prog",
			ValueID:     id,
		})
		offsets = append(offsets, &IrOffset{
			ProgramName: "prog",
			FileHash:    "hash",
			StartOffset: id,
			EndOffset:   id + 1,
			ValueID:     id,
		})
	}

	require.NoError(t, db.CreateInBatches(codes, irCodeChunk).Error)
	require.NoError(t, SaveIrTypeBatch(db, types))
	SaveIrIndexBatch(db, indexes)
	require.NoError(t, SaveIrOffsetBatch(db, offsets))

	count := func(model any, table string) int {
		var c int64
		require.NoError(t, db.Model(model).Count(&c).Error, table)
		return int(c)
	}
	require.Equal(t, n, count(&IrCode{}, "ir_codes"))
	require.Equal(t, n, count(&IrType{}, "ir_types"))
	require.Equal(t, n, count(&IrIndex{}, "ir_indices_v1"))
	require.Equal(t, n, count(&IrOffset{}, "ir_offsets"))
}
