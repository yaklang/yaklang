package ssa

import (
	"sync"
)

// TODO
// data flow graph node
type Node interface {
	String() string

	GetType() Types

	GetUsers() []User
	GetValues() []Value
}

type Value interface {
	Node

	String() string

	GetUsers() []User
	AddUser(User)
	RemoveUser(User)

	SetType(Types)
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

	// pos
	Pos() string
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

	// package
	Package *Package

	Param  []*Parameter
	Return []*Return

	// type
	ParamTyp    []Types
	ReturnTyp   []Types
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
	instReg     map[Instruction]string // instruction -> virtual register
	symbolTable map[string][]Value

	// ssa error
	err SSAErrors

	// for builder
	builder *FunctionBuilder
}

func (f *Function) GetType() Types {
	return nil
}

func (f *Function) SetType(ts Types) {
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

func (b *BasicBlock) GetType() Types {
	return nil
}

func (b *BasicBlock) SetType(ts Types) {
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
	typs Types

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
func (a *anInstruction) GetType() Types {
	return a.typs
}

func (a *anInstruction) SetType(ts Types) {
	a.typs = ts
}

// value

// ----------- Phi
type Phi struct {
	anInstruction
	Edge []Value // edge[i] from phi.Block.Preds[i]
	user []User
	// for build
	wit1, wit2 Value // witness for trivial-phi
	variable   string
}

var _ Node = (*Phi)(nil)
var _ Value = (*Phi)(nil)
var _ User = (*Phi)(nil)
var _ Instruction = (*Phi)(nil)

// ----------- Const
// only Value
type Const struct {
	user  []User
	value any
	typ   Types
	str   string

	// other
	Unary int
}

// get type
func (c Const) GetType() Types {
	return c.typ
}

func (c *Const) SetType(ts Types) {
	c.typ = ts
}

var _ Node = (*Const)(nil)
var _ Value = (*Const)(nil)

// ----------- Parameter
type Parameter struct {
	variable    string
	Func        *Function
	isFreevalue bool
	typs        Types

	user []User
}

func (p *Parameter) GetType() Types {
	return p.typs
}

func (p *Parameter) SetType(ts Types) {
	p.typs = ts
}

var _ Node = (*Parameter)(nil)
var _ Value = (*Parameter)(nil)

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

	// call is a value
	user []User

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
	OpShl    BinaryOpcode = iota // <<
	OpShr                        // >>
	OpAnd                        // &
	OpAndNot                     // &^
	OpOr                         // |
	OpXor                        // ^
	OpAdd                        // +
	OpSub                        // -
	OpMul                        // *
	OpDiv                        // /
	OpMod                        // %
	OpGt                         // >
	OpLt                         // <
	OpGtEq                       // >=
	OpLtEq                       // <=
	OpEq                         // ==
	OpNotEq                      // != <>
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

type UnaryOpcode int

const (
	OpNot UnaryOpcode = iota
	OpPlus
	OpNeg
	OpChan
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

// special instruction ------------------------------------------

// ----------- Interface
// instruction + value + user
type Interface struct {
	anInstruction

	// when slice
	low, high, max Value

	parentI *Interface // parent interface

	// field
	field map[Value]*Field // field.key->field

	// when slice or map
	Len, Cap Value

	users []User
}

var _ Node = (*Interface)(nil)
var _ Value = (*Interface)(nil)
var _ User = (*Interface)(nil)
var _ Instruction = (*Interface)(nil)

// instruction
// ----------- Field
type Field struct {
	anInstruction

	// field
	Key Value
	I   Value

	// capture by other function
	OutCapture bool

	update []Value // value

	users []User

	//TODO:map[users]update
	// i can add the map[users]update,
	// to point what update value when user use this field

}

var _ Node = (*Field)(nil)
var _ Value = (*Field)(nil)
var _ User = (*Field)(nil)
var _ Instruction = (*Field)(nil)

// ----------- Update
type Update struct {
	anInstruction
	value   Value
	address *Field
}

var _ Node = (*Update)(nil)
var _ Value = (*Update)(nil)
var _ User = (*Update)(nil)
var _ Instruction = (*Update)(nil)
