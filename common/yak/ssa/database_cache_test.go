package ssa

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestLazyInstructionSaveAgain(t *testing.T) {
	t.Skip("this test is not stable, need to fix it")

	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, ProgramCacheDBWrite, Application, vf, "", 0, ttl)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))
	cache := prog.Cache
	// enable cache save to database
	// cache.InstructionCache.EnableSave()

	// create instruction
	undefineA := builder.EmitUndefined("a")
	undefineB := builder.EmitUndefined("b")
	undefineC := builder.EmitUndefined("c")

	binInst := builder.EmitBinOp(OpAdd, undefineA, undefineB)
	instID := binInst.GetId()
	require.Greater(t, instID, int64(0))
	require.Equal(t, LineDisASM(binInst), "add(Undefined-a, Undefined-b)")

	{
		// wait instruction save to db
		time.Sleep(ttl * 2)

		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		// require.Contains(t, ir.String, fmt.Sprint(undefineA.GetId()))
		// require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

	// load instruction from db
	inst2 := cache.GetInstruction(instID)
	require.NotNil(t, inst2)
	require.Equal(t, inst2.GetId(), instID)
	require.Equal(t, inst2.GetOpcode(), SSAOpcodeBinOp)
	// this inst2 is load form db, is lazyInstruction

	// // replace value
	ReplaceAllValue(undefineA, undefineC) // a -> c
	// // a + b => c + b
	require.Equal(t, LineDisASM(binInst), "add(Undefined-c, Undefined-b)")

	// wait instruction save to db
	time.Sleep(ttl * 2)
	{
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		// require.Contains(t, ir.String, fmt.Sprint(undefineC.GetId()))
		// require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

	// // load instruction from db
	inst3 := cache.GetInstruction(instID)
	require.NotNil(t, inst3)
	require.Equal(t, inst3.GetId(), instID)
	require.Equal(t, inst3.GetOpcode(), SSAOpcodeBinOp)
	require.Equal(t, LineDisASM(inst3), "add(Undefined-c, Undefined-b)")

	prog.Finish()
	if prog.DatabaseKind != ProgramCacheMemory { // save program
		prog.UpdateToDatabase()
	}
	prog.Cache.SaveToDatabase()

	{
		// check database
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		// require.Contains(t, ir.String, fmt.Sprint(undefineC.GetId()))
		// require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

}

func TestCache_with_lazyBuilder(t *testing.T) {

	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, ProgramCacheDBWrite, Application, vf, "", 0, ttl)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	builded := false
	var undefineId int64
	subFunctionName := "sub"
	subFunction := builder.NewFunc(subFunctionName)
	subFunction.AddLazyBuilder(func() {
		// builder := subFunction.GetBuilder()
		builder := prog.GetAndCreateFunctionBuilder("", subFunctionName)
		undefineId = builder.EmitUndefined("a").GetId()
		builded = true
	})
	subFuncId := subFunction.GetId()
	require.Greater(t, subFuncId, int64(0))

	// wait
	time.Sleep(ttl * 2)

	// check database
	require.False(t, builded)
	require.Equal(t, undefineId, int64(0))

	// finish
	prog.Finish()
	if prog.DatabaseKind != ProgramCacheMemory { // save program
		prog.UpdateToDatabase()
	}
	prog.Cache.SaveToDatabase()

	require.Greater(t, undefineId, int64(0))
	require.True(t, builded)
	{
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), subFuncId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeFunction))
		require.Equal(t, ir.Name, subFunctionName)
	}
	{
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), undefineId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeUndefined))
		require.Equal(t, ir.CurrentFunction, subFuncId)
	}

}
