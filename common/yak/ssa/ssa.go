package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type ErrorLogger interface {
	NewError(ErrorKind, ErrorTag, string)
}

type LeftInstruction interface {
	// Instruction
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

	LineDisasm() string

	GetOpcode() Opcode

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

	// extern
	IsExtern() bool
	SetExtern(bool)

	// has left-value
	HasLeftVariable() bool
	// GetLeftItem() LeftInstruction
}
type (
	Users            []User
	Values           []Value
	InstructionNodes []InstructionNode
)

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
	// Node
	// type
	GetType() Type
	SetType(Type)
}

type InstructionNode interface {
	Node
	Instruction
}

// basic handle item (interface)
type Value interface {
	InstructionNode
	LeftInstruction
	TypedNode
	AddUser(User)
	RemoveUser(User)

	SetType(Type)
	GetType() Type
}

type User interface {
	InstructionNode
	ReplaceValue(Value, Value)
}

type anInstruction struct {
	fun    *Function
	block  *BasicBlock
	Pos    *Position
	symbol *blockSymbolTable

	variable string

	isExtern bool
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
func (c *anInstruction) GetPosition() *Position { return c.Pos }

func (c *anInstruction) SetPosition(pos *Position) {
	if c.Pos == nil {
		c.Pos = pos
	}
}

func (c *anInstruction) IsExtern() bool   { return c.isExtern }
func (c *anInstruction) SetExtern(b bool) { c.isExtern = b }

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

func (a *anInstruction) LineDisasm() string { return "" }

// opcode
func (a *anInstruction) GetOpcode() Opcode      { return OpUnknown } // cover by instruction
func (a *anInstruction) GetOperands() Values    { return nil }       // cover by instruction
func (a *anInstruction) GetOperand(i int) Value { return a.GetOperands()[i] }
func (a *anInstruction) GetOperandNum() int     { return len(a.GetOperands()) }

var (
	_ Instruction     = (*anInstruction)(nil)
	_ LeftInstruction = (*anInstruction)(nil)
)

type anValue struct {
	typ      Type
	userList Users
}

func NewValue() anValue {
	return anValue{
		typ:      BasicTypes[Any],
		userList: make(Users, 0),
	}
}

func (n *anValue) String() string { return "" }

// has/get user and value
func (n *anValue) HasUsers() bool  { return len(n.userList) != 0 }
func (n *anValue) GetUsers() Users { return n.userList }

// for Value
func (n *anValue) AddUser(u User) {
	if index := slices.Index(n.userList, u); index == -1 {
		n.userList = append(n.userList, u)
	}
}

func (n *anValue) RemoveUser(u User) {
	n.userList = utils.RemoveSliceItem(n.userList, u)
}

// for Value : type
func (n *anValue) GetType() Type    { return n.typ }
func (n *anValue) SetType(typ Type) { n.typ = typ }

// both instruction and value
type Program struct {
	// package list
	Packages map[string]*Package

	// for build
	buildOnce sync.Once
}

type Package struct {
	Name string
	// point to program
	Prog *Program
	// function list
	Funcs map[string]*Function
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
	DeferBlock *BasicBlock

	// anonymous function in this function
	AnonFuncs []*Function

	// if this function is anonFunc
	FreeValues   []Value          // the value, captured variable form parent-function,
	SideEffects  map[string]Value // closure function side effects
	parent       *Function        // parent function if anonymous function; nil if global function.
	symbolObject *Make            // for function symbol table

	// for instruction
	InstReg     map[Instruction]string // instruction -> virtual register
	symbolTable map[string]map[*BasicBlock]Values

	// extern lib
	externInstance map[string]Value // lib and value
	externType     map[string]Type

	// ssa error
	err        SSAErrors
	errComment ErrorComment

	// for builder
	builder *FunctionBuilder
}

func (f *Function) FirstBlockInstruction() []Instruction {
	if len(f.Blocks) > 0 {
		return f.Blocks[0].Insts
	}
	return nil
}

var (
	_ Node  = (*Function)(nil)
	_ Value = (*Function)(nil)
)

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

var (
	_ Node  = (*BasicBlock)(nil)
	_ Value = (*BasicBlock)(nil)
)

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

var (
	_ Node        = (*Phi)(nil)
	_ Value       = (*Phi)(nil)
	_ User        = (*Phi)(nil)
	_ Instruction = (*Phi)(nil)
)

// ----------- Parameter
type Parameter struct {
	anValue
	anInstruction

	// pos *Position
	variable    string
	IsFreeValue bool

	// for extern lib
	BuildField func(string) Value

	// TODO: is modify , not cover
	IsModify     bool
	defaultValue Value
}

func (p *Parameter) SetDefault(v Value) {
	p.defaultValue = v
}

var (
	_ Node  = (*Parameter)(nil)
	_ Value = (*Parameter)(nil)
)

// ================================= Normal Value

// ----------- Const
// ConstInst also have block pointer, which block set this const to variable
type ConstInst struct {
	*Const
	anInstruction
	anValue
	Unary      int
	isIdentify bool // field key
}

// ConstInst cont set Type
func (c *ConstInst) GetType() Type   { return c.Const.GetType() }
func (c *ConstInst) SetType(ts Type) {}

var (
	_ Node        = (*ConstInst)(nil)
	_ Value       = (*ConstInst)(nil)
	_ Instruction = (*ConstInst)(nil)
)

// ----------- Undefined
type Undefined struct {
	anInstruction
	anValue
}

var (
	_ Node        = (*Undefined)(nil)
	_ Value       = (*Undefined)(nil)
	_ Instruction = (*Undefined)(nil)
)

// ----------- BinOp
type BinOp struct {
	anInstruction
	anValue
	Op   BinaryOpcode
	X, Y Value
}

var (
	_ Value       = (*BinOp)(nil)
	_ User        = (*BinOp)(nil)
	_ Node        = (*BinOp)(nil)
	_ Instruction = (*BinOp)(nil)
)

type UnOp struct {
	anValue

	anInstruction

	Op UnaryOpcode
	X  Value
}

var (
	_ Value       = (*UnOp)(nil)
	_ User        = (*UnOp)(nil)
	_ Node        = (*UnOp)(nil)
	_ Instruction = (*UnOp)(nil)
)

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

var (
	_ Node        = (*Call)(nil)
	_ Value       = (*Call)(nil)
	_ User        = (*Call)(nil)
	_ Instruction = (*Call)(nil)
)

// ----------- SideEffect
type SideEffect struct {
	anInstruction
	anValue
	target Value // call instruction
}

var (
	_ Node        = (*SideEffect)(nil)
	_ Value       = (*SideEffect)(nil)
	_ User        = (*SideEffect)(nil)
	_ Instruction = (*SideEffect)(nil)
)

// ----------- Return
// The Return instruction returns values and control back to the calling
// function.
type Return struct {
	anInstruction
	anValue
	Results []Value
}

var (
	_ Node        = (*Return)(nil)
	_ User        = (*Return)(nil)
	_ Value       = (*Return)(nil)
	_ Instruction = (*Return)(nil)
)

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

var (
	_ Node        = (*Make)(nil)
	_ Value       = (*Make)(nil)
	_ User        = (*Make)(nil)
	_ Instruction = (*Make)(nil)
)

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

var (
	_ Node        = (*Field)(nil)
	_ Value       = (*Field)(nil)
	_ User        = (*Field)(nil)
	_ Instruction = (*Field)(nil)
)

// ----------- Update
type Update struct {
	anInstruction

	Value   Value
	Address Value // this point to field
}

var (
	_ Node        = (*Update)(nil)
	_ User        = (*Update)(nil)
	_ Instruction = (*Update)(nil)
)

// ------------- Next
type Next struct {
	anInstruction
	anValue
	Iter   Value
	InNext bool // "in" grammar
}

var (
	_ Node        = (*Next)(nil)
	_ User        = (*Next)(nil)
	_ Value       = (*Next)(nil)
	_ Instruction = (*Next)(nil)
)

// ================================= Assert Value

// ----------- assert
type Assert struct {
	anInstruction

	Cond     Value
	Msg      string
	MsgValue Value
}

var (
	_ Node        = (*Assert)(nil)
	_ User        = (*Assert)(nil)
	_ Instruction = (*Assert)(nil)
)

// ----------- Type-cast
// cast value -> type
type TypeCast struct {
	anInstruction
	anValue

	Value Value
}

var (
	_ Node        = (*TypeCast)(nil)
	_ Value       = (*TypeCast)(nil)
	_ User        = (*TypeCast)(nil)
	_ Instruction = (*TypeCast)(nil)
)

// ------------- type value
type TypeValue struct {
	anInstruction
	anValue
}

var (
	_ Node        = (*TypeValue)(nil)
	_ Value       = (*TypeValue)(nil)
	_ Instruction = (*TypeValue)(nil)
)

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

var (
	_ Node        = (*Panic)(nil)
	_ User        = (*Panic)(nil)
	_ Instruction = (*Panic)(nil)
)

// --------------- RECOVER
type Recover struct {
	anInstruction
	anValue
}

var (
	_ Node        = (*Recover)(nil)
	_ Value       = (*Recover)(nil)
	_ Instruction = (*Recover)(nil)
)

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

var (
	_ Node        = (*If)(nil)
	_ User        = (*If)(nil)
	_ Instruction = (*If)(nil)
)

// ----------- For
// for loop
type Loop struct {
	anInstruction

	Body, Exit *BasicBlock

	Init, Cond, Step Value
	Key              Value
}

var (
	_ Node        = (*Loop)(nil)
	_ User        = (*Loop)(nil)
	_ Instruction = (*Loop)(nil)
)

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

var (
	_ Node        = (*Switch)(nil)
	_ User        = (*Switch)(nil)
	_ Instruction = (*Switch)(nil)
)
