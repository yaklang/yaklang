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
		Param:        make([]Value, 0),
		Blocks:       make([]*BasicBlock, 0),
		AnonFuncs:    make([]*Function, 0),
		parent:       nil,
		Package:      p,
		FreeValue:    make([]Value, 0),
		user:         make([]User, 0),
		currentBlock: nil,
		currentDef:   make(map[string]map[*BasicBlock]Value),
	}
	p.funcs = append(p.funcs, f)
	return f
}

func (f *Function) newBasicBlock(comment string) *BasicBlock {
	index := len(f.Blocks)
	if comment != "" {
		comment = fmt.Sprintf("%s%d", comment, index)
	}
	b := &BasicBlock{
		Comment: comment,
		Index:   index,
		Parent:  f,
	}
	f.Blocks = append(f.Blocks, b)
	return b
}
