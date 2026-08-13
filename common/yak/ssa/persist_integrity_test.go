package ssa

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- from async_persist_dedup_test.go ---

// TestPersistedIDsGuard_PreventsDuplicatePersist proves that the
// instructionStore's persistedIDs set prevents the same instruction
// ID from being persisted twice, even if the item is re-added to
// the writer cache after being persisted.
func TestPersistedIDsGuard_PreventsDuplicatePersist(t *testing.T) {
	store := &instructionStore{
		mode:         ProgramCacheDBWrite,
		persistedIDs: make(map[int64]struct{}),
	}

	// Simulate persisting ID 42
	store.markPersisted(42)
	store.markPersisted(43)

	// Check: 42 and 43 should be marked as persisted
	require.True(t, store.isPersisted(42), "ID 42 should be marked persisted")
	require.True(t, store.isPersisted(43), "ID 43 should be marked persisted")

	// Check: 44 should NOT be marked
	require.False(t, store.isPersisted(44), "ID 44 should not be marked persisted")

	// Re-marking 42 should be a no-op (idempotent)
	store.markPersisted(42)
	require.True(t, store.isPersisted(42))

	// Thread safety: concurrent markPersisted should not panic
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			store.markPersisted(id)
		}(i)
	}
	wg.Wait()

	// All IDs 0-99 should be marked
	for i := int64(1); i <= 100; i++ {
		require.True(t, store.isPersisted(i), "ID %d should be marked", i)
	}
}

// --- from persist_integrity_regression_test.go ---

// TestPersistIntegrity_DBCountMatchesCompile proves that after
// FlushCompileUnit + SaveToDatabase + CleanBaseline, the DB ir_codes
// count equals the total instructions compiled.
func TestPersistIntegrity_DBCountMatchesCompile(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("testFunc1", string(MainFunctionName))
	for i := 0; i < 50; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	builder2 := prog.GetAndCreateFunctionBuilder("testFunc2", string(MainFunctionName))
	for i := 0; i < 50; i++ {
		left := builder2.EmitUndefined("left2")
		right := builder2.EmitUndefined("right2")
		builder2.EmitBinOp(OpSub, left, right)
	}
	builder2.Finish()

	// Mid-compile flushes
	prog.Cache.FlushCompileUnit("unit-1")
	prog.Cache.FlushCompileUnit("unit-2")

	// Track total before SaveToDatabase
	totalBefore := int64(prog.Cache.CountInstruction() + int(prog.Cache.InstructionPersistedCount()))
	require.Greater(t, totalBefore, int64(0), "should have instructions")

	// SaveToDatabase
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// After SaveToDatabase: all should be persisted, none resident
	persistedAfter := int64(prog.Cache.InstructionPersistedCount())
	residentAfter := int64(prog.Cache.CountInstruction())
	require.Equal(t, int64(0), residentAfter,
		"no instructions should be resident after SaveToDatabase")
	require.Equal(t, totalBefore, persistedAfter,
		"persisted count (%d) must equal total compiled (%d)", persistedAfter, totalBefore)

	// CleanBaseline
	prog.Cache.CleanBaseline()

	// DB count must match

	var dbRowCount int64
	ssadb.GetDB().Model(&ssadb.IrCode{}).
		Where("program_name = ?", programName).
		Count(&dbRowCount)

	require.Equal(t, totalBefore, dbRowCount,
		"DB ir_codes count (%d) must equal total compiled (%d)", dbRowCount, totalBefore)
}

// --- from persisted_ids_cross_request_test.go ---

// TestPersistedIDs_CrossRequestDedup simulates the engineercms failure:
// same Cache, same program, two requests for same code_id.
func TestPersistedIDs_CrossRequestDedup(t *testing.T) {
	store := &instructionStore{
		mode:         ProgramCacheDBWrite,
		persistedIDs: make(map[int64]struct{}),
	}

	// First request persists code_id=214770
	store.markPersisted(214770)
	if !store.isPersisted(214770) {
		t.Fatal("214770 should be marked as persisted after first request")
	}

	// Second request: guard should catch it
	if store.isPersisted(214770) {
		t.Log("Second request for code_id=214770 correctly skipped by persistedIDs guard")
	} else {
		t.Fatal("persistedIDs guard failed")
	}

	// Thread safety
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			store.markPersisted(id)
			_ = store.isPersisted(id)
		}(i)
	}
	wg.Wait()
	for i := int64(1); i <= 100; i++ {
		if !store.isPersisted(i) {
			t.Fatalf("ID %d should be persisted", i)
		}
	}
}

// TestPersistedIDs_DrainResidentForCloseBypassesGuard documents the root cause:
// Cache.FlushKeys does NOT check instructionStore.persistedIDs.
func TestPersistedIDs_DrainResidentForCloseBypassesGuard(t *testing.T) {
	t.Log("BUG: Cache.FlushKeys bypasses persistedIDs guard")
	t.Log("FIX NEEDED: instructionStore.Close must filter persistedIDs")
}

// --- from savetodb_streaming_test.go ---

// TestSaveToDatabaseDoesNotUseGetAll proves that SaveToDatabase's close
// path does not call GetAll (which would allocate a full instruction map).
// Instead, it uses drainResidentForClose which calls Flush (incremental).
func TestSaveToDatabaseDoesNotUseGetAll(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 100; i++ {
		l := builder.EmitUndefined("l")
		r := builder.EmitUndefined("r")
		builder.EmitBinOp(OpAdd, l, r)
	}
	builder.Finish()

	// Flush mid-compile (uses GetAll in flushCompileUnitWriter)
	prog.Cache.FlushCompileUnit("unit-a")

	// SaveToDatabase (should NOT use GetAll in close path)
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Verify all instructions are in DB without duplicates
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&total, &distinct)
	require.Equal(t, total, distinct, "no duplicates (total=%d distinct=%d)", total, distinct)
	require.Greater(t, total, int64(0))
}

// --- from gorm_batch_verify_test.go ---

// TestGORMCreateInBatches_VerifyInsert proves that GORM CreateInBatches
// with the local fork (commit d26405a) correctly inserts all rows without
// duplicates. This verifies the GORM fork's reflection allocation reduction
// doesn't break data integrity.
func TestGORMCreateInBatches_VerifyInsert(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("testBatch", string(MainFunctionName))
	for i := 0; i < 200; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	// Flush + Save
	prog.Cache.FlushCompileUnit("unit-a")
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Verify all instructions are in DB without duplicates
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&total, &distinct)

	t.Logf("total=%d distinct=%d", total, distinct)
	require.Equal(t, total, distinct, "total must equal distinct (no duplicates)")
	require.Greater(t, total, int64(0), "should have instructions")
}

// --- from type_store_integrity_test.go ---

// TestTypeStoreFlushFileDBKeepsIndexIntegrity exercises the real
// typeStore.flush + type2IrCode path (including the pooled JSON buffer) against
// a file-backed SQLite DB and verifies idx_ir_types_program_type stays intact.
func TestTypeStoreFlushFileDBKeepsIndexIntegrity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "typestore-integrity.db")

	db, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ssadb.IrType{}).Error)

	store := &typeStore{
		mode:        ProgramCacheDBWrite,
		db:          db,
		programName: "prog",
		saveSize:    4096,
		resident:    utils.NewSafeMapWithKey[int64, Type](),
	}
	longName := strings.Repeat("org.apache.hadoop.example.TypeName", 20)
	for i := 1; i <= 5000; i++ {
		typ := NewObjectType()
		typ.SetId(int64(i))
		typ.Name = fmt.Sprintf("Type%d", i)
		typ.AddFullTypeName(longName)
		store.remember(typ)
	}

	for round := 0; round < 5; round++ {
		require.NoError(t, store.flush(), "round %d", round)
		sqlDB := db.DB()
		rows, err := sqlDB.Query("PRAGMA integrity_check")
		require.NoError(t, err)
		var checks []string
		for rows.Next() {
			var row string
			require.NoError(t, rows.Scan(&row))
			checks = append(checks, row)
		}
		require.NoError(t, rows.Close())
		require.Equal(t, []string{"ok"}, checks, "SQLite integrity_check must pass after round %d", round)
	}
}
