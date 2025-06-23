package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Instruction2IrCode : marshal instruction to ir code, used in cache, to save to database
func Instruction2IrCode(inst Instruction, ir *ssadb.IrCode) error {
	if ir.ID != uint(inst.GetId()) {
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
	SaveValueOffset(inst)
	return nil
}

// IrCodeToInstruction : unmarshal ir code to instruction, used in LazyInstruction
func (c *Cache) IrCodeToInstruction(inst Instruction, ir *ssadb.IrCode) Instruction {
	instructionFromIrCode(inst, ir)
	c.valueFromIrCode(inst, ir)
	basicBlockFromIrCode(inst, ir)

	// extern info
	unmarshalExtraInformation(inst, ir)

	return inst
}

func fitRange(c *ssadb.IrCode, rangeIns memedit.RangeIf) {
	if utils.IsNil(rangeIns) || utils.IsNil(rangeIns.GetEditor()) {
		log.Warnf("(BUG or in DEBUG MODE) Range not found for %s", c.Name)
		return
	}
	editor := rangeIns.GetEditor()
	c.SourceCodeHash = editor.GetIrSourceHash(c.ProgramName)
	// start, end := rangeIns.GetOffsetRange()
	c.SourceCodeStartOffset = int64(rangeIns.GetStartOffset())
	c.SourceCodeEndOffset = int64(rangeIns.GetEndOffset())
}

func instruction2IrCode(inst Instruction, ir *ssadb.IrCode) {
	// name
	ir.Name = inst.GetName()
	ir.VerboseName = inst.GetVerboseName()
	ir.ShortVerboseName = inst.GetShortVerboseName()
	ir.String = inst.String()
	ir.ReadableName = LineDisASM(inst)
	ir.ReadableNameShort = LineShortDisASM(inst)
	// opcode
	ir.Opcode = int64(inst.GetOpcode())
	ir.OpcodeName = SSAOpcode2Name[inst.GetOpcode()]

	var codeRange memedit.RangeIf
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
		switch ret := inst.(type) {
		case *BasicBlock:
			if len(ret.Insts) > 0 {
				codeRange = ret.GetInstructionById(ret.Insts[0]).GetRange()
			}
		case *Function:
			if len(ret.Blocks) > 0 {
				codeRange = ret.GetBasicBlockByID(ret.Blocks[0]).GetRange()
			}
		}
	}

	if codeRange == nil {
		log.Errorf("Range not found for %s", inst.GetName())
	}

	inst.SetRange(codeRange)
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
	// id
	inst.SetId(ir.GetIdInt64())

	// name
	inst.SetName(ir.Name)
	inst.SetVerboseName(ir.VerboseName)

	// not function
	if !ir.IsFunction {
		if fun, err := NewInstructionFromLazy(ir.CurrentFunction, ToFunction); err == nil {
			inst.SetFunc(fun)
		} else {
			log.Errorf("BUG: set CurrentFunction[%d]: %v", ir.CurrentFunction, err)
		}
		if block, err := NewInstructionFromLazy(ir.CurrentBlock, ToBasicBlock); err == nil {
			inst.SetBlock(block)
		} else {
			log.Errorf("BUG: set CurrentBlock[%d]: %v", ir.CurrentBlock, err)
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
		log.Errorf("not value: %s", inst.GetName())
		return
	}

	// ir.String = value.String()
	ir.HasDefs = value.HasValues()

	anValue := value.getAnValue()

	// user
	ir.Users = anValue.userList

	// occulatation
	ir.Occulatation = anValue.occultation

	// object
	ir.IsObject = anValue.IsObject()
	if ir.IsObject {
		ir.ObjectMembers = make(ssadb.Int64Map, 0, anValue.member.Len())
		anValue.member.ForEach(func(i, v int64) bool {
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

	ir.Variable = make(ssadb.StringSlice, 0, anValue.variables.Len())
	anValue.variables.ForEach(func(i string, v *Variable) bool {
		ir.Variable = append(ir.Variable, i)
		if v.GetValue() == nil {
			log.Errorf("aa")
		}
		go SaveVariableOffset(v, i, anValue.id)
		return true
	})

	// mask
	anValue.mask.ForEach(func(i string, v int64) bool {
		ir.MaskedCodes = append(ir.MaskedCodes, v)
		return true
	})

	ir.Point = anValue.reference
	ir.Pointer = anValue.pointer

	if inst.GetOpcode() == SSAOpcodeConstInst {
		if constInst, ok := ToConstInst(inst); ok {
			ir.ConstType = string(constInst.ConstType)
		}
	}

	ir.TypeID = SaveTypeToDB(anValue.GetType(), ir.ProgramName)
}

func (c *Cache) valueFromIrCode(inst Instruction, ir *ssadb.IrCode) {
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
		anValue.member.Set(key, value)
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
		anValue.mask.Add(m)
	}

	// reference
	anValue.pointer = ir.Pointer
	anValue.reference = ir.Point

	// type
	value.SetIsFromDB(true)
	value.SetType(GetTypeFromDB(ir.TypeID))
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
