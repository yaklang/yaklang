package ssa

import (
	"reflect"

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

	// do not use it directly
	_editor *memedit.MemEditor

	// disable free-value
	SupportClosure bool
	// Support obtaining static members and static method, even if the class is not instantiated.
	SupportClassStaticModifier bool
	SupportClass               bool

	RefParameter map[string]struct{}

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call
	deferExpr []*Call // defer function, reverse  for-range

	// for build
	CurrentBlock *BasicBlock // current block to build
	CurrentRange *Range      // current position in source code
	CurrentFile  string      // current file name

	parentScope *ParentScope

	DefineFunc map[string]any

	MarkedFuncType  *FunctionType
	MarkedFunctions []*Function

	MarkedVariable           *Variable
	MarkedThisObject         Value
	MarkedThisClassBlueprint *ClassBluePrint

	MarkedIsStaticMethod bool
	parentBuilder        *FunctionBuilder
}

func NewBuilder(editor *memedit.MemEditor, f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		_editor:       editor,
		Function:      f,
		target:        &target{},
		labels:        make(map[string]*BasicBlock),
		deferExpr:     make([]*Call, 0),
		CurrentBlock:  nil,
		CurrentRange:  nil,
		parentBuilder: parent,
		RefParameter:  make(map[string]struct{}),
	}
	if parent != nil {
		b.DefineFunc = parent.DefineFunc
		// sub scope
		// b.parentScope = parent.CurrentBlock.ScopeTable
		b.parentScope = parent.parentScope.Create(parent.CurrentBlock.ScopeTable)
		b.SupportClosure = parent.SupportClosure
		b.SupportClass = parent.SupportClass
		b.MarkedThisObject = parent.MarkedThisObject
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
func (b *FunctionBuilder) AddDefer(call *Call) {
	b.deferExpr = append(b.deferExpr, call)
}

func (b *FunctionBuilder) SetMarkedFunction(name string) {
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
}

func (b *FunctionBuilder) GetMarkedFunction() *FunctionType {
	return b.MarkedFuncType
}

func (b *FunctionBuilder) ReferenceParameter(name string) {
	b.RefParameter[name] = struct{}{}
}
