package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Instruction2IrCode : marshal instruction to ir code, used in cache, to save to database
func Instruction2IrCode(inst Instruction, ir *ssadb.IrCode) error {
	if ir.ID != uint(inst.GetId()) {
		return utils.Errorf("marshal instruction id not match")
	}

	instruction2IrCode(inst, ir)
	value2IrCode(inst, ir)

	function2IrCode(inst, ir)
	basicBlock2IrCode(inst, ir)
	ir.SetExtraInfo(marshalExtraInformation(inst))
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

func fitRange(c *ssadb.IrCode, rangeIns *Range) {
	if rangeIns == nil {
		log.Warnf("(BUG or in DEBUG MODE) Range not found for %s", c.Name)
		return
	}
	c.SourceCodeHash = codec.Sha256(rangeIns.GetEditor().GetSourceCode())
	start, end := rangeIns.GetOffsetRange()
	c.SourceCodeStartOffset = int64(start)
	c.SourceCodeEndOffset = int64(end)
}

func instruction2IrCode(inst Instruction, ir *ssadb.IrCode) {
	// name
	ir.Name = inst.GetName()
	ir.VerboseName = inst.GetVerboseName()
	ir.ShortVerboseName = inst.GetShortVerboseName()

	// opcode
	ir.Opcode = int64(inst.GetOpcode())
	ir.OpcodeName = SSAOpcode2Name[inst.GetOpcode()]

	var codeRange *Range
	if ret := inst.GetRange(); ret != nil {
		codeRange = ret
	} else if ret := inst.GetFunc().GetRange(); ret != nil {
		log.Warnf("Fallback, the %v is not set range, use its function instance' ", inst.GetName())
		inst.SetRange(ret)
		codeRange = ret
	}
	if codeRange == nil {
		log.Warnf("Range not found for %s", inst.GetName())
	} else {
		fitRange(ir, codeRange)
	}
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

		if !ir.IsBlock {
			if block, err := NewInstructionFromLazy(ir.CurrentBlock, ToBasicBlock); err == nil {
				inst.SetBlock(block)
			} else {
				log.Errorf("BUG: set CurrentBlock[%d]: %v", ir.CurrentBlock, err)
			}
		}
	}

	inst.SetExtern(ir.IsExternal)
}

func value2IrCode(inst Instruction, ir *ssadb.IrCode) {
	value, ok := ToValue(inst)
	if !ok {
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
	for name := range value.GetAllVariables() {
		ir.Variable = append(ir.Variable, name)
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

	ir.TypeID = SaveTypeToDB(value.GetType())
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
