package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
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
}

func (p *Program) GetInstructionById(id int64) Instruction {
	if p == nil {
		return nil
	}
	return GetEx[Instruction](p.Cache, id)
}

func GetEx[T Instruction](c *Cache, id int64) T {
	var zero T
	if c == nil {
		return zero
	}
	slice := GetExs[T](c, id)
	if len(slice) == 0 {
		return zero
	}
	return slice[0]
}

func GetExs[T Instruction](c *Cache, ids ...int64) []T {
	if c == nil {
		return nil
	}
	ret := make([]T, 0)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		inst := c.GetInstruction(id)
		v, ok := inst.(T)
		if !ok {
			log.Errorf("BUG::: %v err: %d", inst, id)
			continue
		}
		ret = append(ret, v)
	}
	return ret
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
