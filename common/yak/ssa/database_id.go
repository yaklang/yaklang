package ssa

import (
	"strings"
)

func (p *Program) DeleteInstruction(inst Instruction) {
	if p == nil {
		return
	}
	p.Cache.DeleteInstruction(inst)

	if assignable, ok := inst.(AssignAble); ok {
		for name := range assignable.GetAllVariables() {
			p.Cache.RemoveVariable(name, inst)
		}
	}
}

// set virtual register, and this virtual-register will be instruction-id and set to the instruction
func (p *Program) SetVirtualRegister(i Instruction) {
	if p == nil {
		return
	}
	p.Cache.SetInstruction(i)
	i.RefreshString()
}

func (p *Program) GetInstructionById(id int64) (Instruction, bool) {
	if p == nil {
		return nil, false
	}
	inst := p.Cache.GetInstruction(id)
	return inst, inst != nil
}

func (p *Program) AddConstInstruction(instruction Instruction) {
	if p == nil {
		return
	}
	p.Cache.AddConst(instruction)
}
func (p *Program) SetInstructionWithName(name string, i Instruction) {
	if p == nil {
		return
	}
	p.Cache.AddVariable(name, i)
	if !strings.Contains(name, ".") {
		i.SetVerboseName(name)
	}
}
