package ssa

import "golang.org/x/exp/slices"

type Opcode string

const (
	OpUnknown      Opcode = "unknown"
	OpFunction     Opcode = "Function"
	OpBasicBlock   Opcode = "BasicBlock"
	OpParameter    Opcode = "Parameter"
	OpFreeValue    Opcode = "FreeValue"
	OpExternLib    Opcode = "ExternLib"
	OpPhi          Opcode = "Phi"
	OpConstInst    Opcode = "ConstInst"
	OpUndefined    Opcode = "Undefined"
	OpBinOp        Opcode = "BinOp"
	OpUnOp         Opcode = "UnOp"
	OpCall         Opcode = "Call"
	OpSideEffect   Opcode = "SideEffect"
	OpReturn       Opcode = "Return"
	OpMake         Opcode = "Make"
	OpField        Opcode = "Field"
	OpUpdate       Opcode = "Update"
	OpNext         Opcode = "Next"
	OpAssert       Opcode = "Assert"
	OpTypeCast     Opcode = "TypeCast"
	OpTypeValue    Opcode = "TypeValue"
	OpErrorHandler Opcode = "ErrorHandler"
	OpPanic        Opcode = "Panic"
	OpRecover      Opcode = "Recover"
	OpJump         Opcode = "Jump"
	OpIf           Opcode = "If"
	OpLoop         Opcode = "Loop"
	OpSwitch       Opcode = "Switch"
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
