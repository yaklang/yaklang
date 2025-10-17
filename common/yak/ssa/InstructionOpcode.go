package ssa

import (
	"github.com/yaklang/yaklang/common/utils/memedit"
	"golang.org/x/exp/slices"
)

type Opcode int

const (
	SSAOpcodeUnKnow Opcode = iota
	SSAOpcodeAssert
	SSAOpcodeBasicBlock
	SSAOpcodeBinOp
	SSAOpcodeCall
	SSAOpcodeConstInst
	SSAOpcodeErrorHandler
	SSAOpcodeErrorCatch
	SSAOpcodeExternLib
	SSAOpcodeIf
	SSAOpcodeJump
	SSAOpcodeLoop
	SSAOpcodeMake
	SSAOpcodeNext
	SSAOpcodePanic
	SSAOpcodeParameter
	SSAOpcodeFreeValue
	SSAOpcodeParameterMember
	SSAOpcodePhi
	SSAOpcodeRecover
	SSAOpcodeReturn
	SSAOpcodeSideEffect
	SSAOpcodeSwitch
	SSAOpcodeTypeCast
	SSAOpcodeTypeValue
	SSAOpcodeUnOp
	SSAOpcodeUndefined
	SSAOpcodeFunction
)

func (op Opcode) String() string {
	if name, ok := SSAOpcode2Name[op]; ok {
		return name
	}
	return SSAOpcode2Name[SSAOpcodeUnKnow]
}

var SSAOpcode2Name = map[Opcode]string{
	SSAOpcodeUnKnow:          "UnKnow",
	SSAOpcodeAssert:          "Assert",
	SSAOpcodeBasicBlock:      "BasicBlock",
	SSAOpcodeBinOp:           "BinOp",
	SSAOpcodeCall:            "Call",
	SSAOpcodeConstInst:       "ConstInst",
	SSAOpcodeErrorHandler:    "ErrorHandler",
	SSAOpcodeErrorCatch:      "ErrorCatch",
	SSAOpcodeExternLib:       "ExternLib",
	SSAOpcodeIf:              "If",
	SSAOpcodeJump:            "Jump",
	SSAOpcodeLoop:            "Loop",
	SSAOpcodeMake:            "Make",
	SSAOpcodeNext:            "Next",
	SSAOpcodePanic:           "Panic",
	SSAOpcodeParameter:       "Parameter",
	SSAOpcodeFreeValue:       "FreeValue",
	SSAOpcodeParameterMember: "ParameterMember",
	SSAOpcodePhi:             "Phi",
	SSAOpcodeRecover:         "Recover",
	SSAOpcodeReturn:          "Return",
	SSAOpcodeSideEffect:      "SideEffect",
	SSAOpcodeSwitch:          "Switch",
	SSAOpcodeTypeCast:        "TypeCast",
	SSAOpcodeTypeValue:       "TypeValue",
	SSAOpcodeUnOp:            "UnOp",
	SSAOpcodeUndefined:       "Undefined",
	SSAOpcodeFunction:        "Function",
}

func (i *Function) GetOpcode() Opcode   { return SSAOpcodeFunction }
func (i *BasicBlock) GetOpcode() Opcode { return SSAOpcodeBasicBlock }
func (i *BasicBlock) _GetRange() *memedit.Range {
	if i == nil || i.anValue.id <= 0 {
		return nil
	}
	if i.anValue.R != nil {
		return i.anValue.R
	}
	// if len(i.Insts) == 1 {
	// 	return i.GetInstructionById(i.Insts[0]).GetRange()
	// } else if len(i.Insts) > 1 {
	// 	first := i.GetInstructionById(i.Insts[0])
	// 	last := i.GetInstructionById(i.Insts[len(i.Insts)-1])
	// 	firstRange := first.GetRange()
	// 	lastRange := last.GetRange()
	// 	if firstRange != nil && lastRange != nil {
	// 		return first.GetRange().GetEditor().GetRangeOffset(firstRange.GetStartOffset(), lastRange.GetEndOffset())
	// 	}
	// }
	return nil
}

// func (i *BasicBlock) GetRange() *memedit.Range {
// 	result := i._GetRange()
// 	if result != nil && i.anValue.R == nil {
// 		i.SetRange(result)
// 	}
// 	return result
// }

func (i *Function) _GetRange() *memedit.Range {
	if i == nil || i.anValue.id <= 0 {
		return nil
	}

	if i.anValue.R != nil {
		return i.anValue.R
	}

	if i.EnterBlock <= 0 {
		log.Warnf("function: %v's enter_block is not set, use the entry_block's range fallback", i.GetName())
		return nil
	}
	enter, ok := i.GetBasicBlockByID(i.EnterBlock)
	if ok && enter != nil {
		log.Warnf("funcion: %v's range is not set, use the entry_block's range fallback", i.GetName())
		return enter.GetRange()
	}

	return nil
}

// func (i *Function) GetRange() *memedit.Range {
// result := i._GetRange()
// if result != nil && i.anValue.R == nil {
// 	i.SetRange(result)
// }
// return result
// }

func (i *ParameterMember) GetOpcode() Opcode { return SSAOpcodeParameterMember }
func (i *Parameter) GetOpcode() Opcode {
	if i.IsFreeValue {
		return SSAOpcodeFreeValue
	}
	return SSAOpcodeParameter
}
func (i *ExternLib) GetOpcode() Opcode    { return SSAOpcodeExternLib }
func (i *Phi) GetOpcode() Opcode          { return SSAOpcodePhi }
func (i *ConstInst) GetOpcode() Opcode    { return SSAOpcodeConstInst }
func (i *Undefined) GetOpcode() Opcode    { return SSAOpcodeUndefined }
func (i *BinOp) GetOpcode() Opcode        { return SSAOpcodeBinOp }
func (i *UnOp) GetOpcode() Opcode         { return SSAOpcodeUnOp }
func (i *Call) GetOpcode() Opcode         { return SSAOpcodeCall }
func (i *SideEffect) GetOpcode() Opcode   { return SSAOpcodeSideEffect }
func (i *Return) GetOpcode() Opcode       { return SSAOpcodeReturn }
func (i *Make) GetOpcode() Opcode         { return SSAOpcodeMake }
func (i *Next) GetOpcode() Opcode         { return SSAOpcodeNext }
func (i *Assert) GetOpcode() Opcode       { return SSAOpcodeAssert }
func (i *TypeCast) GetOpcode() Opcode     { return SSAOpcodeTypeCast }
func (i *TypeValue) GetOpcode() Opcode    { return SSAOpcodeTypeValue }
func (i *ErrorHandler) GetOpcode() Opcode { return SSAOpcodeErrorHandler }
func (i *ErrorCatch) GetOpcode() Opcode   { return SSAOpcodeErrorCatch }
func (i *Panic) GetOpcode() Opcode        { return SSAOpcodePanic }
func (i *Recover) GetOpcode() Opcode      { return SSAOpcodeRecover }
func (i *Jump) GetOpcode() Opcode         { return SSAOpcodeJump }
func (i *If) GetOpcode() Opcode           { return SSAOpcodeIf }
func (i *Loop) GetOpcode() Opcode         { return SSAOpcodeLoop }
func (i *Switch) GetOpcode() Opcode       { return SSAOpcodeSwitch }

func IsControlInstruction(i Instruction) bool {
	return slices.Index([]Opcode{SSAOpcodeErrorHandler, SSAOpcodeJump, SSAOpcodeIf, SSAOpcodeLoop, SSAOpcodeSwitch}, i.GetOpcode()) != -1
}

func IsValueInstruction(i Instruction) bool {
	return slices.Index([]Opcode{SSAOpcodeErrorCatch, SSAOpcodePanic, SSAOpcodeBasicBlock, SSAOpcodeBinOp, SSAOpcodeCall, SSAOpcodeExternLib, SSAOpcodeFunction, SSAOpcodeConstInst, SSAOpcodeMake, SSAOpcodeNext, SSAOpcodeParameter, SSAOpcodeFreeValue, SSAOpcodeParameterMember, SSAOpcodePhi, SSAOpcodeRecover, SSAOpcodeReturn, SSAOpcodeSideEffect, SSAOpcodeTypeCast, SSAOpcodeTypeValue, SSAOpcodeUnOp, SSAOpcodeUndefined}, i.GetOpcode()) != -1
}

func IsUserInstruction(i Instruction) bool {
	return slices.Index([]Opcode{SSAOpcodeErrorCatch, SSAOpcodeErrorHandler, SSAOpcodeLoop, SSAOpcodeSwitch, SSAOpcodeIf, SSAOpcodeAssert, SSAOpcodePanic, SSAOpcodeBasicBlock, SSAOpcodeBinOp, SSAOpcodeCall, SSAOpcodeExternLib, SSAOpcodeFunction, SSAOpcodeConstInst, SSAOpcodeMake, SSAOpcodeNext, SSAOpcodeParameter, SSAOpcodeFreeValue, SSAOpcodeParameterMember, SSAOpcodePhi, SSAOpcodeRecover, SSAOpcodeReturn, SSAOpcodeSideEffect, SSAOpcodeTypeCast, SSAOpcodeTypeValue, SSAOpcodeUnOp, SSAOpcodeUndefined}, i.GetOpcode()) != -1
}

func CreateInstruction(op Opcode) Instruction {
	switch op {
	case SSAOpcodeFunction:
		return &Function{
			anValue: NewValue(),
		}
	case SSAOpcodeBasicBlock:
		return &BasicBlock{
			anValue: NewValue(),
		}
	case SSAOpcodeParameterMember:
		return &ParameterMember{
			anValue:              NewValue(),
			FormalParameterIndex: 0,
			parameterMemberInner: &parameterMemberInner{},
		}
	case SSAOpcodeFreeValue:
		return &Parameter{
			anValue:     NewValue(),
			IsFreeValue: true,
		}
	case SSAOpcodeParameter:
		return &Parameter{
			anValue:     NewValue(),
			IsFreeValue: false,
		}
	case SSAOpcodeExternLib:
		return &ExternLib{
			anValue: NewValue(),
		}
	case SSAOpcodePhi:
		return &Phi{
			anValue: NewValue(),
		}
	case SSAOpcodeConstInst:
		return &ConstInst{
			Const:   &Const{value: nil, str: "nil"},
			anValue: NewValue(),
		}
	case SSAOpcodeUndefined:
		return &Undefined{
			anValue: NewValue(),
		}
	case SSAOpcodeBinOp:
		return &BinOp{
			anValue: NewValue(),
		}
	case SSAOpcodeUnOp:
		return &UnOp{
			anValue: NewValue(),
		}
	case SSAOpcodeCall:
		return &Call{
			anValue: NewValue(),
		}
	case SSAOpcodeSideEffect:
		return &SideEffect{
			anValue: NewValue(),
		}
	case SSAOpcodeReturn:
		return &Return{
			anValue: NewValue(),
		}
	case SSAOpcodeMake:
		return &Make{
			anValue: NewValue(),
		}
	case SSAOpcodeNext:
		return &Next{
			anValue: NewValue(),
		}
	case SSAOpcodeAssert:
		return &Assert{
			anInstruction: NewInstruction(),
		}
	case SSAOpcodeTypeCast:
		return &TypeCast{
			anValue: NewValue(),
		}
	case SSAOpcodeTypeValue:
		return &TypeValue{
			anValue: NewValue(),
		}
	case SSAOpcodeErrorHandler:
		return &ErrorHandler{
			anInstruction: NewInstruction(),
		}
	case SSAOpcodeErrorCatch:
		return &ErrorCatch{
			anValue: NewValue(),
		}
	case SSAOpcodePanic:
		return &Panic{
			anValue: NewValue(),
		}
	case SSAOpcodeRecover:
		return &Recover{
			anValue: NewValue(),
		}
	case SSAOpcodeJump:
		return &Jump{
			anInstruction: NewInstruction(),
		}
	case SSAOpcodeIf:
		return &If{
			anInstruction: NewInstruction(),
		}
	case SSAOpcodeLoop:
		return &Loop{
			anInstruction: NewInstruction(),
		}
	case SSAOpcodeSwitch:
		return &Switch{
			anInstruction: NewInstruction(),
		}
	default:
		return nil
	}
}
