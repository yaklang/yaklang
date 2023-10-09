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
			name = fmt.Sprintf("AnonymousFunc-%d", index)
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
		symbol: &Make{
			anInstruction: anInstruction{
				variable: name + "-symbol",
			},
			// I:     parent.symbol,
			anNode: NewNode(),
		},
		err: make(SSAErrors, 0),

		externInstance: make(map[string]Value),
		externType:     make(map[string]Type),
	}
	p.Funcs = append(p.Funcs, f)
	f.symbol.Func = f
	if parent != nil {
		parent.addAnonymous(f)
		// Pos: parent.CurrentPos,
		f.Pos = parent.builder.CurrentPos
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
	p := NewParam(name, false, f)
	// p.typs = append(p.typs, BasicTypesKind[Any])
	f.Param = append(f.Param, p)
	f.WriteVariable(name, p)
}

func (f *Function) ReturnValue() []Value {
	ret := f.ExitBlock.LastInst().(*Return)
	return ret.Results
}

func (f *Function) GetSymbol() *Make {
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

// calculate all return instruction in function, get return type
func handlerReturnType(rs []*Return) []Type {
	tmp := make(map[string][]Type, len(rs))
	for _, r := range rs {
		id := ""
		typs := lo.Map(r.Results,
			func(r Value, _ int) Type {
				t := r.GetType()
				//TODO: modify this id
				id += t.RawString()
				return t
			},
		)

		if _, ok := tmp[id]; !ok {
			tmp[id] = typs
		}
	}

	typs := lo.Values(tmp)
	if len(typs) == 0 {
		return []Type{BasicTypes[Null]}
	} else if len(typs) == 1 {
		return typs[0]
	} else {
		//TODO: how handler this?
		// should set Warn!!
		// and ?? Type ??
		return nil
	}
}

// current function finish
func (f *Function) Finish() {
	f.EnterBlock = f.Blocks[0]
	f.ExitBlock = f.Blocks[len(f.Blocks)-1]

	f.SetType(NewFunctionType("",
		lo.Map(f.Param, func(p *Parameter, _ int) Type {
			t := p.GetType()
			return t
		}),
		handlerReturnType(f.Return),
		f.hasEllipsis,
	))
}
