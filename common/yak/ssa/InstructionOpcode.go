package ssa

import "golang.org/x/exp/slices"

type Opcode int

const (
	SSAOpcodeUnKnow Opcode = iota
	SSAOpcodeAssert
	SSAOpcodeBasicBlock
	SSAOpcodeBinOp
	SSAOpcodeCall
	SSAOpcodeConstInst
	SSAOpcodeErrorHandler
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

var SSAOpcode2Name = map[Opcode]string{
	SSAOpcodeAssert:          "Assert",
	SSAOpcodeBasicBlock:      "BasicBlock",
	SSAOpcodeBinOp:           "BinOp",
	SSAOpcodeCall:            "Call",
	SSAOpcodeConstInst:       "ConstInst",
	SSAOpcodeErrorHandler:    "ErrorHandler",
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

func (i *Function) GetOpcode() Opcode        { return SSAOpcodeFunction }
func (i *BasicBlock) GetOpcode() Opcode      { return SSAOpcodeBasicBlock }
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
func (i *Panic) GetOpcode() Opcode        { return SSAOpcodePanic }
func (i *Recover) GetOpcode() Opcode      { return SSAOpcodeRecover }
func (i *Jump) GetOpcode() Opcode         { return SSAOpcodeJump }
func (i *If) GetOpcode() Opcode           { return SSAOpcodeIf }
func (i *Loop) GetOpcode() Opcode         { return SSAOpcodeLoop }
func (i *Switch) GetOpcode() Opcode       { return SSAOpcodeSwitch }

func IsControlInstruction(i Instruction) bool {
	return slices.Index([]Opcode{SSAOpcodeErrorHandler, SSAOpcodeJump, SSAOpcodeIf, SSAOpcodeLoop, SSAOpcodeSwitch}, i.GetOpcode()) != -1
}
