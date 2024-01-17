package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/samber/lo"
)

func (p *Package) NewFunction(name string) *Function {
	return p.NewFunctionWithParent(name, nil)
}
func (p *Package) NewFunctionWithParent(name string, parent *Function) *Function {
	index := len(p.Funcs)
	if index == 0 {
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
		anInstruction:  NewInstruction(),
		anValue:        NewValue(),
		Package:        p,
		Param:          make([]*Parameter, 0),
		hasEllipsis:    false,
		Blocks:         make([]*BasicBlock, 0),
		EnterBlock:     nil,
		ExitBlock:      nil,
		ChildFuncs:     make([]*Function, 0),
		parent:         nil,
		FreeValues:     make(map[string]*Parameter),
		SideEffects:    make(map[string]Value),
		externInstance: make(map[string]Value),
		externType:     make(map[string]Type),
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
	f.EnterBlock = f.NewBasicBlock("entry")
	return f
}

func (f *Function) GetProgram() *Program {
	return f.Package.Prog
}

func (f *Function) GetReferenceFiles() []string {
	return f.referenceFiles.Keys()
}

func (f *Function) addAnonymous(anon *Function) {
	f.ChildFuncs = append(f.ChildFuncs, anon)
	anon.parent = f
}

func (f *FunctionBuilder) NewParam(name string) *Parameter {
	p := NewParam(name, false, f)
	p.SetRange(f.CurrentRange)
	// f.CurrentScope.AddVariable(NewVariable(name, p), f.CurrentRange)
	// p.typs = append(p.typs, BasicTypesKind[Any])
	f.Param = append(f.Param, p)
	f.writeVariableByBlock(name, p, f.EnterBlock)
	return p
}

func (f *Function) AddSideEffect(name string, v Value) {
	f.SideEffects[name] = v
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
		anInstruction:  NewInstruction(),
		anValue:        NewValue(),
		referenceFiles: omap.NewOrderedMap(map[string]string{}),
	}
	f.SetType(typ)
	f.SetName(name)
	return f
}

// calculate all return instruction in function, get return type
func handlerReturnType(rs []*Return) Type {
	tmp := make(map[string]Type, len(rs))
	for _, r := range rs {
		id := ""
		typs := r.GetType()

		if _, ok := tmp[id]; !ok {
			tmp[id] = typs
		}
	}

	typs := lo.Values(tmp)
	if len(typs) == 0 {
		return BasicTypes[Null]
	} else if len(typs) == 1 {
		return typs[0]
	} else {
		//TODO: how handler this? multiple return with different type
		// should set Warn!!
		// and ?? Type ??
		return nil
	}
}

// current function finish
func (f *Function) Finish() {
	f.EnterBlock = f.Blocks[0]
	f.ExitBlock = f.Blocks[len(f.Blocks)-1]

	funType := NewFunctionType("",
		lo.Map(f.Param, func(p *Parameter, _ int) Type {
			t := p.GetType()
			return t
		}),
		handlerReturnType(f.Return),
		f.hasEllipsis,
	)
	f.SetType(funType)
	if len(f.FreeValues) != 0 {
		funType.SetFreeValue(
			lo.Keys(f.FreeValues),
		)
	}
	if len(f.SideEffects) != 0 {
		funType.SetSideEffect(
			lo.MapToSlice(f.SideEffects, func(name string, _ Value) string { return name }),
		)
	}
}
