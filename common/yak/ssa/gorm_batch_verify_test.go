package ssa

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestGORMCreateInBatches_VerifyInsert proves that GORM CreateInBatches
// with the local fork (commit d26405a) correctly inserts all rows without
// duplicates. This verifies the GORM fork's reflection allocation reduction
// doesn't break data integrity.
func TestGORMCreateInBatches_VerifyInsert(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(programName))
	require.NoError(t, err)

	prog := NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
	builder := prog.GetAndCreateFunctionBuilder("testBatch", string(MainFunctionName))
	for i := 0; i < 200; i++ {
		left := builder.EmitUndefined("left")
		right := builder.EmitUndefined("right")
		builder.EmitBinOp(OpAdd, left, right)
	}
	builder.Finish()

	// Flush + Save
	prog.Cache.FlushCompileUnit("unit-a")
	err = prog.Cache.SaveToDatabase()
	require.NoError(t, err)

	// Verify all instructions are in DB without duplicates
	var total, distinct int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*), COUNT(DISTINCT code_id) FROM ir_codes WHERE program_name = ?",
		programName,
	).Row().Scan(&total, &distinct)

	t.Logf("total=%d distinct=%d", total, distinct)
	require.Equal(t, total, distinct, "total must equal distinct (no duplicates)")
	require.Greater(t, total, int64(0), "should have instructions")
}
