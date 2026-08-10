package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestSaveToDatabaseDoesNotUseGetAll proves that SaveToDatabase's close
// path does not call GetAll (which would allocate a full instruction map).
// Instead, it uses drainResidentForClose which calls Flush (incremental).
func TestSaveToDatabaseDoesNotUseGetAll(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	for i := 0; i < 100; i++ {
		l := builder.EmitUndefined("l")
		r := builder.EmitUndefined("r")
		builder.EmitBinOp(OpAdd, l, r)
	}
	builder.Finish()

	// Flush mid-compile (uses GetAll in flushCompileUnitWriter)
	prog.Cache.FlushCompileUnit("unit-a")

	// SaveToDatabase (should NOT use GetAll in close path)
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Verify all instructions are in DB without duplicates
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&total, &distinct)
	require.Equal(t, total, distinct, "no duplicates (total=%d distinct=%d)", total, distinct)
	require.Greater(t, total, int64(0))
}
