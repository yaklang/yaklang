package ssa

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func newProgramWithTTL(programName string, ttl time.Duration, kind ProgramCacheKind, fs *filesys.VirtualFS) *Program {
	opts := []ssaconfig.Option{
		ssaconfig.WithSetProgramName(programName),
	}
	if ttl > 0 {
		opts = append(opts, ssaconfig.WithCompileIrCacheTTL(ttl))
	}
	cfg, _ := ssaconfig.New(ssaconfig.ModeSSACompile, opts...)
	return NewProgram(cfg, kind, Application, fs, "", 0)
}

func TestResolveInstructionCacheSettings(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	require.NoError(t, err)

	cfg.SetCompileProjectBytes(512 * 1024)
	ttl, maxEntries := resolveInstructionCacheSettings(cfg)
	require.Equal(t, time.Duration(0), ttl)
	require.Equal(t, 0, maxEntries)

	cfg.SetCompileProjectBytes(8 * 1024 * 1024)
	ttl, maxEntries = resolveInstructionCacheSettings(cfg)
	require.Equal(t, time.Second, ttl)
	require.Equal(t, 5000, maxEntries)

	cfg.SetCompileProjectBytes(32 * 1024 * 1024)
	ttl, maxEntries = resolveInstructionCacheSettings(cfg)
	require.Equal(t, largeProjectCacheTTL, ttl)
	require.Equal(t, largeProjectCacheMax, maxEntries)
}

func TestMarshalBasicBlockUsesOwnCurrentBlockID(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	block := builder.Function.NewBasicBlock("self")
	require.Greater(t, block.GetId(), int64(0))

	irCode, err := marshalIrCode(block)
	require.NoError(t, err)
	require.NotNil(t, irCode)
	require.Equal(t, block.GetId(), irCode.CurrentBlock, "basic block rows should persist their own block id")
}

func TestCloneProgramConfigKeepsCompileProjectBytes(t *testing.T) {
	cfg, err := ssaconfig.New(
		ssaconfig.ModeSSACompile,
		ssaconfig.WithSetProgramName("root"),
		ssaconfig.WithCompileIrCacheTTL(123*time.Millisecond),
		ssaconfig.WithCompileIrCacheMax(77),
	)
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(largeProjectByteThreshold + 1024)

	cloned := cloneProgramConfig(cfg, "child")
	require.NotNil(t, cloned)
	require.Equal(t, cfg.GetCompileIrCacheTTL(), cloned.GetCompileIrCacheTTL())
	require.Equal(t, cfg.GetCompileIrCacheMax(), cloned.GetCompileIrCacheMax())
	require.Equal(t, cfg.GetCompileProjectBytes(), cloned.GetCompileProjectBytes())
	require.Equal(t, "child", cloned.GetProgramName())
}

func TestSaveEditorTracksOnlySourceHashInMemory(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	editor := prog.CreateEditor([]byte("class Demo {}"), "/src/Demo.java", false)
	hash := editor.GetIrSourceHash()

	prog.SaveEditor(editor)
	prog.SaveEditor(editor)

	require.Equal(t, 0, prog.Cache.sources.PersistedCount(), "source hash should not be marked persisted before save ack")
	require.Equal(t, 1, prog.Cache.sources.PayloadCount(), "duplicate source saves should collapse to one registered payload")

	prog.Cache.sources.Close()
	require.Equal(t, 1, prog.Cache.sources.PersistedCount(), "source hash should be marked persisted after close flush")
	require.Equal(t, 0, prog.Cache.sources.PayloadCount(), "payload cache should be released after persistence")

	var count int64
	err := ssadb.GetDB().Model(&ssadb.IrSource{}).
		Where("program_name = ? AND source_code_hash = ?", programName, hash).
		Count(&count).Error
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestLazyInstructionSaveAgain(t *testing.T) {
	t.Skip("this test is not stable, need to fix it")

	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
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
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
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
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
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
		if cache != nil {
			cache.rememberType(testType)
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
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
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

func TestGetTypeFromDBFallsBackToResidentTypeCache(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	value := builder.EmitUndefined("residentType")
	testType := CreateStringType()
	value.SetType(testType)

	typeID := testType.GetId()
	require.Greater(t, typeID, int64(0))

	residentType, ok := prog.Cache.residentType(typeID)
	require.True(t, ok, "type should remain resident in the memory bridge")
	require.NotNil(t, residentType)
	require.Nil(t, ssadb.GetIrTypeById(ssadb.GetDB(), programName, typeID), "type should not be in DB before type cache close")

	loadedType := GetTypeFromDB(prog.Cache, typeID)
	require.NotNil(t, loadedType, "type bridge should return resident type before DB persistence")
	require.Equal(t, typeID, loadedType.GetId())
	require.Equal(t, testType.String(), loadedType.String())
}

func TestInstructionReloadKeepsResidentTypeReachable(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	value := builder.EmitUndefined("typedValue")
	testType := CreateStringType()
	value.SetType(testType)

	typeID := testType.GetId()
	require.Greater(t, typeID, int64(0))
	require.Nil(t, ssadb.GetIrTypeById(ssadb.GetDB(), programName, typeID), "type should not be in DB before type cache close")

	irCode, err := marshalIrCode(value)
	require.NoError(t, err)
	require.NotNil(t, irCode)
	require.Equal(t, typeID, irCode.TypeID)
	require.NoError(t, ssadb.GetDB().Save(irCode).Error)

	prog.Cache.deleteInstructionByID(value.GetId())

	reloaded := prog.Cache.GetInstruction(value.GetId())
	require.NotNil(t, reloaded, "instruction should reload from DB")

	reloadedValue, ok := ToValue(reloaded)
	require.True(t, ok)

	reloadedType := reloadedValue.GetType()
	require.NotNil(t, reloadedType, "instruction reload should resolve type through memory bridge")
	require.Equal(t, typeID, reloadedType.GetId())
	require.Equal(t, testType.String(), reloadedType.String())
}

func TestGetStringMemberUsesStringIndexWithoutReloadingConstKey(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	object := builder.EmitUndefined("obj")
	member := builder.EmitUndefined("member")
	key := builder.EmitConstInstPlaceholder(string(BlueprintRelationParents))
	object.AddMember(key, member)

	prog.Cache.deleteInstructionByID(key.GetId())
	require.False(t, prog.Cache.hasResidentInstruction(key.GetId()), "const key should no longer be resident")
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, key.GetId()), "const key should not be reloaded from DB in this test")

	got, ok := object.GetStringMember(string(BlueprintRelationParents))
	require.True(t, ok, "string member lookup should succeed without reloading the const key")
	require.NotNil(t, got)
	require.Equal(t, member.GetId(), got.GetId())
}

func TestReloadedObjectGetStringMemberDoesNotReloadConstKeyInstruction(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	object := builder.EmitUndefined("obj")
	member := builder.EmitUndefined("member")
	key := builder.EmitConstInstPlaceholder(string(BlueprintRelationParents))
	object.AddMember(key, member)

	keyIR, err := marshalIrCode(key)
	require.NoError(t, err)
	require.NotNil(t, keyIR)
	require.NoError(t, ssadb.GetDB().Save(keyIR).Error)

	objectIR, err := marshalIrCode(object)
	require.NoError(t, err)
	require.NotNil(t, objectIR)
	require.NoError(t, ssadb.GetDB().Save(objectIR).Error)

	prog.Cache.deleteInstructionByID(object.GetId())
	prog.Cache.deleteInstructionByID(key.GetId())

	reloadedObject := prog.Cache.GetInstruction(object.GetId())
	require.NotNil(t, reloadedObject, "object should reload from DB")
	reloadedValue, ok := ToValue(reloadedObject)
	require.True(t, ok)

	countBeforeLookup := prog.Cache.CountInstruction()
	got, ok := reloadedValue.GetStringMember(string(BlueprintRelationParents))
	require.True(t, ok, "reloaded object should resolve string member without reloading the const key instruction")
	require.NotNil(t, got)
	require.Equal(t, member.GetId(), got.GetId())
	require.Equal(t, countBeforeLookup, prog.Cache.CountInstruction(), "string lookup should not pull the const key instruction back into the cache")
}

func TestDisableInstructionSpillKeepsInstructionResidentUntilFunctionFinish(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("spill_guard_left")
	right := builder.EmitUndefined("spill_guard_right")
	inst := builder.EmitBinOp(OpAdd, left, right)
	instID := inst.GetId()
	require.Greater(t, instID, int64(0))

	prog.Cache.DisableInstructionSpill()
	require.True(t, prog.Cache.IsInstructionSpillDisabled())

	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID), "instruction should not spill while spill is disabled")

	resident := prog.Cache.GetInstruction(instID)
	require.NotNil(t, resident, "instruction should stay resident while spill is disabled")
	require.Equal(t, instID, resident.GetId())

	prog.Cache.EnableInstructionSpill()
	require.False(t, prog.Cache.IsInstructionSpillDisabled())

	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID), "instruction should still stay resident before function finish")

	builder.Finish()

	require.Eventually(t, func() bool {
		return ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID) != nil
	}, 2*time.Second, ttl, "instruction should spill after function finish enables eviction tracking")
}

func TestHotInstructionStaysResidentAfterSpillReenabled(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	inst := builder.EmitUndefined("spill_guard_hot")
	instID := inst.GetId()
	require.Greater(t, instID, int64(0))

	prog.Cache.DisableInstructionSpill()
	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID))

	prog.Cache.EnableInstructionSpill()
	time.Sleep(ttl * 3)

	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID), "hot instruction should remain resident after spill is re-enabled")
	resident := prog.Cache.GetInstruction(instID)
	require.NotNil(t, resident)
	require.Equal(t, instID, resident.GetId())
}

func TestBasicBlockStaysResidentAfterFunctionFinish(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	prog.compileConfig.SetCompileProjectBytes(largeProjectByteThreshold)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	builder.EmitBinOp(OpAdd, left, right)
	block := builder.Function.NewBasicBlock("cooldown-target")
	block.SetScope(NewScope(builder.Function, prog.GetProgramName()))
	blockID := block.GetId()

	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, blockID), "basic block should stay resident before function finish")
	resident := prog.Cache.GetInstruction(blockID)
	require.NotNil(t, resident)
	require.Equal(t, blockID, resident.GetId())

	builder.Finish()

	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, blockID), "basic block should remain resident after function finish")
	resident = prog.Cache.GetInstruction(blockID)
	require.NotNil(t, resident)
	require.Equal(t, blockID, resident.GetId())
}

func TestFunctionScopedInstructionsSpillAfterFinish(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	param := builder.NewParam("cooldown_param")
	key := builder.EmitConstInst("field")
	member := builder.NewParameterMember("cooldown_param.field", param, key)
	captured := builder.EmitUndefined("captured")
	capturedVariable := builder.CreateVariable("captured")
	builder.AssignVariable(capturedVariable, captured)
	freeValue := builder.BuildFreeValue("captured")

	time.Sleep(ttl * 3)
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, param.GetId()), "function-scoped parameter should stay resident before finish")
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, member.GetId()), "parameter member should stay resident before finish")
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, freeValue.GetId()), "free value should stay resident before finish")

	builder.Finish()

	waitInstructionSpilledAfterFinish(t, prog, programName, param.GetId(), ttl)
	waitInstructionSpilledAfterFinish(t, prog, programName, member.GetId(), ttl)
	waitInstructionSpilledAfterFinish(t, prog, programName, freeValue.GetId(), ttl)
}

func waitInstructionSpilledAfterFinish(t *testing.T, prog *Program, programName string, id int64, ttl time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		if ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, id) == nil {
			return false
		}
		return !prog.Cache.hasResidentInstruction(id)
	}, 3*time.Second, ttl, "instruction %d should be persisted and evicted from resident cache", id)
}

func waitInstructionPersisted(t *testing.T, programName string, id int64, ttl time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		return ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, id) != nil
	}, 3*time.Second, ttl, "instruction %d should be persisted to DB", id)
}

func TestReloadedInstructionGetBlockUsesResidentHotBasicBlockAfterFinish(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	prog.compileConfig.SetCompileProjectBytes(largeProjectByteThreshold)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	binInst := builder.EmitBinOp(OpAdd, left, right)
	instID := binInst.GetId()
	blockID := builder.CurrentBlock.GetId()

	builder.Finish()

	waitInstructionPersisted(t, programName, instID, ttl)
	prog.Cache.deleteInstructionByID(instID)
	residentBeforeLoad := prog.Cache.hasResidentInstruction(blockID)
	require.True(t, residentBeforeLoad, "basic block should stay resident as a hot instruction")
	require.Nil(t, ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, blockID), "hot basic block should not spill during compile")

	reloaded := prog.Cache.GetInstruction(instID)
	require.NotNil(t, reloaded)
	_, ok := ToLazyInstruction(reloaded)
	require.True(t, ok, "spilled instruction should reload as LazyInstruction")
	residentAfterInstReload := prog.Cache.hasResidentInstruction(blockID)
	require.True(t, residentAfterInstReload, "reloading the instruction should still see the resident hot block")

	block := reloaded.GetBlock()
	require.NotNil(t, block)
	require.Equal(t, blockID, block.GetId())
}

func TestInstruction2IrCodeReloadsSpilledBasicBlockAfterFinish(t *testing.T) {
	programName := uuid.NewString()
	ttl := 20 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	prog.compileConfig.SetCompileProjectBytes(largeProjectByteThreshold)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	binInst := builder.EmitBinOp(OpAdd, left, right)
	instID := binInst.GetId()
	blockID := builder.CurrentBlock.GetId()
	editor := prog.CreateEditor([]byte("class Demo { void run(){ left + right; } }"), "Demo.java", false)
	rng := editor.GetRangeByPosition(memedit.NewPosition(1, 26), memedit.NewPosition(1, 38))
	binInst.SetRange(rng)
	block := builder.CurrentBlock
	block.SetRange(rng)
	sourceHash := editor.GetIrSourceHash()
	require.NoError(t, ssadb.GetDB().Save(ssadb.MarshalFile(editor, sourceHash)).Error)

	builder.Finish()

	instIR, err := marshalIrCode(binInst)
	require.NoError(t, err)
	require.NotNil(t, instIR)
	require.NoError(t, ssadb.GetDB().Save(instIR).Error)

	blockIR, err := marshalIrCode(block)
	require.NoError(t, err)
	require.NotNil(t, blockIR)
	require.NoError(t, ssadb.GetDB().Save(blockIR).Error)
	prog.Cache.deleteInstructionByID(instID)
	prog.Cache.deleteInstructionByID(blockID)

	reloaded := prog.Cache.GetInstruction(instID)
	require.NotNil(t, reloaded)
	_, ok := ToLazyInstruction(reloaded)
	require.True(t, ok, "spilled instruction should reload as LazyInstruction")
	residentAfterInstReload := prog.Cache.hasResidentInstruction(blockID)
	require.False(t, residentAfterInstReload, "reloading the instruction itself should not eagerly reload its block")

	ir := ssadb.EmptyIrCode(programName, instID)
	require.NoError(t, Instruction2IrCode(reloaded, ir))
	require.Equal(t, blockID, ir.CurrentBlock, "save path still needs block identity for the reloaded instruction")
	residentAfterMarshal := prog.Cache.hasResidentInstruction(blockID)
	require.False(t, residentAfterMarshal, "Instruction2IrCode should not reload the spilled BasicBlock when it only needs CurrentBlock id")
}

func TestLazyInstructionHasUsersReflectsPersistedUsers(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	_ = builder.EmitBinOp(OpAdd, left, right)

	require.True(t, left.HasUsers(), "resident instruction should report users before reload")

	leftIR, err := marshalIrCode(left)
	require.NoError(t, err)
	require.NotNil(t, leftIR)
	require.NoError(t, ssadb.GetDB().Save(leftIR).Error)

	leftID := left.GetId()
	prog.Cache.deleteInstructionByID(leftID)

	reloaded := prog.Cache.GetInstruction(leftID)
	require.NotNil(t, reloaded)
	_, ok := ToLazyInstruction(reloaded)
	require.True(t, ok, "instruction should reload as LazyInstruction")

	value, ok := ToValue(reloaded)
	require.True(t, ok)
	require.True(t, value.HasUsers(), "lazy instruction should preserve persisted user information")
}

func TestCreateVariableIndexWithNilScopeDoesNotPanic(t *testing.T) {
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(uuid.NewString(), 0, ProgramCacheMemory, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	inst := builder.EmitUndefined("index_scope_guard")
	variable := builder.CreateVariable("index_scope_guard")
	require.NotNil(t, variable)
	builder.AssignVariable(variable, inst)
	variable.SetScope(nil)

	require.NotPanics(t, func() {
		index := CreateVariableIndexByName("index_scope_guard", inst)
		require.NotNil(t, index)
		require.Empty(t, index.ScopeName)
	})
}

func TestSwitchFreevalueInSideEffectSkipsNonCallCallSite(t *testing.T) {
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(uuid.NewString(), 0, ProgramCacheMemory, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	variable := builder.CreateVariable("captured")
	require.NotNil(t, variable)
	value := builder.EmitUndefined("captured")
	builder.AssignVariable(variable, value)

	nonCall := builder.EmitConstInst("not-call-site")
	sideEffect := &SideEffect{
		anValue:  NewValue(),
		CallSite: nonCall.GetId(),
		Value:    value.GetId(),
	}
	sideEffect.SetProgram(prog)
	sideEffect.SetFunc(builder.Function)
	sideEffect.SetBlock(builder.CurrentBlock)

	require.NotPanics(t, func() {
		builder.SwitchFreevalueInSideEffect("captured", sideEffect)
	})
}

// TestEmitExCallsSetVirtualRegister tests emitEx which is the upper level caller of SetVirtualRegister
func TestEmitExCallsSetVirtualRegister(t *testing.T) {
	programName := uuid.NewString()
	ttl := time.Millisecond * 100

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
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

func TestInstructionCache_TTLReloadFromDB(t *testing.T) {
	programName := uuid.NewString()
	ttl := 80 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("left")
	right := builder.EmitUndefined("right")
	binInst := builder.EmitBinOp(OpAdd, left, right)
	instID := binInst.GetId()

	builder.Finish()
	waitInstructionSpilledAfterFinish(t, prog, programName, instID, ttl)

	reloaded := prog.Cache.GetInstruction(instID)
	require.NotNil(t, reloaded)
	require.Equal(t, instID, reloaded.GetId())
	_, ok := ToLazyInstruction(reloaded)
	require.True(t, ok, "expired instruction should reload as LazyInstruction")

	prog.Finish()
	prog.UpdateToDatabase()
	prog.Cache.SaveToDatabase()
}

func TestInstructionCache_DeleteDoesNotPersist(t *testing.T) {
	programName := uuid.NewString()
	ttl := 60 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	value := builder.EmitUndefined("temp")
	instID := value.GetId()

	prog.DeleteInstruction(value)
	time.Sleep(ttl * 2)

	ir := ssadb.GetIrCodeItemById(ssadb.GetDB(), programName, instID)
	require.Nil(t, ir, "DeleteInstruction should drop the instruction without saving it")
}

func TestInstructionCache_TTLUpsertsDirtyLazyInstruction(t *testing.T) {
	programName := uuid.NewString()
	ttl := 80 * time.Millisecond

	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, ttl, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	left := builder.EmitUndefined("writeback_left")
	right := builder.EmitUndefined("writeback_right")
	value := builder.EmitBinOp(OpAdd, left, right)
	instID := value.GetId()
	builder.Finish()
	waitInstructionSpilledAfterFinish(t, prog, programName, instID, ttl)

	reloaded := prog.Cache.GetInstruction(instID)
	require.NotNil(t, reloaded)
	reloaded.SetExtern(true)
	prog.Cache.coolDownInstructions([]int64{instID}, ttl)

	require.Eventually(t, func() bool {
		var ir ssadb.IrCode
		var count int
		countErr := ssadb.GetDB().Model(&ssadb.IrCode{}).
			Where("program_name = ? AND code_id = ?", programName, instID).
			Count(&count).Error
		err := ssadb.GetDB().Model(&ssadb.IrCode{}).
			Where("program_name = ? AND code_id = ?", programName, instID).
			Order("id DESC").
			First(&ir).Error
		return countErr == nil && count == 1 && err == nil && ir.IsExternal && !prog.Cache.hasResidentInstruction(instID)
	}, 3*time.Second, ttl, "dirty lazy instruction should be written back after finish-triggered eviction")

	prog.Finish()
	prog.UpdateToDatabase()
	prog.Cache.SaveToDatabase()
}

func TestInstructionCache_SaveDeduplicatesSourcesWithinBatch(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	vf := filesys.NewVirtualFs()
	prog := newProgramWithTTL(programName, 0, ProgramCacheDBWrite, vf)
	builder := prog.GetAndCreateFunctionBuilder("", string(MainFunctionName))

	editor := prog.CreateEditor([]byte("class A { int x; }"), "A.java")
	range1 := editor.GetRangeByPosition(memedit.NewPosition(1, 0), memedit.NewPosition(1, 1))
	range2 := editor.GetRangeByPosition(memedit.NewPosition(1, 2), memedit.NewPosition(1, 3))

	left := builder.EmitUndefined("left")
	left.SetRange(range1)
	right := builder.EmitUndefined("right")
	right.SetRange(range2)

	prog.Finish()
	prog.UpdateToDatabase()
	prog.Cache.SaveToDatabase()

	var count int
	err := ssadb.GetDB().Model(&ssadb.IrSource{}).
		Where("program_name = ? AND source_code_hash = ?", programName, editor.GetIrSourceHash()).
		Count(&count).Error
	require.NoError(t, err)
	require.Equal(t, 1, count, "same editor hash should only be persisted once per batch")
}
