package ssa

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

func (p *Program) NewFunction(name string) *Function {
	return p.NewFunctionWithParent(name, nil)
}

func (p *Program) NewFunctionWithParent(name string, parent *Function) *Function {
	index := p.Funcs.Len()
	if index == 0 && name == "" {
		name = "main"
	}
	if name == "" {
		if parent != nil {
			name = fmt.Sprintf("%s$%d", parent.GetName(), index)
		} else {
			name = fmt.Sprintf("AnonymousFunc-%d", index)
		}
	}
	f := &Function{
		LazyBuilder: NewLazyBuilder("Function:" + name),
		anValue:     NewValue(),
		Params:      make([]int64, 0),
		hasEllipsis: false,
		Blocks:      make([]int64, 0),
		EnterBlock:  0,
		ExitBlock:   0,
		ChildFuncs:  make([]int64, 0),
		FreeValues:  make(map[*Variable]int64),
		SideEffects: make([]*FunctionSideEffect, 0),
		builder:     nil,
	}
	f.SetName(name)
	f.SetProgram(p)
	p.SetVirtualRegister(f)

	if parent != nil {
		parent.addAnonymous(f)
		// Pos: parent.CurrentPos,
		f.SetRange(parent.builder.CurrentRange)
	} else {
		// p.Funcs[name] = f
		if _, ok := p.Funcs.Get(name); ok {
			log.Debugf("function %s already exists", name)
			name = fmt.Sprintf("%s$%d", name, index)
		}
		p.Funcs.Set(name, f)
	}
	// function 's Range is essential!
	if f.GetRange() == nil {
		// if editor := p.getCurrentEditor(); editor != nil {
		// 	f.SetRangeInit(editor)
		// } else {
		if f.GetParent() != nil {
			log.Warnf("the program must contains a editor to init function range: %v", p.Name)
		}
		// }
	}

	enter := f.NewBasicBlock("entry")
	enter.SetScope(NewScope(f, p.GetProgramName()))
	f.EnterBlock = enter.GetId()
	return f
}

func (f *Function) SetCurrentBlueprint(blueprint *Blueprint) {
	f.currentBlueprint = blueprint
}
func (f *Function) GetCurrentBlueprint() *Blueprint {
	return f.currentBlueprint
}
func (f *Function) GetType() Type {
	if f == nil {
		return defaultAnyType
	}
	// If Type is nil, try to build the function first (trigger lazy builder)
	// This ensures FunctionType is properly set before accessing
	if f.Type == nil {
		f.Build()
	}
	if f.Type != nil {
		return f.Type
	}
	return defaultAnyType
}

func (f *Function) AddThrow(vs ...Value) {
	f.Throws = append(f.Throws, lo.Map(vs, func(v Value, _ int) int64 {
		return v.GetId()
	})...)
}

func (f *Function) SetType(t Type) {
	if utils.IsNil(f) || utils.IsNil(t) {
		return
	}

	if funTyp, ok := ToFunctionType(t); ok {
		f.Type = funTyp
	} else if t.GetTypeKind() == AnyTypeKind {
		log.Infof("skip any type for Function: %v alias: %v", f.name, f.verboseName)
	} else if t != nil {
		log.Warnf("ssa.Function type cannot be set type from: %v", t)
	}
}

func (f *Function) SetGeneric(b bool) {
	f.isGeneric = b
}

func (f *Function) IsGeneric() bool {
	return f.isGeneric
}

func (f *Function) GetFunc() *Function {
	return f
}

func (f *Function) addAnonymous(anon *Function) {
	f.ChildFuncs = append(f.ChildFuncs, anon.GetId())
	anon.parent = f.GetId()
}

func (f *FunctionBuilder) NewParam(name string, pos ...CanStartStopToken) *Parameter {
	p := NewParam(name, false, f)
	f.appendParam(p, pos...)
	return p
}

func (f *FunctionBuilder) NewParameterMember(name string, obj *Parameter, key Value) *ParameterMember {
	paraMember := NewParamMember(name, f, obj, key)
	f.ParameterMembers = append(f.ParameterMembers, paraMember.GetId())
	paraMember.FormalParameterIndex = len(f.ParameterMembers) - 1
	if f.MarkedThisObject != nil &&
		obj.GetDefault() != nil &&
		f.MarkedThisObject.GetId() == obj.GetDefault().GetId() {
		f.SetMethod(true, obj.GetType())
	}
	variable := f.CreateVariable(name)
	f.AssignVariable(variable, paraMember)
	return paraMember
}
func (f *FunctionBuilder) NewMoreParameterMember(name string, member *ParameterMember, key Value) *ParameterMember {
	paraMember := NewMoreParamMember(name, f, member, key)
	variable := f.CreateVariable(name)
	f.AssignVariable(variable, paraMember)
	f.ParameterMembers = append(f.ParameterMembers, paraMember.GetId())
	paraMember.FormalParameterIndex = len(f.ParameterMembers) - 1
	return paraMember
}

func (f *FunctionBuilder) appendParam(p *Parameter, token ...CanStartStopToken) {
	f.Params = append(f.Params, p.GetId())
	p.FormalParameterIndex = len(f.Params) - 1
	p.IsFreeValue = false
	variable := f.CreateVariableForce(p.GetName(), token...)
	variable.AddRange(f.CurrentRange, false)
	f.AssignVariable(variable, p)
}

func (f *Function) ReturnValue() []Value {
	exitBlock, ok := f.GetBasicBlockByID(f.ExitBlock)
	if !ok || exitBlock == nil {
		log.Warnf("function exit block cannot convert to BasicBlock: %v", f.ExitBlock)
		return nil
	}
	ret := exitBlock.LastInst().(*Return)
	return f.GetValuesByIDs(ret.Results)
}

func (f *Function) IsMain() bool {
	return f.GetName() == string(MainFunctionName)
}

func (f *Function) GetParent() *Function {
	if f.parent <= 0 {
		return nil
	}

	parent, ok := f.GetValueById(f.parent)
	if !ok || parent == nil {
		log.Warnf("function parent not found: %v", f.parent)
		return nil
	}
	fu, ok := ToFunction(parent)
	if ok {
		return fu
	}
	log.Warnf("function parent cannot convert to Function: %v", parent)
	return nil
}

// just create a function define, only function parameter type \ return type \ ellipsis
func NewFunctionWithType(name string, typ *FunctionType) *Function {
	f := &Function{
		anValue: NewValue(),
	}
	f.SetType(typ)
	f.SetName(name)
	f.GetProgram().SetVirtualRegister(f)
	return f
}

func (f *Function) IsMethod() bool {
	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
		f.Type.This = f
	}
	return f.Type.IsMethod
}

func (f *Function) SetMethod(is bool, objType Type) {
	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
		f.Type.This = f
	}
	f.Type.IsMethod = is
	f.Type.ObjectType = objType
}

func (f *Function) SetVerboseName(name string) {
	// only set once
	if f.verboseName == "" {
		f.verboseName = name
	}
}
