package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/slices"
)

type ErrorLogger interface {
	NewError(ErrorKind, ErrorTag, string)
}

type Instruction interface {
	ErrorLogger

	GetOpcode() Opcode

	// function
	GetFunc() *Function
	SetFunc(*Function)
	// block
	GetBlock() *BasicBlock
	SetBlock(*BasicBlock)
	// program
	GetProgram() *Program

	GetName() string
	SetName(variable string)

	GetId() int // for identify
	SetId(int)

	// position
	GetRange() *Range
	SetRange(*Range)

	// Scope
	SetScope(*Scope)
	GetScope() *Scope

	// extern
	IsExtern() bool
	SetExtern(bool)

	GetVariable(string) *Variable
	GetAllVariables() map[string]*Variable
	AddVariable(*Variable)
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
	TypedNode
	Maskable
	AddUser(User)
	RemoveUser(User)
}

type Maskable interface {
	AddMask(Value)
	GetMask() []Value
	Masked() bool
}

type User interface {
	InstructionNode
	ReplaceValue(Value, Value)
}

type anInstruction struct {
	fun   *Function
	block *BasicBlock
	R     *Range
	scope *Scope

	name string
	id   int

	isExtern  bool
	variables map[string]*Variable

	// mask is a map, key is variable name, value is variable value
	// it record the variable is masked by closure function or some scope changed
	mask *omap.OrderedMap[string, Value]
}

func (i *anInstruction) AddMask(v Value) {
	i.mask.Add(v)
}

func (i *anInstruction) GetMask() []Value {
	return i.mask.Values()
}

func (i *anInstruction) Masked() bool {
	return i.mask.Len() != 0
}

func NewInstruction() anInstruction {
	return anInstruction{
		variables: make(map[string]*Variable),
		id:        -1,
		mask:      omap.NewEmptyOrderedMap[string, Value](),
	}
}

// ssa function and block
func (a *anInstruction) SetFunc(f *Function)        { a.fun = f }
func (a *anInstruction) GetFunc() *Function         { return a.fun }
func (a *anInstruction) GetProgram() *Program       { return a.fun.Package.Prog }
func (a *anInstruction) SetBlock(block *BasicBlock) { a.block = block }
func (a *anInstruction) GetBlock() *BasicBlock      { return a.block }

// source code position
func (c *anInstruction) GetRange() *Range { return c.R }

func (c *anInstruction) SetRange(pos *Range) {
	// if c.Pos == nil {
	c.R = pos
	// }
}

func (c *anInstruction) IsExtern() bool   { return c.isExtern }
func (c *anInstruction) SetExtern(b bool) { c.isExtern = b }

// error logger
func (c *anInstruction) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	c.GetFunc().NewErrorWithPos(kind, tag, c.GetRange(), msg)
}

// symbol-table
func (a *anInstruction) GetScope() *Scope  { return a.scope }
func (a *anInstruction) SetScope(s *Scope) { a.scope = s }

// variable
func (a *anInstruction) SetName(v string) { a.name = v }
func (a *anInstruction) GetName() string  { return a.name }

// id
func (a *anInstruction) SetId(id int) { a.id = id }
func (a *anInstruction) GetId() int   { return a.id }

func (a *anInstruction) LineDisasm() string { return "" }

// opcode
func (a *anInstruction) GetOpcode() Opcode      { return OpUnknown } // cover by instruction
func (a *anInstruction) GetOperands() Values    { return nil }       // cover by instruction
func (a *anInstruction) GetOperand(i int) Value { return a.GetOperands()[i] }
func (a *anInstruction) GetOperandNum() int     { return len(a.GetOperands()) }

func (a *anInstruction) GetVariable(name string) *Variable {
	if ret, ok := a.variables[name]; ok {
		return ret
	} else {
		return nil
	}
}

func (a *anInstruction) GetAllVariables() map[string]*Variable {
	return a.variables
}
func (a *anInstruction) AddVariable(v *Variable) { a.variables[v.Name] = v }

var (
	_ Instruction = (*anInstruction)(nil)
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

	ConstInstruction   *omap.OrderedMap[int, *ConstInst]
	NameToInstructions *omap.OrderedMap[string, []Instruction]
	IdToInstructionMap *omap.OrderedMap[int, Instruction]

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

	// package, double link
	Package *Package

	// just function parameter and all return instruction
	Param  []*Parameter
	Return []*Return

	// BasicBlock list
	Blocks []*BasicBlock
	// First and End block
	EnterBlock *BasicBlock
	ExitBlock  *BasicBlock
	// For Defer  semantic
	// this block will always execute when the function exits,
	// regardless of whether the function returns normally or exits due to a panic.
	DeferBlock *BasicBlock

	// for closure function
	FreeValues map[string]*Parameter // store the captured variable form parent-function, just contain name, and type is Parameter
	// closure function side effects
	// TODO: currently, this value is not being used, but it should be utilized in the future.
	SideEffects map[string]Value
	// closure function double link. parentFunc <-> childFuncs
	parent     *Function   // parent function;  can be nil if there is no parent function
	ChildFuncs []*Function // child function within this function

	// extern lib
	externInstance map[string]Value // lib and value
	externType     map[string]Type

	// ssa error
	err        SSAErrors
	errComment ErrorComment

	// ================  for build
	// builder
	builder *FunctionBuilder
	// this function is variadic parameter, for function type create
	hasEllipsis bool
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
	symbolTable   map[string]Values
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
	// branch *Instruction // loop or if :
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

// ----------- externLib
type ExternLib struct {
	anInstruction
	anValue

	BuildField func(string) Value

	Member []Value
}

var _ Node = (*ExternLib)(nil)
var _ Value = (*ExternLib)(nil)
var _ User = (*ExternLib)(nil)

// ----------- Parameter
type Parameter struct {
	anValue
	anInstruction

	IsFreeValue bool

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
	Origin     User
}

// ConstInst cont set Type
func (c *ConstInst) GetType() Type   { return c.Const.GetType() }
func (c *ConstInst) SetType(ts Type) {}

var (
	_ Node        = (*ConstInst)(nil)
	_ Value       = (*ConstInst)(nil)
	_ User        = (*ConstInst)(nil)
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
