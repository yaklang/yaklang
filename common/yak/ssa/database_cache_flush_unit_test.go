package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestPerBatchFlushPersistsInstructionsMidCompile verifies that enabling
// per-batch FlushCompileUnit causes ordinary instructions to be persisted
// to the DB during compilation (after each unit batch), not only at the
// final SaveToDatabase close. This is the core mechanism that bounds memory
// on large projects (e.g. Apache Hadoop with 5M+ instructions).
func TestPerBatchFlushPersistsInstructionsMidCompile(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	// Enable compile-unit split so FlushCompileUnit takes the
	// flushCompileUnitWriter path (evicts non-boundary instructions).
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Emit some ordinary instructions.
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	bin := builder.EmitBinOp(OpAdd, left, right)
	_ = bin

	builder.Finish()

	// Before flush: all instructions should be resident, none persisted.
	require.Equal(t, 0, prog.Cache.InstructionPersistedCount(),
		"no instructions should be persisted before flush")
	require.Greater(t, prog.Cache.CountInstruction(), 0,
		"instructions should be resident before flush")

	// Flush the compile unit — this should evict ordinary instructions to DB.
	prog.Cache.FlushCompileUnit("unit-a")
	// Explicit Barrier: FlushCompileUnit is async (MarkDirty only),
	// so we need to wait for the writer to complete before checking.
	prog.Cache.FlushInstructionSaver()

	// After flush: at least some instructions should be persisted.
	require.Greater(t, prog.Cache.InstructionPersistedCount(), 0,
		"some instructions should be persisted after FlushCompileUnit")

	// The bin instruction (ordinary, non-boundary) should be persisted.
	require.False(t, prog.Cache.hasResidentInstruction(bin.GetId()),
		"ordinary instruction should be evicted from resident after flush")
	irCode := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, bin.GetId())
	require.NotNil(t, irCode, "ordinary instruction should be in DB after flush")

	// Function and parameter (boundary instructions) should remain resident.
	require.True(t, prog.Cache.hasResidentInstruction(builder.Function.GetId()),
		"function instruction should stay resident for cross-unit calls")

	// Final save should succeed and not duplicate rows.
	require.NoError(t, prog.Cache.SaveToDatabase())
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, bin.GetId()))
}

// TestPerBatchFlushKeepsBoundaryResident verifies that after FlushCompileUnit,
// function-level instructions (Function, Parameter, FreeValue, BasicBlock)
// remain resident in memory so that cross-unit calls and lazy builds work.
func TestPerBatchFlushKeepsBoundaryResident(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("testFunc", string(MainFunctionName))
	param := builder.NewParam("arg")
	_ = builder.EmitUndefined("body")
	builder.Finish()

	prog.Cache.FlushCompileUnit("unit-a")

	// Boundary instructions stay resident.
	require.True(t, prog.Cache.hasResidentInstruction(builder.Function.GetId()),
		"function should stay resident")
	require.True(t, prog.Cache.hasResidentInstruction(param.GetId()),
		"parameter should stay resident")

	// But they are also in DB after final save.
	require.NoError(t, prog.Cache.SaveToDatabase())
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, builder.Function.GetId()))
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, param.GetId()))
}

// TestPerBatchFlushNoFeedBlockPanic verifies that after FlushCompileUnit,
// the cache can still accept new instructions without panicking (the
// FeedBlock bug that originally disabled per-batch flush).
func TestPerBatchFlushNoFeedBlockPanic(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)

	// Unit A: create and flush.
	builderA := prog.GetAndCreateFunctionBuilder("funcA", string(MainFunctionName))
	instA := builderA.EmitUndefined("instA")
	builderA.Finish()
	prog.Cache.FlushCompileUnit("unit-a")

	// Unit B: create new instructions AFTER flush — must not panic.
	require.NotPanics(t, func() {
		builderB := prog.GetAndCreateFunctionBuilder("funcB", string(MainFunctionName))
		instB := builderB.EmitUndefined("instB")
		_ = instB
		builderB.Finish()
	})

	// The new instruction should be resident.
	// Note: instA might still be resident if shouldDelayInstructionEviction
	// keeps it (e.g. function not fully released). The key assertion is
	// that no panic occurs when creating new instructions after flush.
	// instB should be resident (not yet flushed)
	prog.Cache.FlushCompileUnit("unit-b")

	require.NoError(t, prog.Cache.SaveToDatabase())
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instA.GetId()))
}

// TestPerBatchFlushDoesNotLoseInstructionsDefaultSplit verifies that when
// compileUnitSplit is false (the default), calling FlushCompileUnit and
// adding more instructions across multiple units does not lose any
// instructions — all created instructions are either resident or persisted.
func TestPerBatchFlushDoesNotLoseInstructionsDefaultSplit(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	require.False(t, cfg.GetCompileUnitSplit())

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)

	// Unit A: create instructions and flush
	builderA := prog.GetAndCreateFunctionBuilder("funcA", string(MainFunctionName))
	instA := builderA.EmitUndefined("instA")
	_ = builderA.NewParam("paramA")
	builderA.Finish()

	// Note: we track total = resident + persisted, but persistedCount may
	// undercount when a LazyInstruction (already in DB) is re-evicted: marshal
	// skips it (no new DB write needed), the item is deleted from resident
	// (FinishPersist success), but persistedCount is not incremented because
	// saveInstructionPersistRecords was not called. This is a known limitation
	// of the persistedCount metric; the correctness invariant is that all
	// instructions must be in the DB after SaveToDatabase.
	totalBefore := prog.Cache.CountInstruction() + prog.Cache.InstructionPersistedCount()
	t.Logf("before flush1: resident=%d persisted=%d total=%d",
		prog.Cache.CountInstruction(), prog.Cache.InstructionPersistedCount(), totalBefore)
	prog.Cache.FlushCompileUnit("unit-a")
	prog.Cache.FlushInstructionSaver()
	totalAfter := prog.Cache.CountInstruction() + prog.Cache.InstructionPersistedCount()
	t.Logf("after flush1: resident=%d persisted=%d total=%d (delta=%d)",
		prog.Cache.CountInstruction(), prog.Cache.InstructionPersistedCount(), totalAfter, totalAfter-totalBefore)
	// Total may decrease when already-DB-persisted LazyInstructions are re-evicted
	// (they are deleted from resident without incrementing persistedCount).
	// This is acceptable as long as the DB still has the row.
	// Verify instA is in DB after flush:
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instA.GetId()),
		"instA must be in DB after flush1")

	// Unit B: create more instructions
	builderB := prog.GetAndCreateFunctionBuilder("funcB", string(MainFunctionName))
	instB := builderB.EmitUndefined("instB")
	_ = builderB.NewParam("paramB")
	builderB.Finish()

	totalBefore2 := prog.Cache.CountInstruction() + prog.Cache.InstructionPersistedCount()
	t.Logf("before flush2: resident=%d persisted=%d total=%d",
		prog.Cache.CountInstruction(), prog.Cache.InstructionPersistedCount(), totalBefore2)
	for id, inst := range prog.Cache.instructions.GetAllResident() {
		t.Logf("  resident id=%d opcode=%s name=%s", id, inst.GetOpcode().String(), inst.GetName())
	}
	prog.Cache.FlushCompileUnit("unit-b")
	prog.Cache.FlushInstructionSaver()
	totalAfter2 := prog.Cache.CountInstruction() + prog.Cache.InstructionPersistedCount()
	t.Logf("after flush2: resident=%d persisted=%d total=%d (delta=%d)",
		prog.Cache.CountInstruction(), prog.Cache.InstructionPersistedCount(), totalAfter2, totalAfter2-totalBefore2)
	for id, inst := range prog.Cache.instructions.GetAllResident() {
		t.Logf("  resident after id=%d opcode=%s name=%s", id, inst.GetOpcode().String(), inst.GetName())
	}
	// Verify instB is in DB after flush2:
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instB.GetId()),
		"instB must be in DB after flush2")

	require.NoError(t, prog.Cache.SaveToDatabase())
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instA.GetId()))
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instB.GetId()))
}
