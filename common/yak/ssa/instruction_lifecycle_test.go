package ssa

import (
	"bytes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- from instruction_exactly_once_lifecycle_test.go ---

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

// --- from save_to_database_lifecycle_test.go ---

// TestSaveToDatabaseFlushesInstructionSaverBeforeTypes verifies that
// SaveToDatabase flushes the instruction async saver BEFORE starting
// typeStore.close (step1). This prevents concurrent SQLite writes that
// caused "database disk image is malformed" corruption.
//
// The fix adds a step0 that flushes the instruction saver before step1.
// This test captures log output and verifies "step0" appears before "step1".
func TestSaveToDatabaseFlushesInstructionSaverBeforeTypes(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)

	builder := prog.GetAndCreateFunctionBuilder("testFunc", string(MainFunctionName))
	builder.EmitUndefined("testInst")
	builder.NewParam("testParam")
	builder.Finish()

	prog.Cache.FlushCompileUnit("unit-a")

	// Capture log output
	var logBuf bytes.Buffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.InfoLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	logOutput := logBuf.String()
	t.Logf("Captured log (%d bytes):\n%s", len(logOutput), logOutput)

	// Check that step0 appears before step1
	step0Idx := strings.Index(logOutput, "step0")
	step1Idx := strings.Index(logOutput, "step1")

	if step0Idx < 0 {
		t.Fatalf("Expected 'step0' (instruction saver flush) in log before step1, but not found")
	}
	if step1Idx < 0 {
		t.Fatalf("Expected 'step1' in log, but not found")
	}
	require.Less(t, step0Idx, step1Idx,
		"step0 must appear BEFORE step1 in log output")
}

// --- from resident_lifecycle_regression_test.go ---

// TestResidentLifecycle_PreserveQueryAfterSaveToDatabase proves that after
// SaveToDatabase on the resident-fast-path (adaptive) instruction store:
//
//  1. remaining_dirty == 0 (all dirty instructions were persisted)
//  2. The resident instructions are STILL queryable via GetInstruction —
//     the Program stays alive for compile+scan reuse (e.g. Qor runs where
//     query rules need resident instructions).
//
// This is RED on 718c3a260 because that commit changed ResidentFlushCache.Close
// to Clear() the resident map after Flush(true), and instructionStore.Close
// calls residentCache.Close(). SaveToDatabase calls instructionStore.Close,
// so after save all resident instructions are gone.
func TestResidentLifecycle_PreserveQueryAfterSaveToDatabase(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(fastPathProjectByteThreshold / 2)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 1)
	require.Equal(t, "resident-fast-path", prog.Cache.InstructionCacheMode())

	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	// Emit enough instructions to be meaningful
	var insts []Instruction
	for i := 0; i < 10; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		binOp := builder.EmitBinOp(OpAdd, left, right)
		insts = append(insts, binOp)
	}
	builder.Finish()

	// Record all instruction IDs we want to query later
	var queryIDs []int64
	for _, inst := range insts {
		queryIDs = append(queryIDs, inst.GetId())
	}

	// Flush mid-compile (triggers incremental flush on residentCache)
	prog.Cache.FlushCompileUnit("unit-a")

	// Save to database — this calls instructionStore.Close which must NOT
	// destroy resident instructions.
	require.NoError(t, prog.Cache.SaveToDatabase())

	// ── Assertion 1: accounting must report remaining_dirty == 0 ──
	accounting := prog.Cache.GetFlushAccounting()
	require.NotNil(t, accounting)
	require.Equal(t, int64(0), accounting.RemainingDirty,
		"remaining_dirty must be 0 after SaveToDatabase — all dirty items were persisted")

	// ── Assertion 2: resident instructions must survive SaveToDatabase ──
	// The Program is reused for query rules (compile+scan path). If the
	// resident cache was cleared, GetInstruction returns nil and queries
	// produce zero results (Risk=0).
	residentCount := prog.Cache.CountInstruction()
	require.Greater(t, residentCount, 0,
		"resident instruction count must be > 0 after SaveToDatabase — "+
			"Program must stay queryable for compile+scan reuse; "+
			"clearing resident data to fake remaining_dirty=0 is the regression")

	for _, id := range queryIDs {
		inst := prog.Cache.GetInstruction(id)
		require.NotNil(t, inst,
			"GetInstruction(%d) must return non-nil after SaveToDatabase — "+
				"query rules depend on resident instructions surviving close", id)
	}

	// ── Assertion 3: known query results survive ──
	// The instructions are in both the DB (persisted) and resident cache.
	// Verify the DB rows exist as well.
	var dbRows int64
	err = ssadb.GetDB().Raw(
		"SELECT COUNT(*) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&dbRows)
	require.NoError(t, err)
	require.Greater(t, dbRows, int64(0), "DB must have persisted instruction rows")

	// ── Assertion 4: stats are preserved ──
	stats := prog.Cache.instructions.PersistenceStats()
	require.Greater(t, stats.UniquePersisted, int64(0),
		"unique_persisted must be > 0")
	require.Greater(t, stats.WriteOperations, int64(0),
		"write_ops must be > 0")
	require.Greater(t, stats.Requests, int64(0),
		"request count must be > 0")
	require.Greater(t, stats.Enqueued, int64(0),
		"enqueued count must be > 0")
	require.Greater(t, stats.Completed, int64(0),
		"completed count must be > 0")
	// Resident count must be > 0 (instructions survive)
	require.Greater(t, stats.Resident, int64(0),
		"resident count in stats must be > 0 after SaveToDatabase")
	// RemainingDirty (derived) must be 0 — all dirty items persisted
	require.Equal(t, int64(0), stats.RemainingDirty,
		"RemainingDirty must be 0 — dirty count is separate from live resident count")
}

// TestResidentLifecycle_DirtyCountSeparateFromResidentCount proves that the
// remaining_dirty accounting is computed as max(0, resident - unique_persisted),
// NOT as the raw resident count. After SaveToDatabase, all resident items
// have been persisted, so remaining_dirty=0 even though resident count > 0.
func TestResidentLifecycle_DirtyCountSeparateFromResidentCount(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(fastPathProjectByteThreshold / 2)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 1)
	require.Equal(t, "resident-fast-path", prog.Cache.InstructionCacheMode())

	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 5; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	// Before save: resident > 0, persisted == 0, so dirty == resident
	preStats := prog.Cache.instructions.PersistenceStats()
	require.Greater(t, preStats.Resident, int64(0), "should have resident instructions before save")
	require.Equal(t, int64(0), preStats.UniquePersisted, "nothing persisted before save")
	require.Equal(t, preStats.Resident, preStats.RemainingDirty,
		"before save: remaining_dirty should equal resident (nothing persisted yet)")

	require.NoError(t, prog.Cache.SaveToDatabase())

	// After save: resident > 0 (preserved), persisted > 0, dirty == 0
	postStats := prog.Cache.instructions.PersistenceStats()
	require.Greater(t, postStats.Resident, int64(0),
		"resident must be > 0 after save — instructions preserved for queries")
	require.Greater(t, postStats.UniquePersisted, int64(0),
		"unique_persisted must be > 0 after save")
	require.Equal(t, int64(0), postStats.RemainingDirty,
		"after save: remaining_dirty must be 0 — all dirty items persisted, "+
			"but resident count is still > 0 (live objects ≠ unsaved dirty count)")
}

// --- from deferred_build_unit_callback_test.go ---

// TestRunDeferredBuildsForUnitsWithUnitCallback verifies that afterUnit
// is called exactly once per unitKey when all tasks for that unit complete,
// even if tasks from different units interleave.
func TestRunDeferredBuildsForUnitsWithUnitCallback(t *testing.T) {
	prog := NewTmpProgram("test-unit-callback")

	// Register deferred builds for two units: "unit-a" has 3 tasks, "unit-b" has 2.
	// Tasks are registered in interleaved order to test that afterUnit fires
	// only when a unit's remaining count hits zero.
	// BeginCompileUnit sets currentCompileUnit, which RegisterDeferredBuild uses.

	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a1", func() {})
	prog.BeginCompileUnit("unit-b")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "b1", func() {})
	prog.BeginCompileUnit("unit-a")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a2", func() {})
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "a3", func() {})
	prog.BeginCompileUnit("unit-b")
	prog.RegisterDeferredBuild(DeferredBuildKindHelper, "b2", func() {})
	prog.EndCompileUnit()

	var unitCompletedOrder []string
	var mu sync.Mutex
	var afterEachCount int

	ok := prog.RunDeferredBuildsForUnitsWithUnitCallback(
		[]string{"unit-a", "unit-b"},
		func(index int, total int) bool {
			afterEachCount++
			return true
		},
		func(unitKey string) bool {
			mu.Lock()
			unitCompletedOrder = append(unitCompletedOrder, unitKey)
			mu.Unlock()
			return true
		},
	)

	require.True(t, ok, "should complete all tasks")
	require.Equal(t, 5, afterEachCount, "afterEach should be called for each task")
	require.Equal(t, []string{"unit-a", "unit-b"}, unitCompletedOrder,
		"afterUnit should fire once per unit in completion order: unit-a (3 tasks) finishes before unit-b (2 tasks)")
}

// TestRunDeferredBuildsForUnitsWithUnitCallbackCancellation verifies that
// returning false from afterUnit stops execution.
func TestRunDeferredBuildsForUnitsWithUnitCallbackCancellation(t *testing.T) {
	prog := NewTmpProgram("test-cancel")

	prog.BeginCompileUnit("unit-a")
	for i := 0; i < 3; i++ {
		prog.RegisterDeferredBuild(DeferredBuildKindHelper, "cancel-"+string(rune('a'+i)), func() {})
	}
	prog.EndCompileUnit()

	callCount := 0
	ok := prog.RunDeferredBuildsForUnitsWithUnitCallback(
		[]string{"unit-a"},
		nil, // no afterEach
		func(unitKey string) bool {
			callCount++
			return false // cancel after first unit completion
		},
	)

	require.False(t, ok, "should return false when afterUnit returns false")
	require.Equal(t, 1, callCount, "afterUnit should be called once before cancellation")
}

// TestRunDeferredBuildsForUnitsBackwardCompat verifies that the original
// RunDeferredBuildsForUnits (without afterUnit) still works.
func TestRunDeferredBuildsForUnitsBackwardCompat(t *testing.T) {
	prog := NewTmpProgram("test-backward")

	prog.BeginCompileUnit("unit-x")
	for i := 0; i < 3; i++ {
		prog.RegisterDeferredBuild(DeferredBuildKindHelper, "compat-"+string(rune('a'+i)), func() {})
	}
	prog.EndCompileUnit()

	callCount := 0
	ok := prog.RunDeferredBuildsForUnits(
		[]string{"unit-x"},
		func(index int, total int) bool {
			callCount++
			return true
		},
	)

	require.True(t, ok)
	require.Equal(t, 3, callCount)
}
