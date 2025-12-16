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
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID)
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
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, subFuncId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeFunction))
		require.Equal(t, ir.Name, subFunctionName)
	}
	{
		ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, undefineId)
		require.NotNil(t, ir)
		require.Equal(t, ir.Opcode, int64(SSAOpcodeUndefined))
		require.Equal(t, ir.CurrentFunction, subFuncId)
	}

}

// TestLazySaveType tests LazySaveType directly
func TestLazySaveType(t *testing.T) {
	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, ProgramCacheDBWrite, Application, vf, "", 0, ttl)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Create a value to test with
	testValue := builder.EmitUndefined("testVar")
	require.NotNil(t, testValue)

	// Track if the lazy save function is called
	lazySaveCalled := false
	typeSaved := false
	var savedType Type

	// Create a test type
	testType := CreateStringType()
	require.NotNil(t, testType)

	// Set up lazy save function
	testValue.getAnValue().SetLazySaveType(func() {
		lazySaveCalled = true
		// Simulate type saving logic
		cache := testValue.getAnValue().getProgramCache()
		if cache != nil && cache.TypeCache != nil {
			cache.TypeCache.Set(testType)
			typeSaved = true
			savedType = testType
		}
	})

	// Initially, lazy save should not be called
	require.False(t, lazySaveCalled)
	require.False(t, typeSaved)

	// Call LazySaveType to trigger the lazy save function
	testValue.LazySaveType()

	// Verify that the lazy save function was called
	require.True(t, lazySaveCalled, "LazySaveType should have called the lazy save function")
	require.True(t, typeSaved, "Type should have been saved")
	require.NotNil(t, savedType, "Saved type should not be nil")
	require.Equal(t, testType, savedType, "Saved type should match the test type")

	// Test with nil lazy save function
	testValue.getAnValue().SetLazySaveType(nil)
	require.NotPanics(t, func() {
		testValue.LazySaveType()
	})

	prog.Finish()
	if prog.DatabaseKind != ProgramCacheMemory {
		prog.UpdateToDatabase()
	}
	prog.Cache.SaveToDatabase()
}

// TestSetVirtualRegister tests SetVirtualRegister which calls LazySaveType
func TestSetVirtualRegister(t *testing.T) {
	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, ProgramCacheDBWrite, Application, vf, "", 0, ttl)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Create a new value
	newValue := builder.EmitConstInst("test")
	require.NotNil(t, newValue)

	// Set up lazy save function
	lazySaveCalled := false
	newValue.getAnValue().SetLazySaveType(func() {
		lazySaveCalled = true
	})

	// Initially, lazy save should not be called
	require.False(t, lazySaveCalled)

	// SetVirtualRegister should call LazySaveType
	prog.SetVirtualRegister(newValue)

	// Verify that the lazy save function was called
	require.True(t, lazySaveCalled, "SetVirtualRegister should have called LazySaveType")

	// Verify the value has an ID after SetVirtualRegister
	require.Greater(t, newValue.GetId(), int64(0), "Value should have an ID after SetVirtualRegister")

	prog.Finish()
	if prog.DatabaseKind != ProgramCacheMemory {
		prog.UpdateToDatabase()
	}
	prog.Cache.SaveToDatabase()
}

// TestEmitExCallsSetVirtualRegister tests emitEx which is the upper level caller of SetVirtualRegister
func TestEmitExCallsSetVirtualRegister(t *testing.T) {
	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := NewProgram(programName, ProgramCacheDBWrite, Application, vf, "", 0, ttl)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	// Create operands for binop
	operand1 := builder.EmitConstInst(1)
	operand2 := builder.EmitConstInst(2)

	// Track if LazySaveType is called for the binop instruction
	// We'll set it up after creating the binop but before it's fully emitted
	// Actually, we can't do that because emitEx is called during EmitBinOp
	// Instead, we verify that emitEx -> SetVirtualRegister was called by checking
	// that the instruction was properly registered with an ID

	// Emit a binop which will call emitEx -> SetVirtualRegister -> LazySaveType
	// This tests the upper level call chain: EmitBinOp -> emit -> emitEx -> SetVirtualRegister -> LazySaveType
	binOp := builder.EmitBinOp(OpAdd, operand1, operand2)
	require.NotNil(t, binOp)

	// Verify that SetVirtualRegister was called (which should have called LazySaveType)
	// Since emitEx is called during EmitBinOp, SetVirtualRegister should have been called
	// We verify this by checking that the instruction has an ID and is registered
	require.Greater(t, binOp.GetId(), int64(0), "BinOp should have an ID after emitEx -> SetVirtualRegister")
	require.Greater(t, operand1.GetId(), int64(0), "Operand1 should have an ID after emitEx -> SetVirtualRegister")
	require.Greater(t, operand2.GetId(), int64(0), "Operand2 should have an ID after emitEx -> SetVirtualRegister")

	// Verify the instruction was properly registered in the program cache
	inst, ok := prog.GetInstructionById(binOp.GetId())
	require.True(t, ok, "BinOp instruction should be retrievable by ID after emitEx")
	require.NotNil(t, inst, "BinOp instruction should not be nil")
	require.Equal(t, binOp.GetId(), inst.GetId(), "Retrieved instruction should have the same ID")

	// Verify the instruction is in the current block
	require.NotNil(t, binOp.GetBlock(), "BinOp should have a block after emitEx")
	require.Contains(t, binOp.GetBlock().Insts, binOp.GetId(), "BinOp ID should be in block's instruction list")

	prog.Finish()
	if prog.DatabaseKind != ProgramCacheMemory {
		prog.UpdateToDatabase()
	}
	prog.Cache.SaveToDatabase()
}
