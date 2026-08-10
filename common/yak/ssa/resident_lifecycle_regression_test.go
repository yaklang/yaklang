package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

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
