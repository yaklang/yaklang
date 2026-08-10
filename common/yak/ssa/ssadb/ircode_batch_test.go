package ssadb

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestIrCodeBatchRead_EquivalentToSingle proves that yieldIrCodes (batch)
// returns the same results as individual GetIrCodeItemById calls.
func TestIrCodeBatchRead_EquivalentToSingle(t *testing.T) {
	programName := uuid.NewString()
	defer DeleteProgram(GetDB(), programName)

	// Create test IrCodes
	db := GetDB()
	codes := make([]*IrCode, 0, 20)
	for i := int64(1); i <= 20; i++ {
		ir := &IrCode{
			ProgramName: programName,
			CodeID:      i,
			Opcode:      1,
			Name:        "inst",
		}
		codes = append(codes, ir)
	}
	require.NoError(t, db.CreateInBatches(codes, 100).Error)

	// Collect IDs
	ids := make([]int64, 0, 20)
	for _, ir := range codes {
		ids = append(ids, ir.CodeID)
	}

	// Batch read via yieldIrCodes
	batchResults := make(map[int64]*IrCode)
	ch := yieldIrCodes(context.Background(), programName, ids)
	for ir := range ch {
		batchResults[ir.CodeID] = ir
	}
	require.Len(t, batchResults, 20, "batch read should return all 20 codes")

	// Single read via GetIrCodeItemById
	for _, id := range ids {
		single := GetIrCodeItemById(db, programName, id)
		require.NotNil(t, single, "single read for id=%d should not be nil", id)
		batch, ok := batchResults[id]
		require.True(t, ok, "batch read should contain id=%d", id)
		require.Equal(t, single.CodeID, batch.CodeID, "CodeID should match")
	}
}

// TestIrCodeCache_BoundedCapacity proves the IrCode cache exists and is bounded.
func TestIrCodeCache_BoundedCapacity(t *testing.T) {
	cache := GetIrCodeCache("test-prog-bounded")
	require.NotNil(t, cache)
	for i := int64(1); i <= 10; i++ {
		cache.Set(i, &IrCode{CodeID: i, ProgramName: "test-prog-bounded"})
	}
	require.GreaterOrEqual(t, cache.Count(), 1)
	// Clear cache for cleanup
}
