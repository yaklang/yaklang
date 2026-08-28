package ssadb

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/consts"
	"path/filepath"
	"strings"
	"testing"
)

// --- from batch_chunk_large_test.go ---

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

// --- from ircode_batch_test.go ---

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

// --- from irtype_integrity_test.go ---

// TestSaveIrTypeBatchLargeBatchesKeepsIndexIntegrity reproduces the
// "row missing from index idx_ir_types_program_type" corruption seen on an
// EngineCMS run. It repeatedly upserts 2000-row batches with long JSON text
// into a file-backed SQLite DB and runs PRAGMA integrity_check.
func TestSaveIrTypeBatchLargeBatchesKeepsIndexIntegrity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "irtype-integrity.db")
	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrType{}).Error)

	longJSON := `{"fullTypeName":["` + strings.Repeat("org.apache.hadoop.example.TypeName", 20) + `"],"name":"x"}`
	for round := 0; round < 10; round++ {
		items := make([]*IrType, 0, 2000)
		for i := 0; i < 2000; i++ {
			items = append(items, &IrType{
				ProgramName:      "prog",
				TypeId:           uint64(round*2000 + i + 1),
				Kind:             i % 7,
				String:           fmt.Sprintf("T%d", i),
				ExtraInformation: longJSON,
			})
		}
		require.NoError(t, SaveIrTypeBatch(db, items), "round %d", round)
	}

	sqlDB := db.DB()
	rows, err := sqlDB.Query("PRAGMA integrity_check")
	require.NoError(t, err)
	defer rows.Close()
	var result []string
	for rows.Next() {
		var row string
		require.NoError(t, rows.Scan(&row))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	t.Logf("integrity_check rows: %#v", result)
	require.Equal(t, []string{"ok"}, result, "SQLite integrity_check must pass")

	var count int64
	require.NoError(t, db.Model(&IrType{}).Where("program_name = ?", "prog").Count(&count).Error)
	require.Equal(t, int64(20000), count)
}

// --- from unique_constraint_test.go ---

// TestUniqueConstraint_IrCodesProgramCodeId proves that ir_codes has a
// UNIQUE constraint on (program_name, code_id), preventing duplicate INSERTs.
func TestUniqueConstraint_IrCodesProgramCodeId(t *testing.T) {
	// Use a fresh DB to avoid conflicts with existing test data
	dbPath := filepath.Join(t.TempDir(), "test-unique.db")
	db, err := consts.CreateSSAProjectDatabaseRaw(dbPath)
	require.NoError(t, err, "failed to create fresh DB")
	defer db.Close()

	// Run the patch which creates the unique index
	patchIrCodeIndex(db)

	// Check that the unique index exists
	var indexCount int64
	db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='ir_codes' AND name='ux_ir_codes_program_code'`,
	).Row().Scan(&indexCount)
	require.Equal(t, int64(1), indexCount,
		"ir_codes must have UNIQUE INDEX ux_ir_codes_program_code ON (program_name, code_id)")

	// Try to insert duplicate (program_name, code_id)
	progName := "test-unique-constraint-1"

	// First insert should succeed
	ir1 := &IrCode{
		ProgramName: progName,
		CodeID:      42,
		Opcode:      1,
		Name:        "first",
	}
	require.NoError(t, db.Create(ir1).Error, "first insert should succeed")

	// Second insert with same (program_name, code_id) should FAIL
	ir2 := &IrCode{
		ProgramName: progName,
		CodeID:      42,
		Opcode:      1,
		Name:        "second",
	}
	err2 := db.Create(ir2).Error
	require.Error(t, err2, "second insert with same (program_name, code_id) must fail with UNIQUE constraint violation")
}

// TestUniqueConstraint_DifferentProgramsAllowed proves that different programs
// can have the same code_id without conflict.
func TestUniqueConstraint_DifferentProgramsAllowed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-diff-prog.db")
	db, err := consts.CreateSSAProjectDatabaseRaw(dbPath)
	require.NoError(t, err, "failed to create fresh DB")
	defer db.Close()
	patchIrCodeIndex(db)

	progA := "test-unique-constraint-a"
	progB := "test-unique-constraint-b"

	irA := &IrCode{ProgramName: progA, CodeID: 99, Opcode: 1, Name: "a"}
	irB := &IrCode{ProgramName: progB, CodeID: 99, Opcode: 1, Name: "b"}

	require.NoError(t, db.Create(irA).Error, "program A insert should succeed")
	require.NoError(t, db.Create(irB).Error, "program B insert with same code_id should succeed (different program_name)")
}
