package ssa

import (
	"github.com/samber/lo"
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
	GetRange() *memedit.Range
	SetRange(*memedit.Range)
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

	// use program cache
	GetInstructionById(id int64) (Instruction, bool)
	GetValueById(id int64) (Value, bool)
	GetUsersByID(id int64) (User, bool)
	GetValuesByIDs([]int64) Values
	GetUsersByIDs([]int64) Users

	// string
	String() string
	RefreshString() // for refrensh string/name/short-name in *anInstruction

	getAnInstruction() *anInstruction
}

type (
	Users  []User
	Values []Value
)

// data-flow
type Node interface {

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

	getAnValue() *anValue
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

type Build func(FrontAST, *memedit.MemEditor, *FunctionBuilder) error

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

	DatabaseKind ProgramCacheKind // for compile, whether use database
	irProgram    *ssadb.IrProgram // from database program

	// TODO: this four map should need????!
	editorStack *omap.OrderedMap[string, *memedit.MemEditor]
	FileList    map[string]string // file-name and file hash
	LineCount   int

	LibraryFile           map[string][]string //library and file relation
	editorMap             *omap.OrderedMap[string, *memedit.MemEditor]
	CurrentIncludingStack *utils.Stack[string]

	Cache *ProgramCache

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
	*anValue
	lazyBuilder

	isMethod   bool
	methodName string

	// Type
	Type *FunctionType

	// just function parameter
	Params      []int64 // parameter
	ParamLength int
	// for closure function
	FreeValues map[*Variable]int64 // parameter-freevalue  // store the captured variable form parent-function, just contain name, and type is Parameter
	// parameter member call
	// ParameterMembers []*ParameterMember
	ParameterMembers []int64 // parameter member
	// function side effects
	SideEffects       []*FunctionSideEffect
	SideEffectsReturn []map[*Variable]*FunctionSideEffect

	// throws clause
	Throws []int64

	// closure function double link. parentFunc <-> childFuncs
	parent     int64   // function     // parent function;  can be nil if there is no parent function
	ChildFuncs []int64 // function  // child function within this function

	Return []int64 // return

	// BasicBlock list
	Blocks []int64
	// First and End block
	EnterBlock int64
	ExitBlock  int64
	// For Defer  semantic
	// this block will always execute when the function exits,
	// regardless of whether the function returns normally or exits due to a panic.
	DeferBlock int64

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
		firstBlockId := f.Blocks[0]
		firstBlockValue, ok := f.GetValueById(firstBlockId)
		if !ok || firstBlockValue == nil {
			return nil
		}
		if block, ok := ToBasicBlock(firstBlockValue); ok {
			return f.GetInstructionsByIDs(block.Insts)
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

type BasicBlockReachableKind int

const (
	BasicBlockUnknown     BasicBlockReachableKind = 0
	BasicBlockReachable                           = 1
	BasicBlockUnReachable                         = -1
)

// implement Value
type BasicBlock struct {
	*anValue `json:"-"`

	Index int
	// BasicBlock graph
	Preds, Succs []int64 // basic block

	// for CFG
	Parent int64   // parent block
	Child  []int64 // child block

	/*
		if Condition == true: this block reach
	*/
	canBeReached BasicBlockReachableKind
	Condition    int64 // value

	// instruction list
	Insts []int64 // instruction
	Phis  []int64 // phi

	// error catch
	Handler int64

	// for build
	ScopeTable ScopeIF
	finish     bool // if emitJump finish!
}

func (b *BasicBlock) SetReachable(boolean bool) {
	if boolean {
		b.canBeReached = BasicBlockReachable
	} else {
		b.canBeReached = BasicBlockUnReachable
	}
}

func (b *BasicBlock) IsCFGEnterBlock() ([]Instruction, bool) {
	if len(b.Insts) <= 0 {
		return nil, false
	}
	jmpId, err := lo.Last(b.Insts)
	if jmpId <= 0 || err != nil {
		return nil, false
	}
	jmp, ok := b.GetInstructionById(jmpId)
	if !ok || jmp == nil {
		return nil, false
	}

	_, ok = jmp.(*LazyInstruction)
	if ok {
		jmp = jmp.(*LazyInstruction).Self()
	}

	switch ret := jmp.(type) {
	case *Jump:
		if ret.To <= 0 {
			log.Warnf("Jump To is nil: %T", ret)
			return nil, false
		}
		to, ok := b.GetInstructionById(ret.To)
		if !ok || to == nil {
			return nil, false
		}

		toBlock, ok := ToBasicBlock(to)
		if !ok {
			log.Warnf("Jump To is not *BasicBlock: %T", ret.To)
			return nil, false
		}

		lastId, err := lo.Last(toBlock.Insts)
		if lastId <= 0 || err != nil {
			return nil, false
		}
		last, ok := b.GetInstructionById(lastId)
		if !ok || last == nil {
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
			return lo.FilterMap(ret.Label, func(label SwitchLabel, i int) (Instruction, bool) {
				result, ok := b.GetValueById(label.Value)
				if !ok || result == nil {
					return nil, false
				}
				if inst, ok := result.(Instruction); ok {
					return inst, true
				}
				return nil, false
			}), true
		case *Loop:
			log.Warn("Loop Statement (Condition/Label value should contains jmp) WARNING")
			condValue, ok := b.GetValueById(ret.Cond)
			if !ok || condValue == nil {
				return nil, false
			}
			if condInst, ok := condValue.(Instruction); ok {
				return []Instruction{condInst}, true
			}
			return nil, false
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
	*anValue

	CFGEntryBasicBlock int64

	Edge []int64 // value  // edge[i] from phi.Block.Preds[i]
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
	*anValue

	table   map[string]any
	builder *FunctionBuilder

	MemberMap map[string]int64 // value
	Member    []int64          // value
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
	MemberCallKey         int64  // value
}

func newParameterMember(obj *Parameter, key Value) *parameterMemberInner {
	new := &parameterMemberInner{
		ObjectName:    obj.GetName(),
		MemberCallKey: key.GetId(),
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
		MemberCallKey:         key.GetId(), // Changed: Use key.GetId()
		MemberCallKind:        MoreParameterMember,
		MemberCallObjectName:  member.GetName(),
		MemberCallObjectIndex: member.FormalParameterIndex,
	}
	return p
}

func (p *parameterMemberInner) Get(c *Call) (obj Value, ok bool) {

	var id int64
	switch p.MemberCallKind {
	case NoMemberCall:
		return
	case ParameterMemberCall:
		if p.MemberCallObjectIndex >= len(c.Args) {
			return
		}
		ok = true
		id = c.Args[p.MemberCallObjectIndex]
		// return c.Args[p.MemberCallObjectIndex], true
	case MoreParameterMember:
		/*todo:
		enable closure and readValue have error.
		beacuse create parameter in head scope.the seem error.
		need more test to case
		*/
		if p.MemberCallObjectIndex >= len(c.ArgMember) {
			return nil, false
		}
		// Changed: Fetch Value using GetValueById
		obj, ok := c.GetValueById(c.ArgMember[p.MemberCallObjectIndex])
		return obj, ok
	case FreeValueMemberCall:
		id, ok = c.Binding[p.MemberCallObjectName]
	case CallMemberCall:
		return c, true
	case SideEffectMemberCall:
		id, ok = c.SideEffectValue[p.MemberCallObjectName]
	case ParameterCall:
		if p.MemberCallObjectIndex >= len(c.Args) {
			return
		}
		id, ok = c.Args[p.MemberCallObjectIndex], true
	}
	if id > 0 {
		obj, ok = c.GetValueById(id)
	}
	return
}

type ParameterMember struct {
	*anValue

	FormalParameterIndex int

	*parameterMemberInner
}

var (
	_ Node  = (*Parameter)(nil)
	_ Value = (*Parameter)(nil)
)

// ----------- Parameter
type Parameter struct {
	*anValue

	// for FreeValue
	IsFreeValue  bool
	defaultValue int64 // value

	// Parameter Index
	FormalParameterIndex int
}

func (p *Parameter) ReplaceValue(v Value, to Value) {
	if p.defaultValue == v.GetId() {
		p.defaultValue = to.GetId()
	}
}
func (p *Parameter) GetDefault() Value {
	val, _ := p.GetValueById(p.defaultValue)
	return val
}

func (p *Parameter) SetDefault(v Value) {
	if p == nil {
		return
	}
	p.defaultValue = v.GetId()
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
type ConstType string

const (
	ConstTypeNormal ConstType = "normal"

	// ConstTypePlaceholder stands for unValid const, like member call's key.
	// We don't consider it a normal constant, but just a placeholder
	ConstTypePlaceholder ConstType = "placeholder"
)

type ConstInst struct {
	*Const
	*anValue
	Unary      int
	isIdentify bool  // field key
	Origin     int64 // user
	ConstType  ConstType
}

// ConstInst cont set Type
func (c *ConstInst) IsNormalConst() bool {
	if c == nil {
		return false
	}
	return c.ConstType == ConstTypeNormal
}

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
	*anValue
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
	*anValue
	Op   BinaryOpcode
	X, Y int64
}

var (
	_ Value       = (*BinOp)(nil)
	_ User        = (*BinOp)(nil)
	_ Node        = (*BinOp)(nil)
	_ Instruction = (*BinOp)(nil)
)

type UnOp struct {
	*anValue

	Op UnaryOpcode
	X  int64
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
	*anValue

	// for call function
	Method    int64
	Args      []int64
	Binding   map[string]int64
	ArgMember []int64

	// go function
	Async  bool
	Unpack bool

	// caller
	// caller Value
	// ~ drop error
	IsDropError     bool
	IsEllipsis      bool
	SideEffectValue map[string]int64
}

var (
	_ Node        = (*Call)(nil)
	_ Value       = (*Call)(nil)
	_ User        = (*Call)(nil)
	_ Instruction = (*Call)(nil)
)

// ----------- SideEffect
type SideEffect struct {
	*anValue
	CallSite int64 // call instruction
	Value    int64 // modify to this value
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
	*anValue
	Results []int64
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
	*anValue

	// when slice
	low, high, step int64

	parentI int64 // parent interface

	// when slice or map
	Len, Cap int64
}

var (
	_ Node        = (*Make)(nil)
	_ Value       = (*Make)(nil)
	_ User        = (*Make)(nil)
	_ Instruction = (*Make)(nil)
)

// ------------- Next
type Next struct {
	*anValue
	Iter   int64
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
	*anInstruction

	Cond     int64
	Msg      string
	MsgValue int64
}

var (
	_ Node        = (*Assert)(nil)
	_ User        = (*Assert)(nil)
	_ Instruction = (*Assert)(nil)
)

// ----------- Type-cast
// cast value -> type
type TypeCast struct {
	*anValue

	Value int64
}

var (
	_ Node        = (*TypeCast)(nil)
	_ Value       = (*TypeCast)(nil)
	_ User        = (*TypeCast)(nil)
	_ Instruction = (*TypeCast)(nil)
)

// ------------- type value
type TypeValue struct {
	*anValue
}

var (
	_ Node        = (*TypeValue)(nil)
	_ Value       = (*TypeValue)(nil)
	_ Instruction = (*TypeValue)(nil)
)

// ================================= Error Handler

type ErrorCatch struct {
	*anValue
	CatchBody int64
	Exception int64
}

var _ Instruction = (*ErrorCatch)(nil)
var _ User = (*ErrorCatch)(nil)
var _ Value = (*ErrorCatch)(nil)

// ------------- ErrorHandler
type ErrorHandler struct {
	*anInstruction
	Try, Final, Done int64
	// catch and exception align
	Catch []int64 // error catch
}

var _ Instruction = (*ErrorHandler)(nil)
var _ User = (*ErrorHandler)(nil)
var _ Node = (*ErrorHandler)(nil)

// -------------- PANIC
type Panic struct {
	*anValue
	Info int64
}

var (
	_ Node        = (*Panic)(nil)
	_ User        = (*Panic)(nil)
	_ Instruction = (*Panic)(nil)
)

// --------------- RECOVER
type Recover struct {
	*anValue
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
	*anInstruction
	To int64 // value
}

var _ Instruction = (*Jump)(nil)
var _ User = (*Jump)(nil)
var _ Node = (*Loop)(nil)

// ----------- IF
// The If instruction transfers control to one of the two successors
// of its owning block, depending on the boolean Cond: the first if
// true, the second if false.
type If struct {
	*anInstruction

	Cond  int64
	True  int64
	False int64
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
	if i.False <= 0 {
		return nil
	}

	val, ok := i.GetValueById(i.False)
	if !ok {
		return nil
	}
	falseBlock, ok := ToBasicBlock(val)
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
	*anInstruction

	Body, Exit int64 // basic block

	Init, Cond, Step int64
	Key              int64
}

var (
	_ Node        = (*Loop)(nil)
	_ User        = (*Loop)(nil)
	_ Instruction = (*Loop)(nil)
)

// ----------- Switch
type SwitchLabel struct {
	Value int64
	Dest  int64
}

func NewSwitchLabel(v Value, dest *BasicBlock) SwitchLabel {
	return SwitchLabel{
		Value: v.GetId(),
		Dest:  dest.GetId(),
	}
}

type Switch struct {
	*anInstruction

	Cond         int64
	DefaultBlock *BasicBlock

	Label []SwitchLabel
}

var (
	_ Node        = (*Switch)(nil)
	_ User        = (*Switch)(nil)
	_ Instruction = (*Switch)(nil)
)
