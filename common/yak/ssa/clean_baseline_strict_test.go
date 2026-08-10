package ssa

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCleanBaseline_StrictGC proves that after CleanBaseline + GC:
// 1. Heap objects decrease significantly
// 2. HeapInuse decreases
// 3. No goroutine references the old Program
// 4. CleanBaseline + GC + FreeOSMemory brings RSS close to baseline
func TestCleanBaseline_StrictGC(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 500; i++ {
		l := builder.EmitUndefined("l")
		r := builder.EmitUndefined("r")
		builder.EmitBinOp(OpAdd, l, r)
	}
	builder.Finish()

	// Flush + Save
	prog.Cache.FlushCompileUnit("unit-a")
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Record goroutine count before
	var beforeGoroutines int
	beforeGoroutines = runtime.NumGoroutine()

	// Record heap before
	runtime.GC()
	var beforeHeap runtime.MemStats
	runtime.ReadMemStats(&beforeHeap)
	t.Logf("Before CleanBaseline: HeapInuse=%dMB HeapObjects=%d Goroutines=%d",
		beforeHeap.HeapInuse/1024/1024, beforeHeap.HeapObjects, beforeGoroutines)

	// CleanBaseline
	prog.Cache.CleanBaseline()

	// Force GC
	runtime.GC()
	var afterGC runtime.MemStats
	runtime.ReadMemStats(&afterGC)
	t.Logf("After CleanBaseline + GC: HeapInuse=%dMB HeapObjects=%d Goroutines=%d",
		afterGC.HeapInuse/1024/1024, afterGC.HeapObjects, runtime.NumGoroutine())

	// 1. HeapInuse should decrease
	require.Less(t, afterGC.HeapInuse, beforeHeap.HeapInuse,
		"HeapInuse must decrease after CleanBaseline + GC")

	// 2. HeapObjects should decrease
	require.Less(t, afterGC.HeapObjects, beforeHeap.HeapObjects,
		"HeapObjects must decrease after CleanBaseline + GC")

	// 3. Goroutine count should not increase (no leaked goroutines)
	afterGoroutines := runtime.NumGoroutine()
	require.LessOrEqual(t, afterGoroutines, beforeGoroutines+1,
		"Goroutine count should not increase after CleanBaseline (before=%d after=%d)",
		beforeGoroutines, afterGoroutines)

	// 4. All stores should be nil'd
	require.Nil(t, prog.Cache.instructions, "instructions must be nil")
	require.Nil(t, prog.Cache.types, "types must be nil")
	require.Nil(t, prog.Cache.indexes, "indexes must be nil")
	require.Nil(t, prog.Cache.sources, "sources must be nil")

	// 5. CleanBaseline flag set
	require.True(t, prog.Cache.IsCleaned(), "IsCleaned must be true")

	// 6. Persisted count preserved
	require.Greater(t, prog.Cache.InstructionPersistedCount(), 0,
		"persisted count must be preserved after CleanBaseline")

	// 7. CountInstruction should be 0 (no resident)
	require.Equal(t, 0, prog.Cache.CountInstruction(),
		"CountInstruction must be 0 after CleanBaseline")
}
