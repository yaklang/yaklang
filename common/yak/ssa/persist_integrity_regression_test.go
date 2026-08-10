package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

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
