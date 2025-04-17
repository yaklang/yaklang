package ssa

import (
	"github.com/yaklang/yaklang/common/log"
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
				codeRange = ret.Insts[0].GetRange()
			}
		case *Function:
			if len(ret.Blocks) > 0 {
				codeRange = ret.Blocks[0].GetRange()
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

	// value
	for _, def := range value.GetValues() {
		if def == nil {
			log.Infof("BUG: value[%s: %s] def is nil", value, value.GetRange())
			continue
		}
		ir.Defs = append(ir.Defs, int64(def.GetId()))
	}

	// user
	for _, user := range value.GetUsers() {
		ir.Users = append(ir.Users, user.GetId())

		if call, ok := ToCall(user); ok {
			if call.Method.GetId() == value.GetId() {
				// ir.IsCalled
				ir.CalledBy = append(ir.CalledBy, call.GetId())
			}
		}
	}

	for _, oc := range value.GetOccultation() {
		ir.Occulatation = append(ir.Occulatation, oc.GetId())
	}

	// Object
	ir.IsObject = value.IsObject()
	if ir.IsObject {
		ir.ObjectMembers = make(ssadb.Int64Map, 0)
		value.ForEachMember(func(k, v Value) bool {
			ir.ObjectMembers.Append(k.GetId(), v.GetId())
			return true
		})
	}

	// member
	ir.IsObjectMember = value.IsMember()
	if ir.IsObjectMember {
		ir.ObjectParent = value.GetObject().GetId()
		ir.ObjectKey = value.GetKey().GetId()
	}

	// variable
	for name, variable := range value.GetAllVariables() {
		ir.Variable = append(ir.Variable, name)
		SaveVariableOffset(variable, name)
	}

	// mask
	for _, m := range value.GetMask() {
		ir.MaskedCodes = append(ir.MaskedCodes, m.GetId())
	}
	ir.String = value.String()
	for _, r := range value.GetPointer() {
		ir.Pointer = append(ir.Pointer, r.GetId())
	}
	if point := value.GetReference(); point != nil {
		ir.Point = point.GetId()
	}

	ir.TypeID = SaveTypeToDB(value.GetType(), inst.GetProgramName())
}

func (c *Cache) valueFromIrCode(inst Instruction, ir *ssadb.IrCode) {
	value, ok := ToValue(inst)
	if !ok {
		return
	}

	getUser := func(id int64) User {
		if user, ok := ToUser(c.GetInstruction(id)); ok {
			return user
		}
		return nil
	}
	getValue := func(id int64) Value {
		if value, ok := ToValue(c.GetInstruction(id)); ok {
			return value
		}
		return nil
	}
	// value : none to do

	//  user
	for _, user := range ir.Users {
		value.AddUser(getUser(user))
	}

	//  occulatation
	for _, oc := range ir.Occulatation {
		value.AddOccultation(getValue(oc))
	}

	// object
	if ir.IsObject {
		ir.ObjectMembers.ForEach(func(k, v int64) {
			value.AddMember(getValue(k), getValue(v))
		})
	}

	// object member
	if ir.IsObjectMember {
		value.SetObject(getValue(ir.ObjectParent))
		value.SetKey(getValue(ir.ObjectKey))
	}

	// variable
	// for _, name := range ir.Variable {
	// 	value.AddVariable(NewVariable(name))
	// }

	// mask
	for _, m := range ir.MaskedCodes {
		value.AddMask(getValue(m))
	}

	// reference
	for _, r := range ir.Pointer {
		value.AddPointer(getValue(r))
	}
	if ir.Point != 0 {
		value.SetReference(getValue(ir.Point))
	}

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
		if formArg == nil {
			continue
		}
		ir.FormalArgs = append(ir.FormalArgs, int64(formArg.GetId()))
	}

	for _, fv := range f.FreeValues {
		if fv == nil {
			continue
		}
		ir.FreeValues = append(ir.FreeValues, int64(fv.GetId()))
	}

	for _, returnIns := range f.Return {
		if returnIns == nil {
			continue
		}
		ir.ReturnCodes = append(ir.ReturnCodes, int64(returnIns.GetId()))
	}
	for _, sideEffect := range f.SideEffects {
		if sideEffect == nil {
			continue
		}
	}

	for _, b := range f.Blocks {
		if b == nil {
			continue
		}
		ir.CodeBlocks = append(ir.CodeBlocks, int64(b.GetId()))
	}

	if f.EnterBlock != nil {
		ir.EnterBlock = int64(f.EnterBlock.GetId())
	}
	if f.ExitBlock != nil {
		ir.ExitBlock = int64(f.ExitBlock.GetId())
	}
	if f.DeferBlock != nil {
		ir.DeferBlock = int64(f.DeferBlock.GetId())
	}

	for _, subFunc := range f.ChildFuncs {
		ir.ChildrenFunction = append(ir.ChildrenFunction, int64(subFunc.GetId()))
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
		ir.PredBlock = append(ir.PredBlock, int64(pred.GetId()))
	}

	ir.SuccBlock = make([]int64, 0, len(block.Succs))
	for _, succ := range block.Succs {
		ir.SuccBlock = append(ir.SuccBlock, int64(succ.GetId()))
	}

	ir.Phis = make([]int64, 0, len(block.Phis))
	for _, phi := range block.Phis {
		ir.Phis = append(ir.Phis, int64(phi.GetId()))
	}
}

func basicBlockFromIrCode(inst Instruction, ir *ssadb.IrCode) {
}
