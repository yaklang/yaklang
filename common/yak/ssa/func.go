package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func (p *Package) NewFunction(name string) *Function {
	return p.NewFunctionWithParent(name, nil)
}

func (p *Package) NewFunctionWithParent(name string, parent *Function) *Function {
	index := len(p.Funcs)
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
		anValue:        NewValue(),
		Package:        p,
		Params:         make([]*Parameter, 0),
		hasEllipsis:    false,
		Blocks:         make([]*BasicBlock, 0),
		EnterBlock:     nil,
		ExitBlock:      nil,
		ChildFuncs:     make([]*Function, 0),
		parent:         nil,
		FreeValues:     make(map[string]*Parameter),
		SideEffects:    make([]*FunctionSideEffect, 0),
		builder:        nil,
		referenceFiles: omap.NewOrderedMap(map[string]string{}),
	}
	f.SetName(name)
	if parent != nil {
		parent.addAnonymous(f)
		// Pos: parent.CurrentPos,
		f.SetRange(parent.builder.CurrentRange)
	} else {
		p.Funcs[name] = f
	}
	p.Prog.SetVirtualRegister(f)
	f.EnterBlock = f.NewBasicBlock("entry")
	return f
}

func (f *Function) GetType() Type {
	if f.Type != nil {
		return f.Type
	} else {
		return GetAnyType()
	}
}

func (f *Function) SetType(t Type) {
	if funTyp, ok := ToFunctionType(t); ok {
		f.Type = funTyp
	} else {
		log.Errorf("ssa.Function type cannot covnert to FunctionType: %v", t)
	}
}

func (f *Function) GetProgram() *Program {
	if f.Package == nil {
		return nil
	}
	return f.Package.Prog
}

func (f *Function) GetFunc() *Function {
	return f
}

func (f *Function) GetReferenceFiles() []string {
	return f.referenceFiles.Keys()
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

func (f *FunctionBuilder) appendParam(p *Parameter, token ...CanStartStopToken) {
	f.Params = append(f.Params, p)
	p.FormalParameterIndex = len(f.Params) - 1
	p.IsFreeValue = false
	variable := f.CreateVariable(p.GetName(), token...)
	f.AssignVariable(variable, p)
}

func (f *Function) ReturnValue() []Value {
	ret := f.ExitBlock.LastInst().(*Return)
	return ret.Results
}

func (f *Function) IsMain() bool {
	return f.GetName() == "main"
}

func (f *Function) GetParent() *Function {
	return f.parent
}

// just create a function define, only function parameter type \ return type \ ellipsis
func NewFunctionWithType(name string, typ *FunctionType) *Function {
	f := &Function{
		anValue:        NewValue(),
		referenceFiles: omap.NewOrderedMap(map[string]string{}),
	}
	f.SetType(typ)
	f.SetName(name)
	return f
}

func (f *Function) IsMethod() bool {
	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
	}
	return f.Type.IsMethod
}

func (f *Function) SetMethod(is bool, objType Type) {
	if f.Type == nil {
		f.Type = NewFunctionType("", nil, nil, false)
	}
	f.Type.IsMethod = is
	f.Type.ObjectType = objType
}
