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
