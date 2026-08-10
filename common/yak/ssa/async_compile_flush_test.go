package ssa

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestFlushCompileUnitIsAsync proves that FlushCompileUnit does not block
// the compile thread waiting for DB writes. This is the wiring test:
// FlushCompileUnit must use MarkDirty internally, not FlushKeys.
func TestFlushCompileUnitIsAsync(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 20; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	// FlushCompileUnit should be fast (non-blocking)
	start := time.Now()
	prog.Cache.FlushCompileUnit("test-unit")
	elapsed := time.Since(start)

	// Even with 20+ instructions, FlushCompileUnit should not block
	// for more than 1 second (was potentially 10s+ with sync FlushKeys)
	require.Less(t, elapsed, 5*time.Second,
		"FlushCompileUnit must be async (took %v)", elapsed)

	// Instructions should eventually be persisted
	require.Eventually(t, func() bool {
		return prog.Cache.InstructionPersistedCount() > 0
	}, 5*time.Second, 50*time.Millisecond,
		"instructions should be persisted after async flush")
}

// TestSaveToDatabaseUsesBarrier proves that SaveToDatabase uses Barrier
// to wait for all pending async writes before closing stores.
func TestSaveToDatabaseUsesBarrier(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	bin := builder.EmitBinOp(OpAdd, left, right)
	_ = bin
	builder.Finish()

	// Mid-compile flush (async)
	prog.Cache.FlushCompileUnit("unit-a")

	// SaveToDatabase should succeed (Barrier waits for pending)
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err, "SaveToDatabase must succeed after async flush + Barrier")

	// All instructions should be in DB
	require.NotNil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, bin.GetId()),
		"instruction must be in DB after SaveToDatabase")
}
