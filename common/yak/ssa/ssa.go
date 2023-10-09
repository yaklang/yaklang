package ssa

import (
	"sync"

	"github.com/samber/lo"
)

// TODO: save use-def chain in map[Node]struct{}

// data flow graph node
type Node interface {
	String() string

	GetType() Type

	GetUsers() []User
	GetValues() []Value
}

type Value interface {
	Node

	// user
	AddUser(User)
	RemoveUser(User)

	// type
	SetType(Type)
}

type User interface {
	Node

	AddValue(Value)
	RemoveValue(Value)

	ReplaceValue(Value, Value)
}

type anNode struct {
	user  map[User]struct{}
	value map[Value]struct{}
	// field map[*Field]struct{}
}

func NewNode() anNode {
	return anNode{
		user:  make(map[User]struct{}),
		value: make(map[Value]struct{}),
	}
}

func (n *anNode) GetUsers() []User {
	return lo.Keys(n.user)
}

func (n *anNode) GetValues() []Value {
	return lo.Keys(n.value)
}

func (n *anNode) AddUser(u User) {
	n.user[u] = struct{}{}
}

func (n *anNode) AddValue(v Value) {
	n.value[v] = struct{}{}
}

func (n *anNode) RemoveValue(v Value) {
	delete(n.value, v)
}
func (n *anNode) RemoveUser(u User) {
	delete(n.user, u)
}

// func (n *anNode) GetField() []*Field {
// 	return lo.Keys(n.field)
// }

// func (n *anNode) AddField(f *Field) {
// 	n.field[f] = struct{}{}
// }

// func (n *anNode) RemoveField(f *Field) {
// 	delete(n.field, f)
// }

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

	GetPosition() *Position
	SetPosition(pos *Position)
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
	buildOnce sync.Once
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
	parent     *Function // parent function if anonymous function; nil if global function.
	FreeValues []Value   // the value, captured variable form parent-function,
	symbol     *Make     // for function symbol table

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
	// BasicBlock graph
	Preds, Succs []*BasicBlock

	/*
		if Condition == true: this block reach
	*/
	Condition Value

	// instruction list
	Insts []Instruction
	Phis  []*Phi

	// error catch
	Handler *ErrorHandler

	// position
	pos *Position

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
	// BasicBlock
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
	t := a.typs
	if t == nil {
		return BasicTypes[Any]
	}
	return t
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

func (a *anInstruction) GetPosition() *Position {
	return a.pos
}

func (a *anInstruction) SetPosition(pos *Position) {
	a.pos = pos
}

// value

// ----------- Phi
type Phi struct {
	anInstruction
	anNode

	Edge []Value // edge[i] from phi.Block.Preds[i]
	//	what instruction create this control-flow merge?
	branch *Instruction // loop or if :
	// for build
	create     bool  // for ReadVariable method
	wit1, wit2 Value // witness for trivial-phi
}

var _ Node = (*Phi)(nil)
var _ Value = (*Phi)(nil)
var _ User = (*Phi)(nil)
var _ Instruction = (*Phi)(nil)
var _ InstructionValue = (*Phi)(nil)

// ----------- Const
// ConstInst also have block pointer, which block set this const to variable
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
var _ User = (*ConstInst)(nil)
var _ Instruction = (*ConstInst)(nil)
var _ InstructionValue = (*ConstInst)(nil)

type Undefine struct {
	anInstruction
	anNode
}

var _ Node = (*Undefine)(nil)
var _ Value = (*Undefine)(nil)
var _ User = (*Undefine)(nil)
var _ Instruction = (*Undefine)(nil)
var _ InstructionValue = (*Undefine)(nil)

// const only Value
type Const struct {
	anNode
	value any
	// only one type
	typ Type
	str string

	// other
	Unary int
}

// get type
func (c *Const) GetType() Type {
	t := c.typ
	if t == nil {
		t = BasicTypes[Any]
	}
	return t
}

func (c *Const) SetType(ts Type) {
	// const don't need set type
}

var _ Node = (*Const)(nil)
var _ Value = (*Const)(nil)

// ----------- Parameter
type Parameter struct {
	anNode

	variable    string
	Func        *Function
	IsFreeValue bool
	typs        Type
}

func (p *Parameter) GetType() Type {
	t := p.typs
	if t == nil {
		t = BasicTypes[Any]
		p.SetType(t)
	}
	return t
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
	// call is a value
	anNode

	// for call function
	Method Value
	Args   []Value

	// go function
	Async bool

	binding []Value

	// caller
	// caller Value
	// ~ drop error
	IsDropError bool
	IsEllipsis  bool
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

	OpLogicAnd // &&
	OpLogicOr  // ||

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
	OpIn    //  a in b

	OpSend // <-
)

// ----------- BinOp
type BinOp struct {
	anInstruction
	anNode
	Op   BinaryOpcode
	X, Y Value
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
	OpChan                   // <-
	OpBitwiseNot             // ^
)

type UnOp struct {
	anNode

	anInstruction

	Op UnaryOpcode
	X  Value
}

var _ Value = (*UnOp)(nil)
var _ User = (*UnOp)(nil)
var _ Node = (*UnOp)(nil)
var _ Instruction = (*UnOp)(nil)
var _ InstructionValue = (*UnOp)(nil)

// special instruction ------------------------------------------

// ----------- Make
// instruction + value + user
// use-chain: *interface(self) -> multiple field(value)
type Make struct {
	anInstruction
	anNode

	// when slice
	low, high, step Value

	parentI User // parent interface

	IsNew bool

	// when slice or map
	Len, Cap Value

	// for extern lib
	buildField func(key string) Value
}

var _ Node = (*Make)(nil)
var _ Value = (*Make)(nil)
var _ User = (*Make)(nil)
var _ Instruction = (*Make)(nil)
var _ InstructionValue = (*Make)(nil)

// instruction
// ----------- Field
// use-chain: interface(user) -> *field(self) -> multiple update(value) -> value
type Field struct {
	anInstruction
	anNode

	// field
	Key Value
	Obj User

	// Method or Field
	IsMethod bool

	// capture by other function
	OutCapture bool

	Update []Value // value

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
	Address User
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
	anNode

	Value Value
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
	anNode
	Iter Value
}

var _ Node = (*Next)(nil)
var _ User = (*Next)(nil)
var _ Value = (*Next)(nil)
var _ Instruction = (*Next)(nil)
var _ InstructionValue = (*Next)(nil)

// ------------- ErrorHandler
type ErrorHandler struct {
	anInstruction
	try, catch, final, done *BasicBlock
}

var _ Instruction = (*ErrorHandler)(nil)

// -------------- PANIC
type Panic struct {
	anInstruction
	anNode
	Info Value
}

var _ Node = (*Panic)(nil)
var _ User = (*Panic)(nil)
var _ Instruction = (*Panic)(nil)

// --------------- RECOVER
type Recover struct {
	anInstruction
	anNode
}

var _ Node = (*Recover)(nil)
var _ Value = (*Recover)(nil)
var _ Instruction = (*Recover)(nil)
