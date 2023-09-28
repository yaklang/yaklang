package ssa

import (
	"fmt"

	"github.com/samber/lo"
)

func (p *Package) NewFunction(name string) *Function {
	return p.NewFunctionWithParent(name, nil)
}
func (p *Package) NewFunctionWithParent(name string, parent *Function) *Function {
	index := len(p.Funcs)
	if name == "" {
		if parent != nil {
			name = fmt.Sprintf("%s$%d", parent.Name, index)
		} else {
			name = fmt.Sprintf("Anonymousfunc%d", index)
		}
	}
	f := &Function{
		Name:        name,
		Package:     p,
		Param:       make([]*Parameter, 0),
		Blocks:      make([]*BasicBlock, 0),
		EnterBlock:  nil,
		ExitBlock:   nil,
		AnonFuncs:   make([]*Function, 0),
		parent:      nil,
		FreeValues:  make([]Value, 0),
		user:        make([]User, 0),
		symbolTable: make(map[string][]InstructionValue),
		InstReg:     make(map[Instruction]string),
		symbol: &Interface{
			anInstruction: anInstruction{},
			// I:     parent.symbol,
			users: []User{},
		},
		err: make(SSAErrors, 0),
		// for build
	}
	p.Funcs = append(p.Funcs, f)
	f.symbol.Func = f
	if parent != nil {
		parent.addAnonymous(f)
		// Pos: parent.currtenPos,
		f.Pos = parent.builder.currtenPos
	}
	f.EnterBlock = f.NewBasicBlock("entry")
	return f
}

func (f *Function) addAnonymous(anon *Function) {
	f.AnonFuncs = append(f.AnonFuncs, anon)
	anon.parent = f
	anon.symbol.parentI = f.symbol
}

func (f *Function) NewParam(name string) {
	p := &Parameter{
		variable: name,
		Func:     f,
		users:    []User{},
		typs:     BasicTypes[Any],
	}
	// p.typs = append(p.typs, BasicTypesKind[Any])
	f.Param = append(f.Param, p)
	f.WriteVariable(name, p)
}

func (f *Function) ReturnValue() []Value {
	ret := f.ExitBlock.LastInst().(*Return)
	return ret.Results
}

func (f *Function) GetSymbol() *Interface {
	return f.symbol
}

func (f *Function) GetParent() *Function {
	return f.parent
}

// just create a function define, only function parameter type \ return type \ ellipsis
func NewFunctionWithType(name string, typ *FunctionType) *Function {
	f := &Function{
		Name: name,
		Type: typ,
	}
	return f
}

// current function finish
func (f *Function) Finish() {
	f.EnterBlock = f.Blocks[0]
	f.ExitBlock = f.Blocks[len(f.Blocks)-1]

	f.SetType(NewFunctionType("",
		lo.Map(f.Param, func(p *Parameter, _ int) Type { return p.GetType() }),
		lo.Map(f.Return, func(r *Return, _ int) Type { return r.GetType() }),
		f.hasEllipsis,
	))
}
