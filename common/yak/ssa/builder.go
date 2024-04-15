package ssa

import (
	"github.com/yaklang/yaklang/common/utils/memedit"
	"reflect"

	"github.com/yaklang/yaklang/common/log"
)

type ParentScope struct {
	scope *Scope
	next  *ParentScope
}

func (p *ParentScope) Create(scope *Scope) *ParentScope {
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

	ExternInstance map[string]any
	ExternLib      map[string]map[string]any
	DefineFunc     map[string]any

	MarkedFuncType  *FunctionType
	MarkedFunctions []*Function

	MarkedVariable           *Variable
	MarkedThisObject         Value
	MarkedThisClassBlueprint *ClassBluePrint

	parentBuilder *FunctionBuilder
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
		b.ExternInstance = parent.ExternInstance
		b.ExternLib = parent.ExternLib
		b.DefineFunc = parent.DefineFunc
		// sub scope
		// b.parentScope = parent.CurrentBlock.ScopeTable
		b.parentScope = parent.parentScope.Create(parent.CurrentBlock.ScopeTable)
		b.SupportClosure = parent.SupportClosure
		b.MarkedThisObject = parent.MarkedThisObject
	}

	// b.ScopeStart()
	// b.Function.SetScope(b.CurrentScope)
	b.CurrentBlock = f.EnterBlock
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
		f = b.Package.NewFunctionWithParent(name, b.Function)
	} else {
		f = b.Package.NewFunctionWithParent(name, nil)
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
		newParentScopeLevel = newParentScopeLevel.CreateSubScope().(*Scope)
		// create this object and assign
		v := newParentScopeLevel.CreateVariable(this.GetName(), false)
		newParentScopeLevel.AssignVariable(v, this)
		// update parent  scope
		build.parentScope.scope = newParentScopeLevel
	}
	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	return b.parentBuilder
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	b.Param[len(b.Param)-1].SetType(NewSliceType(BasicTypes[AnyTypeKind]))
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
