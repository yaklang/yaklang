package ssa

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestLazyInstructionSaveAgain(t *testing.T) {
	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, true, Application, vf, "", ttl)
	builder := prog.GetAndCreateFunctionBuilder("", "main")

	// create instruction
	undefineA := builder.EmitUndefined("a")
	undefineB := builder.EmitUndefined("b")
	undefineC := builder.EmitUndefined("c")

	binInst := builder.EmitBinOp(OpAdd, undefineA, undefineB)
	instID := binInst.GetId()
	require.Greater(t, instID, int64(0))
	require.Equal(t, LineDisasm(binInst), "add(Undefined-a, Undefined-b)")

	{
		// wait instruction save to db
		time.Sleep(ttl * 2)

		ir := ssadb.GetIrCodeById(ssadb.GetDB(), instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		require.Contains(t, ir.String, fmt.Sprint(undefineA.GetId()))
		require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

	cache := prog.Cache

	// load instruction from db
	inst2 := cache.GetInstruction(instID)
	require.NotNil(t, inst2)
	require.Equal(t, inst2.GetId(), instID)
	require.Equal(t, inst2.GetOpcode(), SSAOpcodeBinOp)

	// // replace value
	ReplaceAllValue(undefineA, undefineC) // a -> c
	// // a + b => c + b
	require.Equal(t, LineDisasm(inst2), "add(Undefined-c, Undefined-b)")

	// wait instruction save to db
	time.Sleep(ttl * 2)
	{
		ir := ssadb.GetIrCodeById(ssadb.GetDB(), instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		require.Contains(t, ir.String, fmt.Sprint(undefineC.GetId()))
		require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

	// // load instruction from db
	inst3 := cache.GetInstruction(instID)
	require.NotNil(t, inst3)
	require.Equal(t, inst3.GetId(), instID)
	require.Equal(t, inst3.GetOpcode(), SSAOpcodeBinOp)
	require.Equal(t, LineDisasm(inst3), "add(Undefined-c, Undefined-b)")

	prog.Finish()

	{
		// check database
		ir := ssadb.GetIrCodeById(ssadb.GetDB(), instID)
		log.Infof("ir: %v", ir)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeBinOp))
		require.Contains(t, ir.String, fmt.Sprint(undefineC.GetId()))
		require.Contains(t, ir.String, fmt.Sprint(undefineB.GetId()))
	}

}

func TestCache_with_lazyBuilder(t *testing.T) {

	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, true, Application, vf, "", ttl)
	builder := prog.GetAndCreateFunctionBuilder("", "main")

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
	require.Greater(t, undefineId, int64(0))
	require.True(t, builded)
	{
		ir := ssadb.GetIrCodeById(ssadb.GetDB(), subFuncId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeFunction))
		require.Equal(t, ir.Name, subFunctionName)
	}
	{
		ir := ssadb.GetIrCodeById(ssadb.GetDB(), undefineId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeUndefined))
		require.Equal(t, ir.CurrentFunction, subFuncId)
	}

}
