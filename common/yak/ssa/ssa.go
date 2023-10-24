package ssa

import (
	"sync"

	"github.com/samber/lo"
)

type Position struct {
	SourceCode  string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}
type ErrorLogger interface {
	NewError(ErrorKind, ErrorTag, string)
}

type LeftInstruction interface {
	Instruction
	// variable
	GetLeftVariables() []string
	AddLeftVariables(variable string)

	// must
	// left-value position
	// get all left position
	GetLeftPositions() []*Position
	// get last left position
	GetLeftPosition() *Position
	AddLeftPositions(*Position) // add left position
}

type Instruction interface {
	ErrorLogger

	// function
	GetFunc() *Function
	SetFunc(*Function)
	// block
	GetBlock() *BasicBlock
	SetBlock(*BasicBlock)

	GetVariable() string
	SetVariable(variable string)

	// position
	GetPosition() *Position
	SetPosition(*Position)

	// symbol-table
	SetSymbolTable(*blockSymbolTable)
	GetSymbolTable() *blockSymbolTable

	// has left-value
	HasLeftVariable() bool
	GetLeftItem() LeftInstruction
}
type Users []User
type Values []Value

// data-flow
type Node interface {
	// string
	String() string

	// for graph
	HasUsers() bool
	GetUsers() Users
	HasValues() bool
	GetValues() Values
}
type TypedNode interface {
	Node
	// type
	GetType() Type
	SetType(Type)
}

// basic handle item (interface)
type Value interface {
	LeftInstruction
	TypedNode
	AddUser(User)
	RemoveUser(User)

	SetType(Type)
	GetType() Type
}

type User interface {
	Instruction
	Node
	ReplaceValue(Value, Value)
}

type anInstruction struct {
	fun    *Function
	block  *BasicBlock
	Pos    *Position
	symbol *blockSymbolTable

	variable string

	// left
	hasLeft   bool
	variables []string
	LeftPos   []*Position
}

func NewInstruction() anInstruction {
	return anInstruction{}
}

// ssa function and block
func (a *anInstruction) SetFunc(f *Function)        { a.fun = f }
func (a *anInstruction) GetFunc() *Function         { return a.fun }
func (a *anInstruction) SetBlock(block *BasicBlock) { a.block = block }
func (a *anInstruction) GetBlock() *BasicBlock      { return a.block }

// source code position
func (c *anInstruction) GetPosition() *Position    { return c.Pos }
func (c *anInstruction) SetPosition(pos *Position) { c.Pos = pos }

// error logger
func (c *anInstruction) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	c.GetFunc().NewErrorWithPos(kind, tag, c.GetPosition(), msg)
}

// symbol-table
func (a *anInstruction) GetSymbolTable() *blockSymbolTable       { return a.symbol }
func (a *anInstruction) SetSymbolTable(symbol *blockSymbolTable) { a.symbol = symbol }

// variable
func (a *anInstruction) SetVariable(v string) { a.variable = v }
func (a *anInstruction) GetVariable() string  { return a.variable }

// has left-instruction
func (a *anInstruction) HasLeftVariable() bool        { return a.hasLeft }
func (a *anInstruction) GetLeftItem() LeftInstruction { return LeftInstruction(a) }

// left-instruction: variable
func (a *anInstruction) GetLeftVariables() []string { return a.variables }
func (a *anInstruction) AddLeftVariables(name string) {
	a.variables = append(a.variables, name)
}

// left-instruction: left-position
func (a *anInstruction) GetLeftPositions() []*Position { return a.LeftPos }
func (a *anInstruction) GetLeftPosition() *Position {
	if len(a.LeftPos) > 0 {
		return a.LeftPos[len(a.LeftPos)-1]
	} else {
		return nil
	}
}
func (a *anInstruction) AddLeftPositions(pos *Position) {
	a.LeftPos = append(a.LeftPos, pos)
}

var _ Instruction = (*anInstruction)(nil)
var _ LeftInstruction = (*anInstruction)(nil)

type anValue struct {
	typ  Type
	user map[User]struct{}
}

func NewValue() anValue {
	return anValue{
		typ:  BasicTypes[Any],
		user: make(map[User]struct{}),
	}
}

func (n *anValue) String() string { return "" }

// has/get user and value
func (n *anValue) HasUsers() bool  { return len(n.user) != 0 }
func (n *anValue) GetUsers() Users { return lo.Keys(n.user) }

// for Value
func (n *anValue) AddUser(u User)    { n.user[u] = struct{}{} }
func (n *anValue) RemoveUser(u User) { delete(n.user, u) }

// for Value : type
func (n *anValue) GetType() Type    { return n.typ }
func (n *anValue) SetType(typ Type) { n.typ = typ }

// both instruction and value
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
	anInstruction
	anValue

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
	parent       *Function // parent function if anonymous function; nil if global function.
	FreeValues   []Value   // the value, captured variable form parent-function,
	symbolObject *Make     // for function symbol table

	// for instruction
	InstReg     map[Instruction]string // instruction -> virtual register
	symbolTable map[string]map[*BasicBlock]Values

	// extern lib
	externInstance map[string]Value // lib and value
	externType     map[string]Type

	// ssa error
	err SSAErrors

	// for builder
	builder *FunctionBuilder
}

var _ Node = (*Function)(nil)
var _ Value = (*Function)(nil)

// implement Value
type BasicBlock struct {
	anInstruction
	anValue

	Index int
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

	// for build
	finish        bool // if emitJump finish!
	isSealed      bool
	inCompletePhi []*Phi // variable -> phi
	Skip          bool   // for phi build, avoid recursive
}

func (b *BasicBlock) GetType() Type {
	return nil
}

func (b *BasicBlock) SetType(ts Type) {
}

var _ Node = (*BasicBlock)(nil)
var _ Value = (*BasicBlock)(nil)

// =========================================  Value ===============================
// ================================= Spec Value

// ----------- Phi
type Phi struct {
	anInstruction
	anValue

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

// ----------- Parameter
type Parameter struct {
	anValue
	anInstruction

	// pos *Position
	variable    string
	Func        *Function
	IsFreeValue bool
	//TODO: is modify , not cover
	IsModify bool
	typs     Type
}

var _ Node = (*Parameter)(nil)
var _ Value = (*Parameter)(nil)

// ================================= Normal Value

// ----------- Const
// ConstInst also have block pointer, which block set this const to variable
type ConstInst struct {
	*Const
	anInstruction
	anValue
	Unary int
}

// ConstInst cont set Type
func (c *ConstInst) GetType() Type   { return c.Const.GetType() }
func (c *ConstInst) SetType(ts Type) {}

var _ Node = (*ConstInst)(nil)
var _ Value = (*ConstInst)(nil)
var _ Instruction = (*ConstInst)(nil)

// ----------- Undefined
type Undefined struct {
	anInstruction
	anValue
}

var _ Node = (*Undefined)(nil)
var _ Value = (*Undefined)(nil)
var _ Instruction = (*Undefined)(nil)

// ----------- BinOp
type BinOp struct {
	anInstruction
	anValue
	Op   BinaryOpcode
	X, Y Value
}

var _ Value = (*BinOp)(nil)
var _ User = (*BinOp)(nil)
var _ Node = (*BinOp)(nil)
var _ Instruction = (*BinOp)(nil)

type UnOp struct {
	anValue

	anInstruction

	Op UnaryOpcode
	X  Value
}

var _ Value = (*UnOp)(nil)
var _ User = (*UnOp)(nil)
var _ Node = (*UnOp)(nil)
var _ Instruction = (*UnOp)(nil)

// ================================= Function Call

// ----------- Call
// call instruction call method function  with args as argument
type Call struct {
	anInstruction
	// call is a value
	anValue

	// for call function
	Method  Value
	Args    []Value
	binding []Value

	// go function
	Async  bool
	Unpack bool

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

// ================================= Memory Value

// ----------- Make
type Make struct {
	anInstruction
	anValue

	// when slice
	low, high, step Value

	parentI Value // parent interface

	// when slice or map
	Len, Cap Value
}

var _ Node = (*Make)(nil)
var _ Value = (*Make)(nil)
var _ User = (*Make)(nil)
var _ Instruction = (*Make)(nil)

// instruction
// ----------- Field
type Field struct {
	anInstruction
	anValue

	// field
	Key Value
	Obj Value
	// this field is Obj[Key]

	update []User
	// all update for this Field, also contain this update in field.GetUser()

	// Method or Field
	IsMethod bool
	// capture by other function
	OutCapture bool
}

var _ Node = (*Field)(nil)
var _ Value = (*Field)(nil)
var _ User = (*Field)(nil)
var _ Instruction = (*Field)(nil)

// ----------- Update
type Update struct {
	anInstruction

	Value   Value
	Address Value // this point to field
}

var _ Node = (*Update)(nil)
var _ User = (*Update)(nil)
var _ Instruction = (*Update)(nil)

// ------------- Next
type Next struct {
	anInstruction
	anValue
	Iter   Value
	InNext bool // "in" grammar
}

var _ Node = (*Next)(nil)
var _ User = (*Next)(nil)
var _ Value = (*Next)(nil)
var _ Instruction = (*Next)(nil)

// ================================= Assert Value

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

// ----------- Type-cast
// cast value -> type
type TypeCast struct {
	anInstruction
	anValue

	Value Value
}

var _ Node = (*TypeCast)(nil)
var _ Value = (*TypeCast)(nil)
var _ User = (*TypeCast)(nil)
var _ Instruction = (*TypeCast)(nil)

// ------------- type value
type TypeValue struct {
	anInstruction
	anValue
}

var _ Node = (*TypeValue)(nil)
var _ Value = (*TypeValue)(nil)
var _ Instruction = (*TypeValue)(nil)

// ================================= Error Handler

// ------------- ErrorHandler
type ErrorHandler struct {
	anInstruction
	try, catch, final, done *BasicBlock
}

var _ Instruction = (*ErrorHandler)(nil)

// -------------- PANIC
type Panic struct {
	anInstruction
	anValue
	Info Value
}

var _ Node = (*Panic)(nil)
var _ User = (*Panic)(nil)
var _ Instruction = (*Panic)(nil)

// --------------- RECOVER
type Recover struct {
	anInstruction
	anValue
}

var _ Node = (*Recover)(nil)
var _ Value = (*Recover)(nil)
var _ Instruction = (*Recover)(nil)

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
	Key              Value
}

var _ Node = (*Loop)(nil)
var _ User = (*Loop)(nil)
var _ Instruction = (*Loop)(nil)

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
