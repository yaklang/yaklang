package ssa

import (
	"github.com/gobwas/glob"
	"regexp"
	"strings"
)

func (p *Program) DeleteInstruction(inst Instruction) {
	p.Cache.DeleteInstruction(inst)
	//
	for name := range inst.GetAllVariables() {
		p.RemoveInstructionInVariable(name, inst)
	}
}

// set virtual register, and this virtual-register will be instruction-id and set to the instruction
func (p *Program) SetVirtualRegister(i Instruction) {
	p.Cache.SetInstruction(i)
}

func (p *Program) GetInstructionById(id int64) Instruction {
	return p.Cache.GetInstruction(id)
}

func (p *Program) SetInstructionWithName(name string, i Instruction) {
	p.Cache.AddVariable(name, i)
	if !strings.Contains(name, ".") {
		i.SetVerboseName(name)
	}
}

func (p *Program) RemoveInstructionInVariable(name string, i Instruction) {
	p.Cache.RemoveVariable(name, i)
}

func (p *Program) GetInstructionsByName(name string) []Instruction {
	return p.Cache.GetByVariable(name)
}

func (p *Program) GetInstructionsByGlob(g glob.Glob) []Instruction {
	return p.Cache.GetByVariableGlob(g)
}

func (p *Program) GetInstructionsByRegexp(r *regexp.Regexp) []Instruction {
	return p.Cache.GetByVariableRegexp(r)
}
