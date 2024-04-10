package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func (p *Program) DeleteInstruction(i Instruction) {
	// delete IdToInstructionMap
	p.IdToInstructionMap.Delete(i.GetId())

	// delete ConstInstruction
	if c, ok := ToConst(i); ok {
		p.ConstInstruction.Delete(c.GetId())
	}

	// delete NameToInstructions
	for name := range i.GetAllVariables() {
		insts, ok := p.NameToInstructions.Get(name)
		if !ok {
			continue
		}
		insts = utils.RemoveSliceItem(insts, i)
		p.NameToInstructions.Set(name, insts)
	}
}

// set virtual register, and this virtual-register will be instruction-id and set to the instruction
func (p *Program) SetVirtualRegister(i Instruction) {
	if p.IdToInstructionMap.Have(i) {
		return
	}

	programRegister := func(newId int) {
		// id := p.IdToInstructionMap.Len()
		i.SetId(newId)
		p.IdToInstructionMap.Set(newId, i)

		// set const
		if c, ok := ToConst(i); ok {
			p.ConstInstruction.Set(newId, c)
		}
	}

	if p.persistentBackend != nil {
		p.persistentBackendMutex.Lock()
		defer p.persistentBackendMutex.Unlock()

		id, register := p.persistentBackend()
		programRegister(id)
		err := register(i)
		if err != nil {
			log.Error("persistent backend error: " + err.Error())
			return
		}
		return
	}

	id, _, _ := p.IdToInstructionMap.Last()
	id++
	programRegister(id)
}

func (p *Program) GetInstructionById(id int) Instruction {
	if i, ok := p.IdToInstructionMap.Get(id); ok {
		return i
	} else {
		return nil
	}
}

func (p *Program) SetInstructionWithName(name string, i Instruction) {
	insts, ok := p.NameToInstructions.Get(name)
	if ok {
		insts = append(insts, i)
	} else {
		insts = make([]Instruction, 0, 1)
		insts = append(insts, i)
	}
	if !strings.Contains(name, ".") {
		i.SetVerboseName(name)
	}
	p.NameToInstructions.Set(name, insts)
}

func (p *Program) RemoveInstructionWithName(name string, i Instruction) {
	insts, ok := p.NameToInstructions.Get(name)
	if ok {
		insts = utils.RemoveSliceItem(insts, i)
		p.NameToInstructions.Set(name, insts)
	}
}

func (p *Program) GetInstructionsByName(name string) []Instruction {
	insts, ok := p.NameToInstructions.Get(name)
	if !ok {
		insts = make([]Instruction, 0)
	}
	return insts
}
