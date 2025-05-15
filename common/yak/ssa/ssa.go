package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

type ErrorLogger interface {
	NewError(ErrorKind, ErrorTag, string)
}

type GetIdIF interface {
	GetId() int64
}

type Instruction interface {
	ErrorLogger

	// string
	String() string

	GetOpcode() Opcode

	// function
	GetFunc() *Function
	SetFunc(*Function)
	// block
	GetBlock() *BasicBlock
	SetBlock(*BasicBlock)
	// program
	GetProgram() *Program
	GetProgramName() string
	SetProgram(*Program)

	GetName() string
	SetName(variable string)
	GetVerboseName() string
	GetShortVerboseName() string
	SetVerboseName(string)
	SetIsAnnotation(bool)
	IsAnnotation() bool

	GetIdIF
	SetId(int64)

	// position
	GetRange() memedit.RangeIf
	SetRange(memedit.RangeIf)
	GetSourceCode() string
	GetSourceCodeContext(n int) string

	// extern
	IsExtern() bool
	SetExtern(bool)

	SelfDelete()
	IsBlock(string) bool

	IsCFGEnterBlock() ([]Instruction, bool)

	// Self means lazy instruction will check and return
	// real instruction will return itself
	Self() Instruction

	// IsLazy means this instruction is lazy, not loaded all from db
	IsLazy() bool
	// IsFromDB means this instruction is loaded from db
	IsFromDB() bool
	SetIsFromDB(bool)
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
	IsParameter() bool
	IsSideEffect() bool
	IsPhi() bool
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
	SetStringMember(string, Value)
	DeleteMember( /*key*/ Value)   // delete by key
	GetAllMember() map[Value]Value // map[key]value
	ForEachMember(func(k Value, v Value) bool)

	// ReplaceMember( /* key */ Value /* value */, Value) // replace old-value with new-value

	// member, member callee
	IsMember() bool
	SetObject(Value)
	SetKey(Value)
	GetKey() Value
	GetObject() Value

	// ReplaceObject(Value) // replace old-object to new-object
}

type AssignAble interface {
	GetVariable(string) *Variable
	GetLastVariable() *Variable
	GetAllVariables() map[string]*Variable
	AddVariable(*Variable)
}

// basic handle item (interface)
type Value interface {
	Node
	Instruction
	MemberCall
	Typed
	Maskable
	AssignAble
	PointerIF
	Occultation
	AddUser(User)
	RemoveUser(User)
}

type Occultation interface {
	AddOccultation(Value)
	GetOccultation() []Value
	FlatOccultation() []Value
}

type PointerIF interface {
	// the value is pointed by this value
	GetReference() Value
	SetReference(Value)

	// the value that point to this value
	AddPointer(Value)
	GetPointer() Values
}

func Point(pointer Value, reference Value) {
	pointer.SetReference(reference)
	reference.AddPointer(pointer)
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

type Build func(string, *memedit.MemEditor, *FunctionBuilder) error

const (
	Application = ssadb.Application
	Library     = ssadb.Library
)

type FunctionName string

const (
	MainFunctionName    FunctionName = "@main"
	InitFunctionName    FunctionName = "@init"
	VirtualFunctionName FunctionName = "@virtual"
)

// both instruction and value
type Program struct {
	// package list
	Name            string
	Version         string
	ProgramKind     ssadb.ProgramKind // is library or application
	Language        string
	magicMethodName []string
	// from pom.xml file
	SCAPackages []*dxtypes.Package

	// filename and data,
	// if database exist, this is filename and hash, can use this hash to fetch source code
	// if no database, this is filename and file content
	ExtraFile map[string]string

	Application *Program // current Application
	// program relationship
	DownStream map[string]*Program
	// UpStream   map[string]*Program
	UpStream *omap.OrderedMap[string, *Program]

	EnableDatabase bool             // for compile, whether use database
	irProgram      *ssadb.IrProgram // from database program

	// TODO: this four map should need????!
	editorStack           *omap.OrderedMap[string, *memedit.MemEditor]
	FileList              map[string]string   // file-name and file hash
	LibraryFile           map[string][]string //library and file relation
	editorMap             *omap.OrderedMap[string, *memedit.MemEditor]
	CurrentIncludingStack *utils.Stack[string]

	Cache *Cache

	/*
		when build : ref: common/yak/ssa/lazy_builder.go
			*preHandler: set map hash -> false
			*build : 	 delete map hash
			when len(astMap) == 0  this program finish
	*/
	astMap   map[string]struct{} // ast hash list
	finished bool

	//consts
	Consts map[string]Value
	// function list
	Funcs          *omap.OrderedMap[string, *Function]
	Blueprint      *omap.OrderedMap[string, *Blueprint]
	BlueprintStack *utils.Stack[*Blueprint]
	ExportValue    map[string]Value
	ExportType     map[string]Type

	//store import

	// if importCoverInner is true, it will cover the inner import declare
	// when multiple import declare with the same name value/type, the last one will be used
	importCoverInner bool
	// if importCoverOuter is true, it will cover current program value/type declare
	// will use import value/type first, then use current program value/type
	importCoverCurrent bool
	// import declare
	importDeclares *omap.OrderedMap[string, *importDeclareItem]

	// offset
	OffsetMap         map[int]*OffsetItem
	OffsetSortedSlice []int

	// package Loader
	Loader      *ssautil.PackageFileLoader
	Build       Build
	_preHandler bool

	errors SSAErrors

	// process
	ProcessInfof func(string, ...any)

	// extern lib
	GlobalScope             Value            //全局作用域
	cacheExternInstance     map[string]Value // lib and value
	externType              map[string]Type
	externBuildValueHandler map[string]func(b *FunctionBuilder, id string, v any) (value Value)
	ExternInstance          map[string]any
	ExternLib               map[string]map[string]any

	PkgName           string
	fixImportCallback []func()

	// Project Config
	ProjectConfig map[string]*ProjectConfig

	// Template Language
	Template map[string]tl.TemplateGeneratedInfo

	config *LanguageConfig
}

// implement Value
type Function struct {
	anValue
	lazyBuilder

	isMethod   bool
	methodName string

	// Type
	Type *FunctionType

	// just function parameter
	Params      []Value
	ParamLength int
	// for closure function
	FreeValues map[*Variable]Value // store the captured variable form parent-function, just contain name, and type is Parameter
	// parameter member call
	// ParameterMembers []*ParameterMember
	ParameterMembers []Value
	// function side effects
	SideEffects       []*FunctionSideEffect
	SideEffectsReturn []map[*Variable]*FunctionSideEffect

	// throws clause
	Throws []Value

	// closure function double link. parentFunc <-> childFuncs
	parent     Value   // parent function;  can be nil if there is no parent function
	ChildFuncs []Value // child function within this function

	Return []Value

	// BasicBlock list
	Blocks []Instruction
	// First and End block
	EnterBlock Instruction
	ExitBlock  Instruction
	// For Defer  semantic
	// this block will always execute when the function exits,
	// regardless of whether the function returns normally or exits due to a panic.
	DeferBlock Instruction

	// ssa error
	errComment ErrorComment

	// ================  for build
	// scope id
	scopeId int
	// builder
	builder *FunctionBuilder
	// this function is variadic parameter, for function type create
	hasEllipsis bool

	// generic
	isGeneric bool
	// runtime function return type
	currentReturnType Type

	//if blueprint method,we need record.
	currentBlueprint *Blueprint
}

func (f *Function) SetCurrentReturnType(t Type) {
	f.currentReturnType = t
}
func (f *Function) GetCurrentReturnType() Type {
	return f.currentReturnType
}

func (f *Function) SetMethodName(name string) {
	f.isMethod = true
	f.methodName = name
}

func (f *Function) GetMethodName() string {
	return f.methodName
}

func (f *Function) FirstBlockInstruction() []Instruction {
	if len(f.Blocks) > 0 {
		firstBlock := f.Blocks[0]
		if block, ok := ToBasicBlock(firstBlock); ok {
			return block.Insts
		} else {
			log.Warnf("function %s first block is not a basic block", f.GetName())
		}
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
	Preds, Succs []Value

	// for CFG
	Parent Value   // parent block
	Child  []Value // child block

	/*
		if Condition == true: this block reach
	*/
	setReachable bool
	canBeReached int
	Condition    Value

	// instruction list
	Insts []Instruction
	Phis  []Value

	// error catch
	Handler *ErrorHandler

	// for build
	ScopeTable ScopeIF
	finish     bool // if emitJump finish!
}

func (b *BasicBlock) SetReachable(boolean bool) {
	b.setReachable = true
	if boolean {
		b.canBeReached = 1
	} else {
		b.canBeReached = -1
	}
}

func (b *BasicBlock) IsCFGEnterBlock() ([]Instruction, bool) {
	if len(b.Insts) <= 0 {
		return nil, false
	}
	jmp, err := lo.Last(b.Insts)
	if err != nil {
		return nil, false
	}

	_, ok := jmp.(*LazyInstruction)
	if ok {
		jmp = jmp.(*LazyInstruction).Self()
	}

	switch ret := jmp.(type) {
	case *Jump:
		if ret.To == nil {
			log.Warnf("Jump To is nil: %T", ret)
			return nil, false
		}

		toBlock, ok := ToBasicBlock(ret.To)
		if !ok {
			log.Warnf("Jump To is not *BasicBlock: %T", ret.To)
			return nil, false
		}

		last, err := lo.Last(toBlock.Insts)
		if err != nil {
			return nil, false
		}
		// fetch essential instructions via jump
		// if else(elif) condition
		// for loop condition
		// switch condition (label)
		if last.IsLazy() {
			last = last.Self()
		}
		switch ret := last.(type) {
		case *If:
			var ifs []*If
			ifs = append(ifs, ret)
			results := ret.GetSiblings()
			if len(results) > 0 {
				ifs = append(ifs, results...)
			}
			return lo.Map(ifs, func(a *If, i int) Instruction {
				return a
			}), true
		case *Switch:
			log.Warn("Swtich Statement (Condition/Label value should contains jmp) WARNING")
			return lo.Map(ret.Label, func(label SwitchLabel, i int) Instruction {
				var result Instruction = label.Value
				return result
			}), true
		case *Loop:
			log.Warn("Loop Statement (Condition/Label value should contains jmp) WARNING")
			return []Instruction{
				ret.Cond,
			}, true
		default:
			log.Warnf("unsupoorted CFG Entry Instruction: %T", ret)
		}
		return nil, false
	default:
		return nil, false
	}
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

	CFGEntryBasicBlock Value

	Edge []Value // edge[i] from phi.Block.Preds[i]
	//	what instruction create this control-flow merge?
	// branch *Instruction // loop or if :
}

func (p *Phi) IsPhi() bool {
	return true
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

type ParameterMemberCallKind int

const (
	NoMemberCall ParameterMemberCallKind = iota
	ParameterMemberCall
	FreeValueMemberCall
	CallMemberCall
	SideEffectMemberCall

	ParameterCall

	MoreParameterMember
)

type parameterMemberInner struct {
	ObjectName            string
	MemberCallKind        ParameterMemberCallKind
	MemberCallObjectIndex int    // for Parameter
	MemberCallObjectName  string // for FreeValue
	MemberCallKey         Value
}

func newParameterMember(obj *Parameter, key Value) *parameterMemberInner {
	new := &parameterMemberInner{
		ObjectName:    obj.GetName(),
		MemberCallKey: key,
	}

	if obj.IsFreeValue {
		new.MemberCallKind = FreeValueMemberCall
		new.MemberCallObjectName = obj.GetName()
	} else {
		new.MemberCallKind = ParameterMemberCall
		new.MemberCallObjectIndex = obj.FormalParameterIndex
	}
	return new
}

func newMoreParameterMember(member *ParameterMember, key Value) *parameterMemberInner {
	p := &parameterMemberInner{
		MemberCallKey:         key,
		MemberCallKind:        MoreParameterMember,
		MemberCallObjectName:  member.GetName(),
		MemberCallObjectIndex: member.FormalParameterIndex,
	}
	return p
}

func (p *parameterMemberInner) Get(c *Call) (obj Value, ok bool) {
	switch p.MemberCallKind {
	case NoMemberCall:
		return
	case ParameterMemberCall:
		if p.MemberCallObjectIndex >= len(c.Args) {
			return
		}
		return c.Args[p.MemberCallObjectIndex], true
	case MoreParameterMember:
		/*todo:
		enable closure and readValue have error.
		beacuse create parameter in head scope.the seem error.
		need more test to case
		*/
		if p.MemberCallObjectIndex >= len(c.ArgMember) {
			return nil, false
		}
		return c.ArgMember[p.MemberCallObjectIndex], true
	case FreeValueMemberCall:
		obj, ok = c.Binding[p.MemberCallObjectName]
		return obj, ok
	case CallMemberCall:
		return c, true
	case SideEffectMemberCall:
		value, ok := c.SideEffectValue[p.MemberCallObjectName]
		return value, ok
	case ParameterCall:
		if p.MemberCallObjectIndex >= len(c.Args) {
			return
		}
		return c.Args[p.MemberCallObjectIndex], true
	}
	return
}

type ParameterMember struct {
	anValue

	FormalParameterIndex int

	*parameterMemberInner
}

var (
	_ Node  = (*Parameter)(nil)
	_ Value = (*Parameter)(nil)
)

// ----------- Parameter
type Parameter struct {
	anValue

	// for FreeValue
	IsFreeValue  bool
	defaultValue Value

	// Parameter Index
	FormalParameterIndex int
}

func (p *Parameter) ReplaceValue(v Value, to Value) {
	if p.defaultValue == v {
		p.defaultValue = to
	}
}
func (p *Parameter) GetDefault() Value {
	return p.defaultValue
}

func (p *Parameter) SetDefault(v Value) {
	if p == nil {
		return
	}
	p.defaultValue = v
	//增加一个ud关系绑定
	v.AddPointer(p)
	v.AddUser(p)
}

func (p *Parameter) IsParameter() bool {
	return true
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

// UndefinedKind : mark undefined value type
type UndefinedKind int

const (
	// UndefinedValueInValid normal undefined value
	UndefinedValueInValid UndefinedKind = iota
	// UndefinedValueValid is variable only declare
	UndefinedValueValid
	// UndefinedMemberInValid member call but not this key
	UndefinedMemberInValid
	// UndefinedMemberValid member call, has this key, but not this value, this shouldn't mark error
	UndefinedMemberValid

	// Return
	UndefinedValueReturn
)

type Undefined struct {
	anValue
	Kind UndefinedKind
}

func (u *Undefined) IsUndefined() bool { return true }

// func (u *Undefined) ReplaceValue(v Value, to Value) {
// 	builder := u.GetFunc().builder
// 	if v.GetId() == -1 { // 用于处理spin中的empty phi
// 		index := u.GetKey()
// 		for _, user := range u.GetUsers() {
// 			value := builder.ReadMemberCallValue(to, index)
// 			user.ReplaceValue(u, value)
// 			to.AddUser(user)
// 		}
// 	}
// }

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
	Method    Value
	Args      []Value
	Binding   map[string]Value
	ArgMember []Value

	// go function
	Async  bool
	Unpack bool

	// caller
	// caller Value
	// ~ drop error
	IsDropError     bool
	IsEllipsis      bool
	SideEffectValue map[string]Value
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
	CallSite Value // call instruction
	Value    Value // modify to this value
}

func (p *SideEffect) IsSideEffect() bool {
	return true
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

type ErrorCatch struct {
	anValue
	CatchBody Value
	Exception Value
}

var _ Instruction = (*ErrorCatch)(nil)
var _ User = (*ErrorCatch)(nil)
var _ Value = (*ErrorCatch)(nil)

// ------------- ErrorHandler
type ErrorHandler struct {
	anInstruction
	Try, Final, Done Value
	// catch and exception align
	Catch []Value // error catch
}

var _ Instruction = (*ErrorHandler)(nil)
var _ User = (*ErrorHandler)(nil)
var _ Node = (*ErrorHandler)(nil)

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
	To Value
}

var _ Instruction = (*Jump)(nil)
var _ User = (*Jump)(nil)
var _ Node = (*Loop)(nil)

// ----------- IF
// The If instruction transfers control to one of the two successors
// of its owning block, depending on the boolean Cond: the first if
// true, the second if false.
type If struct {
	anInstruction

	Cond  Value
	True  Value
	False Value
}

func (i *If) GetSiblings() []*If {
	return i.getSiblings(nil)
}

func (i *If) getSiblings(m map[int64]struct{}) []*If {
	if m == nil {
		m = make(map[int64]struct{})
	}
	_, visited := m[i.GetId()]
	if visited {
		return nil
	}

	var ifs []*If
	if i.False == nil {
		return nil
	}

	falseBlock, ok := ToBasicBlock(i.False)
	if !ok || len(falseBlock.Insts) == 0 {
		return nil
	}
	raw := falseBlock.LastInst()
	lastIf, ok := ToIfInstruction(raw)
	if ok {
		m[lastIf.GetId()] = struct{}{}
		ifs = append(ifs, lastIf)
		ifs = append(ifs, lastIf.getSiblings(m)...)
	}
	return ifs
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

	Body, Exit Value

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
	Dest  Value
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
