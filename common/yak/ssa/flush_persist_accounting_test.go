package ssa

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestC_FinalBarrierAccounting proves that SaveToDatabase must produce
// verifiable accounting: total = already_persisted + remaining_dirty,
// and mid_flush_coverage/final_pressure_reduction are calculable.
// Currently SaveToDatabase logs resident+persisted=total in step3,
// but does NOT expose FlushAccounting for programmatic verification.
// This test will be RED until FlushAccounting is implemented.

func TestC_FinalBarrierAccounting(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Emit ordinary instructions
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	bin := builder.EmitBinOp(OpAdd, left, right)
	builder.Finish()

	totalBefore := int64(prog.Cache.CountInstruction() + int(prog.Cache.InstructionPersistedCount()))
	require.Greater(t, totalBefore, int64(0), "should have instructions")

	// Mid-compile flush
	prog.Cache.FlushCompileUnit("unit-a")
	prog.Cache.FlushInstructionSaver()

	persistedAfterFlush := prog.Cache.InstructionPersistedCount()

	// Final SaveToDatabase
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// After SaveToDatabase, all instructions should be persisted.
	// GetFlushAccounting returns the final accounting snapshot.
	accounting := prog.Cache.GetFlushAccounting()
	require.NotNil(t, accounting, "GetFlushAccounting must return non-nil after SaveToDatabase")

	// total = already_persisted + remaining_dirty (invariant)
	require.Equal(t, accounting.InstructionsTotal, accounting.AlreadyPersisted+accounting.RemainingDirty,
		"total must equal already_persisted + remaining_dirty")

	// After SaveToDatabase: remaining_dirty should be 0, already_persisted should equal total
	require.Equal(t, int64(0), accounting.RemainingDirty,
		"remaining_dirty must be 0 after SaveToDatabase")
	require.Equal(t, accounting.InstructionsTotal, accounting.AlreadyPersisted,
		"already_persisted must equal total after SaveToDatabase")

	// already_persisted should be >= the count persisted by mid-flush alone
	require.GreaterOrEqual(t, accounting.AlreadyPersisted, int64(persistedAfterFlush),
		"already_persisted after final save must include mid-flush persisted count")

	// mid_flush_coverage = already_persisted / total (should be 1.0 after final save)
	if accounting.InstructionsTotal > 0 {
		expected := float64(accounting.AlreadyPersisted) / float64(accounting.InstructionsTotal)
		require.InDelta(t, expected, accounting.MidFlushCoverage, 0.001,
			"mid_flush_coverage must equal already_persisted/total")
	}

	// final_pressure_reduction = 1 - remaining_dirty / total (should be 1.0 after final save)
	if accounting.InstructionsTotal > 0 {
		expected := 1.0 - float64(accounting.RemainingDirty)/float64(accounting.InstructionsTotal)
		require.InDelta(t, expected, accounting.FinalPressureReduction, 0.001,
			"final_pressure_reduction must equal 1 - remaining_dirty/total")
	}

	// Verify the instruction is in DB
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, bin.GetId()))
}

// TestE_MidFlushReducesFinalRemaining proves that after a mid-compile flush,
// if the writer actually completes, the final SaveToDatabase has fewer
// remaining items to persist. This test verifies that mid-flush coverage
// is > 0 and final remaining < total.
func TestE_MidFlushReducesFinalRemaining(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Emit several ordinary instructions
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	bin := builder.EmitBinOp(OpAdd, left, right)
	left2 := builder.EmitUndefined("left2")
	right2 := builder.EmitUndefined("right2")
	bin2 := builder.EmitBinOp(OpSub, left2, right2)
	_ = bin
	_ = bin2
	builder.Finish()

	totalBefore := int64(prog.Cache.CountInstruction() + int(prog.Cache.InstructionPersistedCount()))
	require.Greater(t, totalBefore, int64(0))

	// Mid-compile flush — should persist ordinary instructions
	prog.Cache.FlushCompileUnit("unit-a")
	prog.Cache.FlushInstructionSaver()

	persistedAfterFlush := int64(prog.Cache.InstructionPersistedCount())
	remainingAfterFlush := int64(prog.Cache.CountInstruction())

	// Mid-flush should have persisted at least some instructions
	require.Greater(t, persistedAfterFlush, int64(0),
		"mid-flush should have persisted some instructions")

	// remaining should be less than total (some were evicted)
	require.Less(t, remainingAfterFlush, totalBefore,
		"remaining after mid-flush should be less than total")

	// Final SaveToDatabase
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	accounting := prog.Cache.GetFlushAccounting()
	require.NotNil(t, accounting)

	// After final save: already_persisted should include mid-flush persisted
	require.GreaterOrEqual(t, accounting.AlreadyPersisted, int64(persistedAfterFlush),
		"already_persisted after final save must include mid-flush persisted count")

	// mid_flush_coverage should be > 0
	require.Greater(t, accounting.MidFlushCoverage, 0.0,
		"mid_flush_coverage must be > 0 when mid-flush occurred")
}

func TestFastPathFinalAccountingUsesUniquePersistedRows(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(fastPathProjectByteThreshold / 2)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, filesys.NewVirtualFs(), "", 1)
	require.Equal(t, "resident-fast-path", prog.Cache.InstructionCacheMode())
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 8; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	prog.Cache.FlushCompileUnit("unit-a")
	first := builder.Function.GetId()
	inst := prog.Cache.GetInstruction(first)
	require.NotNil(t, inst)
	inst.SetExtern(true)

	var logBuf bytes.Buffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.InfoLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	require.NoError(t, prog.Cache.SaveToDatabase())

	var rows, distinct int64
	err = ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&rows, &distinct)
	require.NoError(t, err)
	require.Greater(t, rows, int64(0))
	require.Equal(t, rows, distinct)
	var updated ssadb.IrCode
	err = ssadb.GetDB().Where("program_name = ? AND code_id = ?", programName, first).First(&updated).Error
	require.NoError(t, err)
	require.True(t, updated.IsExternal, "final flush must upsert the mid-flush row")

	accounting := prog.Cache.GetFlushAccounting()
	require.Equal(t, rows, accounting.InstructionsTotal)
	require.Equal(t, rows, accounting.AlreadyPersisted)
	require.Equal(t, int64(0), accounting.RemainingDirty)
	require.Equal(t, int64(0), accounting.RemainingPending)

	logOutput := logBuf.String()
	for _, field := range []string{
		"request=",
		"enqueued=",
		"completed=",
		"write_ops=",
		"unique_persisted=",
		"resident=",
		"pending=",
	} {
		require.True(t, strings.Contains(logOutput, field), "summary must include %s", field)
	}
}
