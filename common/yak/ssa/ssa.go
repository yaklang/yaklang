package ssa

import (
	"sync"
)

// TODO
// data flow graph node
type Node interface {
	String() string

	GetType() Type

	GetUsers() []User
	GetValues() []Value
}

type Value interface {
	Node

	String() string

	// user
	GetUsers() []User
	AddUser(User)
	RemoveUser(User)

	// type
	SetType(Type)
}

type User interface {
	Node

	String() string

	GetValues() []Value
	AddValue(Value)

	ReplaceValue(Value, Value)
}

type Instruction interface {
	GetParent() *Function
	GetBlock() *BasicBlock

	String() string
	// asm
	// ParseByString(string) *Function

	// error
	NewError(ErrorKind, ErrorTag, string, ...any)

	// pos
	Pos() string
}

// both instruction and value
type InstructionValue interface {
	Instruction
	Value

	// variable
	GetVariable() string
	SetVariable(string)
}
type Program struct {
	// package list
	Packages []*Package

	// for build
	buildOnece sync.Once
}

type Package struct {
	Name string
	// point to program
	Prog *Program
	// function list
	Funcs []*Function
}

// implement Value
type Function struct {
	Name string

	Type *FunctionType

	// package
	Package *Package

	Param  []*Parameter
	Return []*Return

	hasEllipsis bool

	// BasicBlock list
	Blocks     []*BasicBlock
	EnterBlock *BasicBlock
	ExitBlock  *BasicBlock

	// anonymous function in this function
	AnonFuncs []*Function

	// if this function is anonFunc
	parent     *Function  // parent function if anonymous function; nil if global function.
	FreeValues []Value    // the value, captured variable form parent-function,
	symbol     *Interface // for function symbol table

	// User
	user []User
	Pos  *Position // current position

	// for instruction
	InstReg     map[Instruction]string // instruction -> virtual register
	symbolTable map[string][]InstructionValue

	// extern lib
	externInstance map[string]Value // lib and value
	externType     map[string]Type

	// ssa error
	err SSAErrors

	// for builder
	builder *FunctionBuilder
}

func (f *Function) GetType() Type {
	return f.Type
}

func (f *Function) SetType(t Type) {
	if ft, ok := t.(*FunctionType); ok {
		f.Type = ft
	}
}

var _ Node = (*Function)(nil)
var _ Value = (*Function)(nil)

// implement Value
type BasicBlock struct {
	Index int
	Name  string
	// function
	Parent *Function
	// basicblock graph
	Preds, Succs []*BasicBlock

	/*
		if Condition == true: this block reach
	*/
	Condition Value

	// instruction list
	Instrs []Instruction
	Phis   []*Phi

	// for build
	finish        bool // if emitJump finish!
	isSealed      bool
	inCompletePhi []*Phi // variable -> phi
	Skip          bool   // for phi build, avoid recursive

	// User
	user []User
}

func (b *BasicBlock) GetType() Type {
	return nil
}

func (b *BasicBlock) SetType(ts Type) {
}

var _ Node = (*BasicBlock)(nil)
var _ Value = (*BasicBlock)(nil)

type Position struct {
	// SourceCodeFilePath *string
	SourceCode  string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

type anInstruction struct {
	// function
	Func *Function
	// basicblock
	Block *BasicBlock
	// type
	typs Type

	variable string
	// source code position
	pos *Position
}

// implement instruction
func (a *anInstruction) GetBlock() *BasicBlock { return a.Block }
func (a *anInstruction) GetParent() *Function  { return a.Func }
func (a *anInstruction) Pos() string {
	if a.pos != nil {
		return a.pos.String()
	} else {
		return ""
	}
}
func (a *anInstruction) GetType() Type {
	return a.typs
}

func (a *anInstruction) SetType(ts Type) {
	a.typs = ts
}

func (a *anInstruction) SetVariable(name string) {
	if a.variable == "" {
		a.variable = name
	}
}

func (a *anInstruction) GetVariable() string {
	return a.variable
}

// value

// ----------- Phi
type Phi struct {
	anInstruction
	Edge []Value // edge[i] from phi.Block.Preds[i]
	user []User
	// for build
	wit1, wit2 Value // witness for trivial-phi
}

var _ Node = (*Phi)(nil)
var _ Value = (*Phi)(nil)
var _ User = (*Phi)(nil)
var _ Instruction = (*Phi)(nil)
var _ InstructionValue = (*Phi)(nil)

// ----------- Const
// constinst also have block pointer, which block set this const to variable
type ConstInst struct {
	Const
	anInstruction
}

func (c *ConstInst) GetType() Type {
	return c.Const.GetType()
}

func (c *ConstInst) SetType(ts Type) {
	// c.typs = ts
}

var _ Node = (*ConstInst)(nil)
var _ Value = (*ConstInst)(nil)
var _ Instruction = (*ConstInst)(nil)
var _ InstructionValue = (*ConstInst)(nil)

type Undefine struct {
	anInstruction
	user   []User
	values []Value
}

var _ Node = (*Undefine)(nil)
var _ Value = (*Undefine)(nil)
var _ User = (*Undefine)(nil)
var _ Instruction = (*Undefine)(nil)
var _ InstructionValue = (*Undefine)(nil)

// const only Value
type Const struct {
	user  []User
	value any
	// only one type
	typ Type
	str string

	// other
	Unary int
}

// get type
func (c *Const) GetType() Type {
	return c.typ
}

func (c *Const) SetType(ts Type) {
	// const don't need set type
}

var _ Node = (*Const)(nil)
var _ Value = (*Const)(nil)

// ----------- Parameter
type Parameter struct {
	variable    string
	Func        *Function
	isFreevalue bool
	typs        Type

	values []Value

	users []User
}

func (p *Parameter) GetType() Type {
	return p.typs
}

func (p *Parameter) SetType(ts Type) {
	p.typs = ts
}

var _ Node = (*Parameter)(nil)
var _ Value = (*Parameter)(nil)
var _ User = (*Parameter)(nil)

// control-flow instructions  ----------------------------------------
// jump / if / return / call / switch

// ----------- Jump
// The Jump instruction transfers control to the sole successor of its
// owning block.
//
// the block containing Jump instruction only have one successor block
type Jump struct {
	anInstruction
	To *BasicBlock
}

var _ Instruction = (*Jump)(nil)

// ----------- IF
// The If instruction transfers control to one of the two successors
// of its owning block, depending on the boolean Cond: the first if
// true, the second if false.
type If struct {
	anInstruction
	Cond  Value
	True  *BasicBlock
	False *BasicBlock
}

var _ Node = (*If)(nil)
var _ User = (*If)(nil)
var _ Instruction = (*If)(nil)

// ----------- For
// for loop
type Loop struct {
	anInstruction

	Body, Exit *BasicBlock

	Init, Cond, Step Value
	Key              *Phi
}

var _ Node = (*Loop)(nil)
var _ User = (*Loop)(nil)
var _ Instruction = (*Loop)(nil)

// ----------- Return
// The Return instruction returns values and control back to the calling
// function.
type Return struct {
	anInstruction
	Results []Value
}

var _ Node = (*Return)(nil)
var _ User = (*Return)(nil)
var _ Instruction = (*Return)(nil)

// ----------- Call
// call instruction call method function  with args as argument
type Call struct {
	anInstruction

	// for call function
	Method Value
	Args   []Value

	// go function
	Async bool

	// call is a value
	user []User

	value []Value

	binding []Value

	// caller
	caller Value
	// ~ drop error
	isDropError bool
}

var _ Node = (*Call)(nil)
var _ Value = (*Call)(nil)
var _ User = (*Call)(nil)
var _ Instruction = (*Call)(nil)
var _ InstructionValue = (*Call)(nil)

// ----------- Switch
type SwitchLabel struct {
	Value Value
	Dest  *BasicBlock
}

func NewSwitchLabel(v Value, dest *BasicBlock) SwitchLabel {
	return SwitchLabel{
		Value: v,
		Dest:  dest,
	}
}

type Switch struct {
	anInstruction

	Cond         Value
	DefaultBlock *BasicBlock

	Label []SwitchLabel
}

var _ Node = (*Switch)(nil)
var _ User = (*Switch)(nil)
var _ Instruction = (*Switch)(nil)

// data-flow instructions  ----------------------------------------
// BinOp / UnOp

type BinaryOpcode int

const (
	// Binary
	OpShl BinaryOpcode = iota // <<

	OpShr    // >>
	OpAnd    // &
	OpAndNot // &^
	OpOr     // |
	OpXor    // ^
	OpAdd    // +
	OpSub    // -
	OpDiv    // /
	OpMod    // %
	// mul
	OpMul // *

	// boolean opcode
	OpGt    // >
	OpLt    // <
	OpGtEq  // >=
	OpLtEq  // <=
	OpEq    // ==
	OpNotEq // != <>
)

// ----------- BinOp
type BinOp struct {
	anInstruction
	Op   BinaryOpcode
	X, Y Value
	user []User
}

var _ Value = (*BinOp)(nil)
var _ User = (*BinOp)(nil)
var _ Node = (*BinOp)(nil)
var _ Instruction = (*BinOp)(nil)
var _ InstructionValue = (*BinOp)(nil)

type UnaryOpcode int

const (
	OpNone       UnaryOpcode = iota
	OpNot                    // !
	OpPlus                   // +
	OpNeg                    // -
	OpChan                   // ->
	OpBitwiseNot             // ^
)

type UnOp struct {
	anInstruction

	Op UnaryOpcode
	X  Value

	user []User
}

var _ Value = (*UnOp)(nil)
var _ User = (*UnOp)(nil)
var _ Node = (*UnOp)(nil)
var _ Instruction = (*UnOp)(nil)
var _ InstructionValue = (*UnOp)(nil)

// special instruction ------------------------------------------

// ----------- Interface
// instruction + value + user
// use-chain: *interface(self) -> multiple field(value)
type Interface struct {
	anInstruction

	// when slice
	low, high, max Value

	parentI User // parent interface

	IsNew bool

	// Field
	// Field map[Value]*Field // field.key->field
	value []Value

	// when slice or map
	Len, Cap Value

	users []User

	// for extern lib
	buildField func(key string) Value
}

var _ Node = (*Interface)(nil)
var _ Value = (*Interface)(nil)
var _ User = (*Interface)(nil)
var _ Instruction = (*Interface)(nil)
var _ InstructionValue = (*Interface)(nil)

// instruction
// ----------- Field
// use-chain: interface(user) -> *field(self) -> multiple update(value) -> value
type Field struct {
	anInstruction

	// field
	Key Value
	I   User // this I type must be InterfaceType

	// capture by other function
	OutCapture bool

	Update []Value // value

	users []User

	//TODO:map[users]update
	// i can add the map[users]update,
	// to point what update value when user use this field

}

var _ Node = (*Field)(nil)
var _ Value = (*Field)(nil)
var _ User = (*Field)(nil)
var _ Instruction = (*Field)(nil)
var _ InstructionValue = (*Field)(nil)

// ----------- Update
// use-chain: field(user) -> *update -> value
type Update struct {
	anInstruction

	Value   Value
	Address *Field
}

var _ Node = (*Update)(nil)
var _ Value = (*Update)(nil)
var _ User = (*Update)(nil)
var _ Instruction = (*Update)(nil)
var _ InstructionValue = (*Update)(nil)

// ----------- Type-cast
// cast value -> type
type TypeCast struct {
	anInstruction

	Value Value
	user  []User
}

var _ Node = (*TypeCast)(nil)
var _ Value = (*TypeCast)(nil)
var _ User = (*TypeCast)(nil)
var _ Instruction = (*TypeCast)(nil)
var _ InstructionValue = (*TypeCast)(nil)

// ----------- assert
type Assert struct {
	anInstruction
	Cond     Value
	Msg      string
	MsgValue Value
}

var _ Node = (*Assert)(nil)
var _ User = (*Assert)(nil)
var _ Instruction = (*Assert)(nil)

// ------------- Next
type Next struct {
	anInstruction
	Iter Value
	user []User
}

var _ Node = (*Next)(nil)
var _ User = (*Next)(nil)
var _ Value = (*Next)(nil)
var _ Instruction = (*Next)(nil)
var _ InstructionValue = (*Next)(nil)
