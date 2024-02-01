package ssa

import "golang.org/x/exp/slices"

type Opcode string

const (
	OpUnknown Opcode = "unknown"

	OpFunction     = "Function"
	OpBasicBlock   = "BasicBlock"
	OpParameter    = "Parameter"
	OpFreeValue    = "FreeValue"
	OpExternLib    = "ExternLib"
	OpPhi          = "Phi"
	OpConstInst    = "ConstInst"
	OpUndefined    = "Undefined"
	OpBinOp        = "BinOp"
	OpUnOp         = "UnOp"
	OpCall         = "Call"
	OpSideEffect   = "SideEffect"
	OpReturn       = "Return"
	OpMake         = "Make"
	OpField        = "Field"
	OpUpdate       = "Update"
	OpNext         = "Next"
	OpAssert       = "Assert"
	OpTypeCast     = "TypeCast"
	OpTypeValue    = "TypeValue"
	OpErrorHandler = "ErrorHandler"
	OpPanic        = "Panic"
	OpRecover      = "Recover"
	OpJump         = "Jump"
	OpIf           = "If"
	OpLoop         = "Loop"
	OpSwitch       = "Switch"
)

func (i *Function) GetOpcode() Opcode   { return OpFunction }
func (i *BasicBlock) GetOpcode() Opcode { return OpBasicBlock }
func (i *Parameter) GetOpcode() Opcode {
	if i.IsFreeValue {
		return OpFreeValue
	}
	return OpParameter
}
func (i *ExternLib) GetOpcode() Opcode    { return OpExternLib }
func (i *Phi) GetOpcode() Opcode          { return OpPhi }
func (i *ConstInst) GetOpcode() Opcode    { return OpConstInst }
func (i *Undefined) GetOpcode() Opcode    { return OpUndefined }
func (i *BinOp) GetOpcode() Opcode        { return OpBinOp }
func (i *UnOp) GetOpcode() Opcode         { return OpUnOp }
func (i *Call) GetOpcode() Opcode         { return OpCall }
func (i *SideEffect) GetOpcode() Opcode   { return OpSideEffect }
func (i *Return) GetOpcode() Opcode       { return OpReturn }
func (i *Make) GetOpcode() Opcode         { return OpMake }
func (i *Field) GetOpcode() Opcode        { return OpField }
func (i *Update) GetOpcode() Opcode       { return OpUpdate }
func (i *Next) GetOpcode() Opcode         { return OpNext }
func (i *Assert) GetOpcode() Opcode       { return OpAssert }
func (i *TypeCast) GetOpcode() Opcode     { return OpTypeCast }
func (i *TypeValue) GetOpcode() Opcode    { return OpTypeValue }
func (i *ErrorHandler) GetOpcode() Opcode { return OpErrorHandler }
func (i *Panic) GetOpcode() Opcode        { return OpPanic }
func (i *Recover) GetOpcode() Opcode      { return OpRecover }
func (i *Jump) GetOpcode() Opcode         { return OpJump }
func (i *If) GetOpcode() Opcode           { return OpIf }
func (i *Loop) GetOpcode() Opcode         { return OpLoop }
func (i *Switch) GetOpcode() Opcode       { return OpSwitch }

func IsControlInstruction(i Instruction) bool {
	return slices.Index([]Opcode{OpErrorHandler, OpJump, OpIf, OpLoop, OpSwitch}, i.GetOpcode()) != -1
}
