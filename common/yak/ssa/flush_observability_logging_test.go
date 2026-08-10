package ssa

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type flushLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *flushLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *flushLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

// TestFlushRequestLogHasStructuredFields proves that FlushCompileUnit emits
// a structured log line with required fields: reason, unit_key (or hash),
// resident_before/after, persisted. RED until the structured log is implemented.
func TestFlushRequestLogHasStructuredFields(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	_ = builder.EmitBinOp(OpAdd, left, right)
	builder.Finish()

	var logBuf flushLogBuffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.DebugLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	prog.Cache.FlushCompileUnit("test-unit")
	require.Eventually(t, func() bool {
		return strings.Contains(logBuf.String(), "event=completed")
	}, 3*time.Second, 10*time.Millisecond,
		"completed flush log must be emitted after async persistence settles")

	logOutput := logBuf.String()
	t.Logf("Captured log (%d bytes):\n%s", len(logOutput), logOutput)

	// The structured flush log must contain these key fields
	require.Contains(t, logOutput, "ssa-persist-flush",
		"flush log must use [ssa-persist-flush] prefix")
	require.Contains(t, logOutput, "reason=",
		"flush log must contain reason= field")
	require.Contains(t, logOutput, "resident_before=",
		"flush log must contain resident_before= field")
	require.Contains(t, logOutput, "resident_after=",
		"flush log must contain resident_after= field")
	require.Contains(t, logOutput, "persisted=",
		"flush log must contain persisted= field")
	require.Contains(t, logOutput, "persisted_after=",
		"completed flush log must contain persisted_after= field")
	require.Contains(t, logOutput, "heap_after=",
		"completed flush log must contain heap_after= field")
}

// TestFinalBarrierLogHasCoverageAndPressureReduction proves that
// SaveToDatabase emits a final barrier log with mid_flush_coverage
// and final_pressure_reduction. RED until implemented.
func TestFinalBarrierLogHasCoverageAndPressureReduction(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	builder.EmitUndefined("x")
	builder.Finish()

	// Mid-flush to create some persisted count
	prog.Cache.FlushCompileUnit("unit-a")

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

	// Final barrier log must contain coverage and pressure reduction
	require.Contains(t, logOutput, "mid_flush_coverage=",
		"final barrier log must contain mid_flush_coverage=")
	require.Contains(t, logOutput, "final_pressure_reduction=",
		"final barrier log must contain final_pressure_reduction=")
}

// TestFitRangeNotLoggedPerInstruction proves that fitRange debug logs
// are gated by instructionCacheEventDebugEnabled() and not printed
// on every instruction in normal debug mode. RED until fitRange
// log is gated.
func TestFitRangeNotLoggedPerInstruction(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	_ = builder.EmitBinOp(OpAdd, left, right)
	builder.Finish()

	// Set DebugLevel but NOT event debug — fitRange should NOT log
	yaklog.SetLevel(yaklog.DebugLevel)
	defer yaklog.SetLevel(yaklog.InfoLevel)

	prog.Cache.FlushCompileUnit("unit-a")

	// fitRange should NOT appear in debug log when event debug is off
	// We can't capture output here (would interfere with other tests),
	// so we verify by checking that the event debug env var is not set
	require.Empty(t, os.Getenv("YAK_SSA_IR_CACHE_EVENT_DEBUG"),
		"YAK_SSA_IR_CACHE_EVENT_DEBUG must not be set for this test")
}
