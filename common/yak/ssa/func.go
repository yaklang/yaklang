package ssa

import (
	"fmt"
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
		anValue:     NewValue(),
		Params:      make([]Value, 0),
		hasEllipsis: false,
		Blocks:      make([]Instruction, 0),
		EnterBlock:  nil,
		ExitBlock:   nil,
		ChildFuncs:  make([]Value, 0),
		parent:      nil,
		FreeValues:  make(map[*Variable]Value),
		SideEffects: make([]*FunctionSideEffect, 0),
		builder:     nil,
	}
	f.SetName(name)
	f.SetProgram(p)

	if parent != nil {
		parent.addAnonymous(f)
		// Pos: parent.CurrentPos,
		f.SetRange(parent.builder.CurrentRange)
	} else {
		// p.Funcs[name] = f
		if _, ok := p.Funcs.Get(name); ok {
			log.Errorf("function %s already exists", name)
			name = fmt.Sprintf("%s$%d", name, index)
		}
		p.Funcs.Set(name, f)
	}
	p.SetVirtualRegister(f)
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
	f.EnterBlock = enter
	return f
}

func (f *Function) SetCurrentBlueprint(blueprint *Blueprint) {
	f.currentBlueprint = blueprint
}
func (f *Function) GetCurrentBlueprint() *Blueprint {
	return f.currentBlueprint
}
func (f *Function) GetType() Type {
	if f != nil && f.Type != nil {
		return f.Type
	} else {
		return CreateAnyType()
	}
}

func (f *Function) SetType(t Type) {
	if t == nil {
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
	f.ChildFuncs = append(f.ChildFuncs, anon)
	anon.parent = f
}

func (f *FunctionBuilder) NewParam(name string, pos ...CanStartStopToken) *Parameter {
	p := NewParam(name, false, f)
	f.appendParam(p, pos...)
	return p
}

func (f *FunctionBuilder) NewParameterMember(name string, obj *Parameter, key Value) *ParameterMember {
	paraMember := NewParamMember(name, f, obj, key)
	f.ParameterMembers = append(f.ParameterMembers, paraMember)
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
	f.ParameterMembers = append(f.ParameterMembers, paraMember)
	paraMember.FormalParameterIndex = len(f.ParameterMembers) - 1
	return paraMember
}

func (f *FunctionBuilder) appendParam(p *Parameter, token ...CanStartStopToken) {
	f.Params = append(f.Params, p)
	p.FormalParameterIndex = len(f.Params) - 1
	p.IsFreeValue = false
	variable := f.CreateVariableForce(p.GetName(), token...)
	variable.AddRange(f.CurrentRange, false)
	f.AssignVariable(variable, p)
}

func (f *Function) ReturnValue() []Value {
	exitBlock, ok := ToBasicBlock(f.ExitBlock)
	if !ok {
		log.Warnf("function exit block cannot convert to BasicBlock: %v", f.ExitBlock)
		return nil
	}
	ret := exitBlock.LastInst().(*Return)
	return ret.Results
}

func (f *Function) IsMain() bool {
	return f.GetName() == string(MainFunctionName)
}

func (f *Function) GetParent() *Function {
	if f.parent == nil {
		return nil
	}

	fu, ok := ToFunction(f.parent)
	if ok {
		return fu
	}
	log.Warnf("function parent cannot convert to Function: %v", f.parent)
	return nil
}

// just create a function define, only function parameter type \ return type \ ellipsis
func NewFunctionWithType(name string, typ *FunctionType) *Function {
	f := &Function{
		anValue: NewValue(),
	}
	f.SetType(typ)
	f.SetName(name)
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
