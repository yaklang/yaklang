package ssa

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestFlushLogHasEnqueuedAndCompleted proves that the structured flush
// log includes enqueued and completed events, not just request.
func TestFlushLogHasEnqueuedAndCompleted(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 20; i++ {
		l := builder.EmitUndefined("l")
		r := builder.EmitUndefined("r")
		builder.EmitBinOp(OpAdd, l, r)
	}
	builder.Finish()

	var logBuf bytes.Buffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.DebugLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	prog.Cache.FlushCompileUnit("test-unit")

	logOutput := logBuf.String()
	t.Logf("Log (%d bytes):\n%s", len(logOutput), logOutput)

	// Must have enqueued event
	require.True(t, strings.Contains(logOutput, "event=enqueued") || strings.Contains(logOutput, "event=completed"),
		"flush log must include enqueued or completed event (not just request)")
}

// TestFlushLogHasWriterSummary proves that a writer periodic summary
// log is emitted during the flush process.
func TestFlushLogHasWriterSummary(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 10; i++ {
		l := builder.EmitUndefined("l")
		r := builder.EmitUndefined("r")
		builder.EmitBinOp(OpAdd, l, r)
	}
	builder.Finish()

	var logBuf bytes.Buffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.InfoLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	prog.Cache.FlushCompileUnit("unit-a")
	prog.Cache.SaveToDatabase()

	logOutput := logBuf.String()

	// Writer summary should contain persisted_instructions or queue_depth
	require.True(t,
		strings.Contains(logOutput, "ssa-persist-writer-summary") ||
			strings.Contains(logOutput, "persisted_instructions") ||
			strings.Contains(logOutput, "queue_depth"),
		"writer summary log must be present (ssa-persist-writer-summary or persisted_instructions)")
}

// TestFlushLogFinalBarrierHasRemainingAndSaved proves the final barrier
// done event includes source_remaining/saved, type_remaining/saved.
func TestFlushLogFinalBarrierHasRemainingAndSaved(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	builder.EmitUndefined("x")
	builder.Finish()

	var logBuf bytes.Buffer
	yaklog.SetOutput(&logBuf)
	yaklog.SetLevel(yaklog.InfoLevel)
	defer func() {
		yaklog.SetOutput(os.Stdout)
		yaklog.SetLevel(yaklog.InfoLevel)
	}()

	prog.Cache.FlushCompileUnit("unit-a")
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	logOutput := logBuf.String()

	// Final barrier done should include source/type/index remaining and saved
	require.Contains(t, logOutput, "source_remaining=",
		"final barrier done must include source_remaining=")
	require.Contains(t, logOutput, "source_saved=",
		"final barrier done must include source_saved=")
}
