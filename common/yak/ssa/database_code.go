package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func marshalInstruction(inst Instruction, irCode *ssadb.IrCode) bool {
	if utils.IsNil(inst) || utils.IsNil(irCode) {
		log.Errorf("BUG: marshalInstruction called with nil instruction")
		return false
	}
	if inst.GetId() == -1 {
		log.Errorf("[BUG]: instruction id is -1: %s", codec.AnyToString(inst))
		return false
	}

	// all instruction from database will be lazy instruction
	if lz, ok := ToLazyInstruction(inst); ok {
		// we just check if this lazy-instruction should be saved again?
		if !lz.ShouldSave() {
			return false
		}
	}

	err := Instruction2IrCode(inst, irCode)
	if err != nil {
		log.Errorf("FitIRCode error: %s", err)
		return false
	}

	if irCode.Opcode == 0 {
		log.Errorf("BUG: saveInstruction called with empty opcode: %v", inst.GetName())
	}
	return true
}

// Instruction2IrCode : marshal instruction to ir code, used in cache, to save to database
func Instruction2IrCode(inst Instruction, ir *ssadb.IrCode) error {
	if ir.CodeID != inst.GetId() {
		return utils.Errorf("marshal instruction id not match")
	}
	if inst.GetId() == -1 {
		log.Errorf("insts is -1")
	}

	instruction2IrCode(inst, ir)
	value2IrCode(inst, ir)

	function2IrCode(inst, ir)
	basicBlock2IrCode(inst, ir)
	ir.SetExtraInfo(marshalExtraInformation(inst))
	return nil
}

// IrCodeToInstruction : unmarshal ir code to instruction, used in LazyInstruction
func (c *ProgramCache) IrCodeToInstruction(inst Instruction, ir *ssadb.IrCode, cache *ProgramCache) Instruction {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			log.Errorf("err: %v", err)
		}
	}()
	instructionFromIrCode(inst, ir)
	c.valueFromIrCode(cache, inst, ir)
	basicBlockFromIrCode(inst, ir)

	// extern info
	unmarshalExtraInformation(cache, inst, ir)

	return inst
}

func fitRange(c *ssadb.IrCode, rangeIns *memedit.Range) {
	if utils.IsNil(rangeIns) || utils.IsNil(rangeIns.GetEditor()) {
		log.Warnf("(BUG or in DEBUG MODE) Range not found for %s", c.Name)
		return
	}
	editor := rangeIns.GetEditor()
	c.SourceCodeHash = editor.GetIrSourceHash()
	// start, end := rangeIns.GetOffsetRange()
	c.SourceCodeStartOffset = int64(rangeIns.GetStartOffset())
	c.SourceCodeEndOffset = int64(rangeIns.GetEndOffset())
}

func instruction2IrCode(inst Instruction, ir *ssadb.IrCode) {

	// --- Section 1 Start ---
	// start1 := time.Now()
	// name
	ir.ProgramName = inst.GetProgramName()
	ir.CodeID = inst.GetId()
	ir.Name = inst.GetName()
	ir.VerboseName = inst.GetVerboseName()
	ir.ShortVerboseName = inst.GetShortVerboseName()
	// ir.String = inst.String()
	// ir.ReadableName = LineDisASM(inst)
	// ir.ReadableNameShort = LineShortDisASM(inst)
	// opcode
	ir.Opcode = int64(inst.GetOpcode())
	ir.OpcodeName = SSAOpcode2Name[inst.GetOpcode()]
	// atomic.AddUint64(&Marshal1, uint64(time.Since(start1)))
	// --- Section 1 End ---

	// --- Section 2 Start ---
	// start2 := time.Now()
	var codeRange *memedit.Range
	if ret := inst.GetRange(); ret != nil {
		codeRange = ret
	} else if ret := inst.GetBlock(); ret != nil {
		block, ok := ToBasicBlock(ret)
		if ok && block != nil && block.GetRange() != nil {
			codeRange = block.GetRange()
			log.Warnf("Fallback, the %v is not set range, use its basic_block instance' ", inst.GetName())
		}
	}

	if codeRange == nil {
		if ret := inst.GetFunc().GetRange(); ret != nil {
			log.Warnf("Fallback, the %v is not set range, use its function instance' ", inst.GetName())
			inst.SetRange(ret)
			codeRange = ret
		}
	}

	if codeRange == nil {
		// switch ret := inst.(type) {
		// case *BasicBlock:
		// 	if len(ret.Insts) > 0 {
		// 		codeRange = ret.GetInstructionById(ret.Insts[0]).GetRange()
		// 	}
		// case *Function:
		// 	if len(ret.Blocks) > 0 {
		// 		codeRange = ret.GetBasicBlockByID(ret.Blocks[0]).GetRange()
		// 	}
		// }
	}

	if codeRange == nil {
		// TODO:解决没有codeRange的问题
		//log.Errorf("Range not found for %s", inst.GetName())
	}

	// inst.SetRange(codeRange)
	fitRange(ir, codeRange)

	if fun := inst.GetFunc(); fun != nil {
		ir.CurrentFunction = fun.GetId()
	}
	if block := inst.GetBlock(); block != nil {
		ir.CurrentBlock = block.GetId()
	}

	ir.IsExternal = inst.IsExtern()
}

func instructionFromIrCode(inst Instruction, ir *ssadb.IrCode) {
	inst.SetId(ir.GetIdInt64())

	// name
	inst.SetName(ir.Name)
	inst.SetVerboseName(ir.VerboseName)

	// not function
	if !ir.IsFunction {
		if currentFunc, ok := inst.GetInstructionById(ir.CurrentFunction); ok && currentFunc != nil {
			if fun, ok := ToFunction(currentFunc); ok {
				inst.SetFunc(fun)
			} else {
				log.Errorf("BUG: set CurrentFunction[%d]: ", ir.CurrentFunction)
			}
		}
		if !ir.IsBlock {
			if currentBlock, ok := inst.GetInstructionById(ir.CurrentBlock); ok && currentBlock != nil {
				if block, ok := ToBasicBlock(currentBlock); ok {
					inst.SetBlock(block)
				} else {
					log.Errorf("BUG: set CurrentBlock[%d]:", ir.CurrentBlock)
				}
			}
		} else {
			if block, ok := ToBasicBlock(inst); ok {
				inst.SetBlock(block)
			} else {
				log.Errorf("BUG: set currentblock for block :%v", inst)
			}
		}
	}
	editor, start, end, err := ir.GetStartAndEndPositions()
	if err == nil {
		inst.SetRange(editor.GetRangeByPosition(start, end))
	}

	inst.SetExtern(ir.IsExternal)
}

func value2IrCode(inst Instruction, ir *ssadb.IrCode) {
	defer func() {
		if msg := recover(); msg != nil {
			log.Errorf("value2IrCode panic: %s", msg)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	value, ok := ToValue(inst)
	if !ok {
		return
	}
	if utils.IsNil(value) {
		return
	}
	var anValue *anValue

	if typ := value.GetType(); !utils.IsNil(typ) && typ.GetId() <= 0 {
		log.Errorf("BUG: value2IrCode called with nil type: %s %s", value.GetOpcode().String(), value.GetName())
		// return
	}
	// ir.String = value.String()
	ir.HasDefs = value.HasValues()

	anValue = value.getAnValue()

	// user
	ir.Users = anValue.userList
	// occulatation
	ir.Occulatation = anValue.occultation

	// object
	ir.IsObject = anValue.IsObject()
	if ir.IsObject {
		member := anValue.getMemberMap()
		ir.ObjectMembers = make(ssadb.Int64Map, 0, member.Len())
		member.ForEach(func(i, v int64) bool {
			ir.ObjectMembers.Append(i, v)
			return true
		})
	}

	// member
	ir.IsObjectMember = anValue.IsMember()
	if ir.IsObjectMember {
		ir.ObjectParent = anValue.object
		ir.ObjectKey = anValue.key
	}
	// variable

	variable := anValue.getVariablesMap()
	ir.Variable = make(ssadb.StringSlice, 0, variable.Len())
	variable.ForEach(func(i string, v *Variable) bool {
		ir.Variable = append(ir.Variable, i)
		if v.GetValue() == nil {
			log.Errorf("aa")
		}
		return true
	})

	// mask
	anValue.getMaskMap().ForEach(func(i int64, v int64) bool {
		ir.MaskedCodes = append(ir.MaskedCodes, v)
		return true
	})

	ir.Point = anValue.reference
	ir.Pointer = anValue.pointer

	if inst.GetOpcode() == SSAOpcodeConstInst {
		if constInst, ok := ToConstInst(inst); ok {
			ir.ConstType = string(constInst.ConstType)
			ir.String = constInst.String()
		}
	}
	ir.TypeID = saveType(anValue.GetType())
}

func (c *ProgramCache) valueFromIrCode(cache *ProgramCache, inst Instruction, ir *ssadb.IrCode) {
	value, ok := ToValue(inst)
	if !ok {
		return
	}

	anValue := value.getAnValue()

	//  user
	anValue.userList = ir.Users

	//  occulatation
	anValue.occultation = ir.Occulatation

	// object
	ir.ObjectMembers.ForEach(func(key, value int64) {
		anValue.getMemberMap(true).Set(key, value)
	})

	// object member
	if ir.IsObjectMember {
		anValue.object = ir.ObjectParent
		anValue.key = ir.ObjectKey
	}

	// variable
	for _, name := range ir.Variable {
		value.AddVariable(GetVariableFromDB(ir.GetIdInt64(), name))
	}

	// mask
	for _, m := range ir.MaskedCodes {
		anValue.getMaskMap(true).Set(m, m)
	}

	// reference
	anValue.pointer = ir.Pointer
	anValue.reference = ir.Point

	// type
	value.SetIsFromDB(true)
	value.SetType(GetTypeFromDB(cache, ir.TypeID))
}

func function2IrCode(inst Instruction, ir *ssadb.IrCode) {
	f, ok := ToFunction(inst)
	if !ok {
		return
	}

	ir.Opcode = int64(f.GetOpcode())
	ir.IsFunction = true
	ir.IsVariadic = f.hasEllipsis

	for _, formArg := range f.Params {
		if formArg <= 0 {
			continue
		}
		ir.FormalArgs = append(ir.FormalArgs, formArg)
	}

	for _, fv := range f.FreeValues {
		if fv <= 0 {
			continue
		}
		ir.FreeValues = append(ir.FreeValues, fv)
	}

	for _, returnIns := range f.Return {
		if returnIns <= 0 {
			continue
		}
		ir.ReturnCodes = append(ir.ReturnCodes, returnIns)
	}
	for _, sideEffect := range f.SideEffects {
		if sideEffect == nil {
			continue
		}
	}

	for _, blockID := range f.Blocks {
		if blockID <= 0 {
			continue
		}
		ir.CodeBlocks = append(ir.CodeBlocks, blockID)
	}

	if f.EnterBlock > 0 {
		ir.EnterBlock = f.EnterBlock
	}
	if f.ExitBlock > 0 {
		ir.ExitBlock = f.ExitBlock
	}
	if f.DeferBlock > 0 {
		ir.DeferBlock = f.DeferBlock
	}

	for _, subFunc := range f.ChildFuncs {
		ir.ChildrenFunction = append(ir.ChildrenFunction, subFunc)
	}
}

func basicBlock2IrCode(inst Instruction, ir *ssadb.IrCode) {
	block, ok := ToBasicBlock(inst)
	if !ok {
		return
	}

	ir.IsBlock = true
	ir.PredBlock = make([]int64, 0, len(block.Preds))
	for _, pred := range block.Preds {
		ir.PredBlock = append(ir.PredBlock, pred)
	}

	ir.SuccBlock = make([]int64, 0, len(block.Succs))
	for _, succ := range block.Succs {
		ir.SuccBlock = append(ir.SuccBlock, succ)
	}

	ir.Phis = make([]int64, 0, len(block.Phis))
	for _, phi := range block.Phis {
		ir.Phis = append(ir.Phis, phi)
	}
}

func basicBlockFromIrCode(inst Instruction, ir *ssadb.IrCode) {
}
