package ssa

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

var log = ssalog.Log

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

	//正在解析的include栈
	SyntaxIncludingStack *utils.Stack[string]
	includeStack         *utils.Stack[*Program]

	Included bool
	IsReturn bool

	RefParameter map[string]struct{ Index int }

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call

	// for build
	CurrentBlock *BasicBlock    // current block to build
	CurrentRange *memedit.Range // current position in source code
	CurrentFile  string         // current file name

	parentScope *ParentScope

	DefineFunc map[string]any

	MarkedFuncName  string
	MarkedFuncType  *FunctionType
	MarkedFunctions []*Function

	MarkedVariable           *Variable
	MarkedThisObject         Value
	MarkedThisClassBlueprint *Blueprint

	MarkedMemberCallWantMethod bool
	parentBuilder              *FunctionBuilder

	//External variables acquired by use will determine whether sideEffect should be generated when assign variable is assigned
	captureFreeValue map[string]struct{}
}

func NewBuilder(editor *memedit.MemEditor, f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		_editor:              editor,
		Function:             f,
		target:               &target{},
		labels:               make(map[string]*BasicBlock),
		CurrentBlock:         nil,
		CurrentRange:         nil,
		parentBuilder:        parent,
		RefParameter:         make(map[string]struct{ Index int }),
		SyntaxIncludingStack: utils.NewStack[string](),
		includeStack:         utils.NewStack[*Program](),
		captureFreeValue:     make(map[string]struct{}),
	}
	if parent != nil {
		b.DefineFunc = parent.DefineFunc
		b.MarkedThisObject = parent.MarkedThisObject
		// sub scope
		// b.parentScope = parent.CurrentBlock.ScopeTable
		b.parentScope = parent.parentScope.Create(parent.CurrentBlock.ScopeTable)
		b.SetBuildSupport(parent)

		b.SupportClosure = parent.SupportClosure
		// b.SupportClassStaticModifier = parent.SupportClassStaticModifier
		// b.SupportClass = parent.SupportClass
		b.ctx = parent.ctx
	}

	// b.ScopeStart()
	// b.Function.SetScope(b.CurrentScope)
	if block, ok := f.GetBasicBlockByID(f.EnterBlock); ok && block != nil {
		b.CurrentBlock = block
	}
	f.builder = b
	return b
}
func (f *FunctionBuilder) AddCaptureFreevalue(name string) {
	f.captureFreeValue[name] = struct{}{}
}
func (b *FunctionBuilder) GetFunc(name, pkg string) *Function {
	var function *Function
	b.includeStack.ForeachStack(func(program *Program) bool {
		functionEx := program.GetFunctionEx(name, pkg)
		if functionEx != nil {
			function = functionEx
			return false
		}
		return true
	})
	if function != nil {
		return function
	}
	return b.GetProgram().GetFunction(name, pkg)
}

func (b *FunctionBuilder) SetBuildSupport(parent *FunctionBuilder) {
	if parent == nil {
		return
	}
	// b.SupportClass = parent.SupportClass
	// b.SupportClassStaticModifier = parent.SupportClassStaticModifier
	b.SupportClosure = parent.SupportClosure
}

func (b *FunctionBuilder) SetEditor(editor *memedit.MemEditor) {
	b._editor = editor
}

func (b *FunctionBuilder) GetEditor() *memedit.MemEditor {
	return b._editor
}

func (b *FunctionBuilder) GetLanguage() consts.Language {
	lang, err := consts.ValidateLanguage(b.GetProgram().Language)
	_ = err
	return lang
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
	f.SetCurrentBlueprint(b.MarkedThisClassBlueprint)
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
	if b.MarkedThisClassBlueprint != nil {
		build.MarkedThisClassBlueprint = b.MarkedThisClassBlueprint
	}

	if build.CurrentRange == nil {
		build.CurrentRange = newFunc.R
	}

	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	// if global := b.GetProgram().GlobalScope; global != nil {
	// 	for i, m := range global.GetAllMember() {
	// 		name := i.String()
	// 		value := b.EmitPhi(name, []Value{m, b.PeekValue(name)})
	// 		global.SetStringMember(name, value)
	// 	}
	// }

	return b.parentBuilder
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	if inst, ok := b.GetInstructionById(b.Params[len(b.Params)-1]); ok && inst != nil {
		if ins, ok := ToParameter(inst); ok {
			ins.SetType(NewSliceType(CreateAnyType()))
		} else {
			log.Warnf("param contains (%T) cannot be set type and ellipsis", ins)
		}
	}
	b.hasEllipsis = true
}

func (b *FunctionBuilder) EmitDefer(instruction Instruction) {
	deferBlock := b.GetDeferBlock()
	endBlock := b.CurrentBlock
	defer func() {
		b.CurrentBlock = endBlock
	}()
	b.CurrentBlock = deferBlock
	b.emitEx(instruction, func(instruction Instruction) {
		if c, flag := ToCall(instruction); flag {
			c.handlerGeneric()
			c.handlerObjectMethod()
			c.handlerReturnType()
			c.handleCalleeFunction()
		}
		if len(deferBlock.Insts) == 0 {
			deferBlock.Insts = append(deferBlock.Insts, instruction.GetId())
		} else {
			deferBlock.Insts = utils.InsertSliceItem(deferBlock.Insts, instruction.GetId(), 0)
		}
	})
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

func (b *FunctionBuilder) ReferenceParameter(name string, index int) {
	b.RefParameter[name] = struct{ Index int }{Index: index}
}
func (b *FunctionBuilder) ClassConstructor(bluePrint *Blueprint, args []Value) Value {
	method := bluePrint.GetMagicMethod(Constructor, b)
	constructor := b.NewCall(method, args)
	b.EmitCall(constructor)
	constructor.SetType(bluePrint)
	destructor := bluePrint.GetMagicMethod(Destructor, b)
	call := b.NewCall(destructor, []Value{constructor})
	b.EmitDefer(call)
	return constructor
}

func (b *FunctionBuilder) ClassConstructorWithoutDeferDestructor(bluePrint *Blueprint, args []Value) Value {
	method := bluePrint.GetMagicMethod(Constructor, b)
	constructor := b.NewCall(method, args)
	b.EmitCall(constructor)
	return constructor
}

func (b *FunctionBuilder) GetStaticMember(classname *Blueprint, field string) *Variable {
	return b.CreateVariable(fmt.Sprintf("%s_%s", classname.Name, strings.TrimPrefix(field, "$")))
}

func (b *FunctionBuilder) GenerateDependence(pkgs []*dxtypes.Package, filename string) {
	container := b.ReadValue("__dependency__")
	if utils.IsNil(container) {
		log.Warnf("not found __dependency__")
		return
	}

	getMinOffsetRng := func(rs1, rs2 []*memedit.Range) *memedit.Range {
		if len(rs1) == 0 || len(rs2) == 0 {
			return nil
		}

		var offsetSlice1 []int
		offsetMap1 := lo.SliceToMap(rs1, func(item *memedit.Range) (int, *memedit.Range) {
			offsetSlice1 = append(offsetSlice1, item.GetStartOffset())
			return item.GetStartOffset(), item
		})

		offsetSlice2 := lo.Map(rs2, func(item *memedit.Range, index int) int {
			return item.GetStartOffset()
		})
		sort.Ints(offsetSlice2)

		minDist := -1
		var minRng *memedit.Range
		for _, offset1 := range offsetSlice1 {
			// 在offsetSlice2中找距离offset1最近的offset2
			for _, offset2 := range offsetSlice2 {
				dist := offset1 - offset2
				if dist < 0 {
					dist = -dist
				}
				if minDist == -1 || dist < minDist {
					minDist = dist
					minRng = offsetMap1[offset1]
				}
			}
		}
		return minRng
	}

	getDependencyRangeByName := func(name string) *memedit.Range {
		id := strings.Split(name, ":")
		if len(id) != 2 {
			return nil
		}
		group, artifact := id[0], id[1]
		// 先匹配artifact，如果只匹配到一个位置
		// 那么就是确定的位置
		rs1 := b.GetRangesByText(artifact)
		if len(rs1) == 1 {
			return rs1[0]
		}
		// 再匹配group，如果只匹配到一个位置
		//那么就是确定的位置
		rs2 := b.GetRangesByText(group)
		if len(rs2) == 1 {
			return rs2[0]
		}
		// 返回每个rs1中位置和rs2最近的
		// 返回匹配到的artifact所有位置
		return getMinOffsetRng(rs1, rs2)
	}
	/*
		__dependency__.name?{}
	*/
	b.SetEmptyRange()
	for _, pkg := range pkgs {
		sub := b.EmitEmptyContainer()

		if pkg.Name == "" {
			continue
		}
		rng := getDependencyRangeByName(pkg.Name)
		for k, v := range map[string]string{
			"name":     pkg.Name,
			"version":  pkg.Version,
			"filename": filename,
		} {
			constInst := b.EmitConstInstPlaceholder(v)
			if rng != nil {
				constInst.SetRange(rng)
			}
			b.AssignVariable(
				b.CreateMemberCallVariable(sub, b.EmitUndefined(k)),
				constInst,
			)
		}

		pkgItem := b.CreateMemberCallVariable(container, b.EmitUndefined(pkg.Name))
		b.AssignVariable(pkgItem, sub)
	}
}

func (b *FunctionBuilder) SetForceCapture(bo bool) {
	b.CurrentBlock.ScopeTable.SetForceCapture(bo)
}

func (b *FunctionBuilder) GetForceCapture() bool {
	return b.CurrentBlock.ScopeTable.GetForceCapture()
}
