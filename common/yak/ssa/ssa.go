package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils/omap"
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
	GetVerboseName() string
	SetVerboseName(string)

	GetId() int // for identify
	SetId(int)

	// position
	GetRange() *Range
	SetRange(*Range)

	// Scope
	// SetScope(*Scope)
	// GetScope() *Scope

	// extern
	IsExtern() bool
	SetExtern(bool)

	GetVariable(string) *Variable
	GetLastVariable() *Variable
	GetAllVariables() map[string]*Variable
	AddVariable(*Variable)
	ReplaceValue(Value, Value)
	SelfDelete()
}

type (
	Users  []User
	Values []Value
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
	IsUndefined() bool
}
type Typed interface {
	// Node
	// type
	GetType() Type
	SetType(Type)
}

type MemberCall interface {
	// object  member caller
	IsObject() bool
	AddMember(Value, Value)
	GetMember(Value) (Value, bool)
	GetIndexMember(int) (Value, bool)
	GetStringMember(string) (Value, bool)
	DeleteMember( /*key*/ Value)   // delete by key
	GetAllMember() map[Value]Value // map[key]value

	// ReplaceMember( /* key */ Value /* value */, Value) // replace old-value with new-value

	// member, member callee
	IsMember() bool
	SetObject(Value)
	SetKey(Value)
	GetKey() Value
	GetObject() Value

	// ReplaceObject(Value) // replace old-object to new-object
}

// basic handle item (interface)
type Value interface {
	Node
	Instruction
	MemberCall
	Typed
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
	Node
	Instruction
	ReplaceValue(Value, Value)
}

// both instruction and value
type Program struct {
	// package list
	Packages map[string]*Package

	ConstInstruction   *omap.OrderedMap[int, *ConstInst]
	NameToInstructions *omap.OrderedMap[string, []Instruction]
	IdToInstructionMap *omap.OrderedMap[int, Instruction]

	errors SSAErrors

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
	anValue

	// package, double link
	Package *Package

	// just function parameter and all return instruction
	Param    []*Parameter
	paramMap map[Value]int // for get parameter index
	Return   []*Return

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
	SideEffects []*FunctionSideEffect
	// closure function double link. parentFunc <-> childFuncs
	parent     *Function   // parent function;  can be nil if there is no parent function
	ChildFuncs []*Function // child function within this function

	// extern lib
	cacheExternInstance map[string]Value // lib and value
	externType          map[string]Type

	// include / require / eval-code / import packet
	referenceFiles *omap.OrderedMap[string, string]

	// ssa error
	errComment ErrorComment

	// ================  for build
	// builder
	builder *FunctionBuilder
	// this function is variadic parameter, for function type create
	hasEllipsis bool
}

func (f *Function) PushReferenceFile(file, code string) {
	f.referenceFiles.Set(file, code)
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
	symbolTable map[string]Values
	ScopeTable  *Scope
	finish      bool // if emitJump finish!
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
	anValue

	table   map[string]any
	builder *FunctionBuilder

	MemberMap map[string]Value
	Member    []Value
}

var (
	_ Node  = (*ExternLib)(nil)
	_ Value = (*ExternLib)(nil)
	_ User  = (*ExternLib)(nil)
)

// ----------- Parameter
type Parameter struct {
	anValue

	IsFreeValue bool

	defaultValue         Value
	FormalParameterIndex int

	// if this flag set, this parameter will be set to member call,
	// if pass a as parameter, it will be set to `a.Key` is parameter
	IsMemberCall          bool
	MemberCallObjectIndex int
	MemberCallKey         Value
}

func (p *Parameter) GetDefault() Value {
	return p.defaultValue
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
	anValue
	Unary      int
	isIdentify bool // field key
	Origin     User
}

// ConstInst cont set Type
func (c *ConstInst) GetType() Type   { return c.anValue.GetType() }
func (c *ConstInst) SetType(ts Type) { c.anValue.SetType(ts) }

var (
	_ Node        = (*ConstInst)(nil)
	_ Value       = (*ConstInst)(nil)
	_ User        = (*ConstInst)(nil)
	_ Instruction = (*ConstInst)(nil)
)

// ----------- Undefined

// mark undefined value type
type UndefinedKind int

const (
	// normal undefined value
	UndefinedValue UndefinedKind = iota
	// member call but not this key
	UndefinedMemberInValid
	// member call, has this key, but not this value, this shouldn't mark error
	UndefinedMemberValid
)

type Undefined struct {
	anValue
	Kind UndefinedKind
}

func (u *Undefined) IsUndefined() bool { return true }

var (
	_ Node        = (*Undefined)(nil)
	_ Value       = (*Undefined)(nil)
	_ Instruction = (*Undefined)(nil)
)

// ----------- BinOp
type BinOp struct {
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
	anValue
	CallSite *Call // call instruction
	Value    Value // modify to this value
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

// ------------- Next
type Next struct {
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
