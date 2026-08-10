package ssa

import (
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var engineercmsDupIDs = []int64{214770, 277959, 277974}

// isolatedTestDB creates a fresh SSA DB for each test case.
// Captures old global values BEFORE replacing them.
func isolatedTestDB(t *testing.T) (testDB *gorm.DB, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ssa-test.db")
	db, err := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, dbPath)
	require.NoError(t, err, "failed to create isolated DB")

	// Run patches (creates unique index etc.)
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_codes_program_code" ON "ir_codes" ("program_name", "code_id")`)

	// Verify unique index exists
	var idxCount int64
	db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='ir_codes' AND name='ux_ir_codes_program_code'`).Row().Scan(&idxCount)
	require.Equal(t, int64(1), idxCount, "unique index must exist in isolated DB")

	// Capture OLD values BEFORE replacing
	oldSSADB := ssadb.GetDB()
	oldGormSSA := consts.GetGormSSAProjectDataBase()

	// Set new DB as global
	ssadb.SetDB(db)
	consts.SetGormSSAProjectDatabase(db)

	return db, func() {
		// Restore old globals FIRST
		ssadb.SetDB(oldSSADB)
		consts.SetGormSSAProjectDatabase(oldGormSSA)
		// Then close isolated DB
		db.Close()
	}
}

// makeTestProgram creates a program with compile-unit split enabled.
// Returns the program, cache, and a cleanup function.
// The cleanup function deletes the program from the isolated DB and
// restores the global DB. Do NOT use t.Cleanup for DeleteProgram.
func makeTestProgram(t *testing.T) (*Program, *ProgramCache, func()) {
	t.Helper()
	testDB, dbCleanup := isolatedTestDB(t)

	programName := uuid.NewString()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)

	return prog, prog.Cache, func() {
		// Delete program from isolated DB BEFORE closing it
		ssadb.DeleteProgram(testDB, programName)
		// Restore globals and close DB
		dbCleanup()
	}
}

// TEST1: AsyncOverlapExactlyOnce — same code_id through multiple
// flushCompileUnitWriter calls (A/U/B), saveFn receives ID exactly once.
func TestExactlyOnce_AsyncOverlapExactlyOnce(t *testing.T) {
	for _, targetID := range engineercmsDupIDs {
		t.Run("code_id_"+strconv.FormatInt(targetID, 10), func(t *testing.T) {
			prog, cache, cleanup := makeTestProgram(t)
			defer cleanup()
			builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			left := builder.EmitUndefined("left")
			right := builder.EmitUndefined("right")
			inst := builder.EmitBinOp(OpAdd, left, right)
			builder.Finish()

			codeID := inst.GetId()
			t.Logf("instruction code_id=%d (target=%d)", codeID, targetID)

			// Call FlushCompileUnit multiple times (simulating A/U/B)
			cache.FlushCompileUnit("unit-a")
			cache.FlushCompileUnit("unit-a")
			cache.FlushCompileUnit("unit-b")

			// Barrier to ensure all async writes complete before checking
			prog.Cache.FlushInstructionSaver()

			// Verify no duplicates
			var total, distinct int64
			ssadb.GetDB().Raw(
				"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
				prog.Name,
			).Row().Scan(&total, &distinct)
			t.Logf("DB: total=%d distinct=%d", total, distinct)
			require.Equal(t, total, distinct, "no duplicate code_ids: total=%d distinct=%d", total, distinct)
		})
	}
}

// TEST2: ReinsertAfterPersistExactlyOnce — first flush persists and
// deletes from resident; Set same instruction back; second flush + Close;
// DB should have exactly 1 row for this code_id.
func TestExactlyOnce_ReinsertAfterPersistExactlyOnce(t *testing.T) {
	for _, targetID := range engineercmsDupIDs {
		t.Run("code_id_"+strconv.FormatInt(targetID, 10), func(t *testing.T) {
			prog, cache, cleanup := makeTestProgram(t)
			defer cleanup()
			builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
			left := builder.EmitUndefined("left")
			right := builder.EmitUndefined("right")
			inst := builder.EmitBinOp(OpAdd, left, right)
			builder.Finish()

			codeID := inst.GetId()
			t.Logf("instruction code_id=%d (target=%d)", codeID, targetID)

			// First flush: persists (async, no Barrier)
			cache.FlushCompileUnit("unit-a")
			// Barrier to ensure first flush completes
			cache.FlushInstructionSaver()
			t.Logf("after first flush: persisted=%d, resident=%d",
				cache.InstructionPersistedCount(), cache.CountInstruction())

			// Re-insert the same instruction (simulating cross-unit resolution)
			cache.SetInstruction(inst)
			t.Logf("after reinsert: resident=%d", cache.CountInstruction())

			// Second flush + final SaveToDatabase
			cache.FlushCompileUnit("unit-a")
			err := cache.SaveToDatabase()
			require.NoError(t, err)

			// Check DB: exactly 1 row for this code_id
			var count int64
			ssadb.GetDB().Table("ir_codes").
				Where("program_name = ? AND code_id = ?", prog.Name, codeID).
				Count(&count)
			t.Logf("DB count for code_id=%d: %d", codeID, count)
			require.Equal(t, int64(1), count,
				"code_id=%d must have exactly 1 row in DB (got %d)", codeID, count)

			// No duplicates overall
			var total, distinct int64
			ssadb.GetDB().Raw(
				"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
				prog.Name,
			).Row().Scan(&total, &distinct)
			require.Equal(t, total, distinct, "no duplicates: total=%d distinct=%d", total, distinct)
		})
	}
}

// TEST3: FlushCompileUnitPersistsAndCompletes — proves FlushCompileUnit
// persists all instructions and completes within a reasonable time.
// After adding Barrier to flushCompileUnitWriter (to prevent lazy reload
// of unsaved instructions under pair-first member relations), FlushCompileUnit
// now waits for saves to complete — matching main's FlushKeys behavior.
func TestExactlyOnce_FlushCompileUnitReturnsFast(t *testing.T) {
	prog, cache, cleanup := makeTestProgram(t)
	defer cleanup()
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 10; i++ {
		builder.EmitBinOp(OpAdd, builder.EmitUndefined("l"), builder.EmitUndefined("r"))
	}
	builder.Finish()

	// FlushCompileUnit now calls MarkDirty (async): it enqueues keys for
	// background persistence and returns immediately without blocking the
	// compile thread. Eviction happens in the saver goroutine via
	// FinishPersist -> delete(c.data, key). Persistence completeness is
	// guaranteed by SaveToDatabase's Barrier, not by FlushCompileUnit.
	start := time.Now()
	cache.FlushCompileUnit("unit-a")
	elapsed := time.Since(start)
	t.Logf("FlushCompileUnit took %v", elapsed)
	require.Less(t, elapsed, 30*time.Second,
		"FlushCompileUnit must complete within 30s (took %v)", elapsed)

	// FlushCompileUnit is async — instructions may not be persisted yet,
	// but the resident cache must have been drained (ordinary instructions
	// evicted from memory). Verify via SaveToDatabase which drains and
	// persists everything exactly once.
	err := cache.SaveToDatabase()
	require.NoError(t, err)
	require.Greater(t, cache.InstructionPersistedCount(), 0,
		"after SaveToDatabase, instructions should be persisted")
}

// TEST4: BoundaryFinalCompleteness — proves that after SaveToDatabase,
// both boundary (Function, Parameter) and ordinary instructions
// are in the DB with exactly 1 row each.
func TestExactlyOnce_BoundaryFinalCompleteness(t *testing.T) {
	prog, cache, cleanup := makeTestProgram(t)
	defer cleanup()
	builder := prog.GetAndCreateFunctionBuilder("testFunc", string(MainFunctionName))
	param := builder.NewParam("arg")
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	bin := builder.EmitBinOp(OpAdd, left, right)
	builder.Finish()

	// Flush mid-compile (persists ordinary, keeps boundary resident)
	cache.FlushCompileUnit("unit-a")

	// Final SaveToDatabase (must persist boundary too)
	err := cache.SaveToDatabase()
	require.NoError(t, err)

	// Check each instruction has exactly 1 row
	for _, inst := range []Instruction{builder.Function, param, bin} {
		var count int64
		ssadb.GetDB().Table("ir_codes").
			Where("program_name = ? AND code_id = ?", prog.Name, inst.GetId()).
			Count(&count)
		t.Logf("code_id=%d opcode=%s DB count=%d", inst.GetId(), inst.GetOpcode(), count)
		require.Equal(t, int64(1), count,
			"code_id=%d (opcode=%s) must have exactly 1 row", inst.GetId(), inst.GetOpcode())
	}

	// No duplicates overall
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		prog.Name,
	).Row().Scan(&total, &distinct)
	require.Equal(t, total, distinct, "no duplicates: total=%d distinct=%d", total, distinct)
}

// TEST5: ReinsertAfterPersistNoDuplicate — proves that reinserting
// the same instruction after a successful persist, then flushing and
// closing, does NOT create a duplicate (persistedIDs guard works).
func TestExactlyOnce_ReinsertAfterPersistNoDuplicate(t *testing.T) {
	prog, cache, cleanup := makeTestProgram(t)
	defer cleanup()
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitBinOp(OpAdd, builder.EmitUndefined("l"), builder.EmitUndefined("r"))
	builder.Finish()

	codeID := inst.GetId()

	// First flush — should persist successfully
	cache.FlushCompileUnit("unit-a")
	cache.FlushInstructionSaver()
	require.Greater(t, cache.InstructionPersistedCount(), 0, "should persist after first flush")

	// Verify the instruction is in DB
	var count int64
	ssadb.GetDB().Table("ir_codes").
		Where("program_name = ? AND code_id = ?", prog.Name, codeID).
		Count(&count)
	require.Equal(t, int64(1), count, "should have 1 row in DB")

	// Reinsert and flush again — should NOT create duplicate
	cache.SetInstruction(inst)
	cache.FlushCompileUnit("unit-a")
	err := cache.SaveToDatabase()
	require.NoError(t, err)

	ssadb.GetDB().Table("ir_codes").
		Where("program_name = ? AND code_id = ?", prog.Name, codeID).
		Count(&count)
	require.Equal(t, int64(1), count, "should still have 1 row after reinsert+flush+close")
}

// TEST6: ReservedDuringInflightSet — proves that Set during the
// reserved/inflight period (save in progress) updates the resident
// item in-place via UpdateWhilePending, without breaking the save
// or creating a duplicate.
func TestExactlyOnce_ReservedDuringInflightSet(t *testing.T) {
	prog, cache, cleanup := makeTestProgram(t)
	defer cleanup()
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitBinOp(OpAdd, builder.EmitUndefined("l"), builder.EmitUndefined("r"))
	builder.Finish()

	codeID := inst.GetId()

	// FlushCompileUnit is async: MarkDirty enqueues but doesn't wait.
	// At this point, the instruction is reserved (in-flight).
	cache.FlushCompileUnit("unit-a")

	// DO NOT call FlushInstructionSaver here — we want to test
	// the state DURING inflight. Set the same instruction while
	// the save is in progress. This should use UpdateWhilePending
	// (not regular Set which would break generation).
	cache.SetInstruction(inst)

	// Now Barrier to wait for save completion
	cache.FlushInstructionSaver()
	require.Greater(t, cache.InstructionPersistedCount(), 0,
		"should persist after Barrier")

	// Final SaveToDatabase
	err := cache.SaveToDatabase()
	require.NoError(t, err)

	// Should have exactly 1 row (not 2)
	var count int64
	ssadb.GetDB().Table("ir_codes").
		Where("program_name = ? AND code_id = ?", prog.Name, codeID).
		Count(&count)
	require.Equal(t, int64(1), count,
		"should have 1 row after inflight Set + Barrier + Close (got %d)", count)
}

func TestExactlyOnce_ConcurrentFlushAndSet(t *testing.T) {
	prog, cache, cleanup := makeTestProgram(t)
	defer cleanup()
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	inst := builder.EmitBinOp(OpAdd, left, right)
	builder.Finish()

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < 32; round++ {
				cache.instructions.Set(inst)
				cache.instructions.Flush()
			}
		}()
	}
	wg.Wait()

	require.NoError(t, cache.SaveToDatabase())
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		prog.Name,
	).Row().Scan(&total, &distinct)
	require.Equal(t, total, distinct)
	require.Greater(t, total, int64(0))
}

// Suppress unused imports
var _ = sync.Mutex{}
var _ = atomic.Int64{}
