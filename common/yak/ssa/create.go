package ssa

import (
	"fmt"
	"sync"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func NewProgram(ast *yak.YaklangParser) *Program {
	pkg := &Package{
		Prog:       nil,
		funcs:      make([]*Function, 0),
		buildOnece: sync.Once{},
		ast:        ast,
	}
	prog := &Program{
		Packages: make([]*Package, 0),
		ast:      ast,
	}

	prog.Packages = append(prog.Packages, pkg)
	pkg.Prog = prog

	return prog
}

func (prog *Program) NewPackage() {

}

func (p *Package) NewFunction(name string) *Function {
	f := &Function{
		name:         name,
		Package:      p,
		Param:        make([]*Parameter, 0),
		Blocks:       make([]*BasicBlock, 0),
		EnterBlock:   nil,
		ExitBlock:    nil,
		AnonFuncs:    make([]*Function, 0),
		parent:       nil,
		FreeValue:    make([]Value, 0),
		user:         make([]User, 0),
		currentBlock: nil,
		currentDef:   make(map[string]map[*BasicBlock]Value),
	}
	p.funcs = append(p.funcs, f)

	enter := f.newBasicBlock("entry")
	f.currentBlock = enter
	return f
}
func (f *Function) newBasicBlock(name string) *BasicBlock {
	return f.newBasicBlockWithSealed(name, true)
}
func (f *Function) newBasicBlockUnSealed(name string) *BasicBlock {
	return f.newBasicBlockWithSealed(name, false)
}

func (f *Function) newBasicBlockWithSealed(name string, isSealed bool) *BasicBlock {
	index := len(f.Blocks)
	if name != "" {
		name = fmt.Sprintf("%s%d", name, index)
	} else {
		name = fmt.Sprintf("b%d", index)
	}
	b := &BasicBlock{
		Index:         index,
		Name:          name,
		Parent:        f,
		Preds:         make([]*BasicBlock, 0),
		Succs:         make([]*BasicBlock, 0),
		Instrs:        make([]Instruction, 0),
		Phis:          make([]*Phi, 0),
		isSealed:      isSealed,
		inCompletePhi: make(map[string]*Phi),
		user:          make([]User, 0),
	}
	f.Blocks = append(f.Blocks, b)
	return b
}
