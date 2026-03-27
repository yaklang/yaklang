package ssa

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestMatchInstructionsByVariableUsesMemoryIndexesWhileDBWrite(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitUndefined("needle")
	prog.Cache.AddVariable("needle", inst)

	var ids []int64
	err := ssadb.GetDB().Model(&ssadb.IrIndex{}).Where("program_name = ?", programName).Pluck("value_id", &ids).Error
	require.NoError(t, err)
	require.Empty(t, ids, "DB should stay empty so the test only passes through memory indexes")

	results := MatchInstructionsByVariableWithExcludeFiles(
		context.Background(),
		prog,
		ssadb.ExactCompare,
		ssadb.NameMatch,
		"needle",
		nil,
	)
	require.Len(t, results, 1)
	require.Equal(t, inst.GetId(), results[0].GetId())
}

func TestMatchInstructionsByVariableReloadsSpilledInstructionFromMemoryIDIndex(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 20*time.Millisecond, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	inst := builder.EmitBinOp(OpAdd, left, right)
	prog.Cache.AddVariable("needle", inst)

	builder.Finish()
	waitInstructionSpilledAfterFinish(t, prog, programName, inst.GetId(), 20*time.Millisecond)

	results := MatchInstructionsByVariableWithExcludeFiles(
		context.Background(),
		prog,
		ssadb.ExactCompare,
		ssadb.NameMatch,
		"needle",
		nil,
	)
	require.Len(t, results, 1)
	require.Equal(t, inst.GetId(), results[0].GetId())
}

func TestMatchInstructionsByVariableMemoryPathStillHonorsExcludeFiles(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	inst := builder.EmitUndefined("needle")

	editor := memedit.NewMemEditor("needle")
	editor.SetProgramName(programName)
	editor.SetFileName("demo.java")
	editor.SetFolderPath("/src/")
	inst.SetRange(editor.GetFullRange())
	prog.Cache.AddVariable("needle", inst)

	results := MatchInstructionsByVariableWithExcludeFiles(
		context.Background(),
		prog,
		ssadb.ExactCompare,
		ssadb.NameMatch,
		"needle",
		[]string{editor.GetUrl()},
	)
	require.Empty(t, results)
}
