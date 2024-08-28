package ssa

import (
	"context"
	"reflect"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
)

type ParentScope struct {
	scope ScopeIF
	next  *ParentScope
}

func (p *ParentScope) Create(scope ScopeIF) *ParentScope {
	return &ParentScope{
		scope: scope,
		next:  p,
	}
}

// Function builder API
type FunctionBuilder struct {
	*Function

	ctx context.Context

	// do not use it directly
	_editor *memedit.MemEditor

	// disable free-value
	SupportClosure bool
	// Support obtaining static members and static method, even if the class is not instantiated.
	SupportClassStaticModifier bool
	SupportClass               bool
	PreHandler                 bool
	IncludeStack               *utils.Stack[string]

	Included bool

	RefParameter map[string]struct{}

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call

	// for build
	CurrentBlock *BasicBlock     // current block to build
	CurrentRange memedit.RangeIf // current position in source code
	CurrentFile  string          // current file name

	parentScope *ParentScope

	DefineFunc map[string]any

	MarkedFuncName  string
	MarkedFuncType  *FunctionType
	MarkedFunctions []*Function

	MarkedVariable           *Variable
	MarkedThisObject         Value
	MarkedThisClassBlueprint *ClassBluePrint

	MarkedMemberCallWantMethod bool
	parentBuilder              *FunctionBuilder
	mainBuilder                *FunctionBuilder //global Scope Builder
}

func NewBuilder(editor *memedit.MemEditor, f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		_editor:       editor,
		Function:      f,
		target:        &target{},
		labels:        make(map[string]*BasicBlock),
		CurrentBlock:  nil,
		CurrentRange:  nil,
		parentBuilder: parent,
		RefParameter:  make(map[string]struct{}),
		IncludeStack:  utils.NewStack[string](),
	}
	if parent != nil {
		b.DefineFunc = parent.DefineFunc
		b.MarkedThisObject = parent.MarkedThisObject
		// sub scope
		// b.parentScope = parent.CurrentBlock.ScopeTable
		b.parentScope = parent.parentScope.Create(parent.CurrentBlock.ScopeTable)
		b.SetBuildSupport(parent)

		b.SupportClosure = parent.SupportClosure
		b.SupportClassStaticModifier = parent.SupportClassStaticModifier
		b.SupportClass = parent.SupportClass
		b.ctx = parent.ctx
		b.mainBuilder = parent.mainBuilder
	}

	// b.ScopeStart()
	// b.Function.SetScope(b.CurrentScope)
	var ok bool
	b.CurrentBlock, ok = ToBasicBlock(f.EnterBlock)
	if !ok {
		log.Errorf("function (%v) enter block is not a basic block", f.name)
	}
	f.builder = b
	return b
}

func (b *FunctionBuilder) SetBuildSupport(parent *FunctionBuilder) {
	if parent == nil {
		return
	}
	b.SupportClass = parent.SupportClass
	b.SupportClassStaticModifier = parent.SupportClassStaticModifier
	b.SupportClosure = parent.SupportClosure
}

func (b *FunctionBuilder) SetEditor(editor *memedit.MemEditor) {
	b._editor = editor
}

func (b *FunctionBuilder) GetEditor() *memedit.MemEditor {
	return b._editor
}

// current block is finish?
func (b *FunctionBuilder) IsBlockFinish() bool {
	return b.CurrentBlock.finish
}

// new function
func (b *FunctionBuilder) NewFunc(name string) *Function {
	var f *Function
	if b.SupportClosure {
		f = b.prog.NewFunctionWithParent(name, b.Function)
	} else {
		f = b.prog.NewFunctionWithParent(name, nil)
	}
	f.SetRange(b.CurrentRange)
	f.SetFunc(b.Function)
	f.SetBlock(b.CurrentBlock)
	return f
}

// function stack
func (b *FunctionBuilder) PushFunction(newFunc *Function) *FunctionBuilder {
	build := NewBuilder(b.GetEditor(), newFunc, b)
	// build.MarkedThisObject = b.MarkedThisObject
	if this := b.MarkedThisObject; this != nil {
		newParentScopeLevel := build.parentScope.scope
		newParentScopeLevel = newParentScopeLevel.CreateSubScope()
		// create this object and assign
		v := newParentScopeLevel.CreateVariable(this.GetName(), false)
		newParentScopeLevel.AssignVariable(v, this)
		// update parent  scope
		build.parentScope.scope = newParentScopeLevel
	}
	if build.CurrentRange == nil {
		build.CurrentRange = newFunc.R
	}
	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	return b.parentBuilder
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	if ins, ok := b.Params[len(b.Params)-1].(*Parameter); ins != nil {
		_ = ok
		ins.SetType(NewSliceType(BasicTypes[AnyTypeKind]))
	} else {
		log.Warnf("param contains (%T) cannot be set type and ellipsis", ins)
	}
	b.hasEllipsis = true
}

// add current function defer function
func (b *FunctionBuilder) EmitDefer(i Instruction) {
	deferBlock := b.GetDeferBlock()
	endBlock := b.CurrentBlock
	defer func() {
		b.CurrentBlock = endBlock
	}()
	b.CurrentBlock = deferBlock
	if len(deferBlock.Insts) == 0 {
		deferBlock.Insts = append(deferBlock.Insts, i)
	} else {
		deferBlock.Insts = utils.InsertSliceItem(deferBlock.Insts, Instruction(i), 0)
	}
}

func (b *FunctionBuilder) SetMarkedFunction(name string) (ret func()) {
	originName := b.MarkedFuncName
	originType := b.MarkedFuncType
	ret = func() {
		b.MarkedFuncName = originName
		b.MarkedFuncType = originType
	}

	b.MarkedFuncName = name
	i, ok := b.DefineFunc[name]
	if !ok {
		return
	}
	// fun := b.BuildValueFromAny()
	typ := reflect.TypeOf(i)
	if typ.Kind() != reflect.Func {
		log.Errorf("config define function %s is not function", name)
		return
	}
	funTyp := b.CoverReflectFunctionType(typ, 0)
	b.MarkedFuncType = funTyp
	return
}

func (b *FunctionBuilder) GetMarkedFunction() *FunctionType {
	return b.MarkedFuncType
}

func (b *FunctionBuilder) ReferenceParameter(name string) {
	b.RefParameter[name] = struct{}{}
}
