package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCleanBaselineReleasesOldProgram proves that after SaveToDatabase,
// CleanBaseline nil's out all compilation state and marks the cache
// as cleaned. This is the v3 step E clean baseline.
func TestCleanBaselineReleasesOldProgram(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 100; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	prog.Cache.FlushCompileUnit("unit-a")
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Before CleanBaseline: cache has stores
	require.False(t, prog.Cache.IsCleaned(),
		"IsCleaned must be false before CleanBaseline")

	// CleanBaseline: release all compilation state
	prog.Cache.CleanBaseline()

	// After CleanBaseline: cache is cleaned
	require.True(t, prog.Cache.IsCleaned(),
		"IsCleaned must be true after CleanBaseline")

	// All stores should be nil'd
	require.Nil(t, prog.Cache.instructions,
		"instructions must be nil after CleanBaseline")
	require.Nil(t, prog.Cache.types,
		"types must be nil after CleanBaseline")
	require.Nil(t, prog.Cache.indexes,
		"indexes must be nil after CleanBaseline")
	require.Nil(t, prog.Cache.sources,
		"sources must be nil after CleanBaseline")

	// DB data should still exist
	require.Greater(t, prog.Cache.InstructionPersistedCount(), 0,
		"instructions should still be persisted in DB after CleanBaseline")

	// CountInstruction should be 0 (no resident)
	require.Equal(t, 0, prog.Cache.CountInstruction(),
		"CountInstruction must be 0 after CleanBaseline")
}
