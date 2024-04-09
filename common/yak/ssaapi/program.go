package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Program struct {
	Program *ssa.Program
	config  *config
}

func NewProgram(prog *ssa.Program) *Program {
	return &Program{
		Program: prog,
	}
}

func (p *Program) Show() *Program {
	p.Program.Show()
	return p
}
func (p *Program) AddConfig(c *config) {
	p.config = c
}

func (p *Program) IsNil() bool {
	return utils.IsNil(p) || utils.IsNil(p.Program)
}

func (p *Program) GetErrors() ssa.SSAErrors {
	return p.Program.GetErrors()
}

func (p *Program) GetValueById(id int64) (*Value, error) {
	val, ok := p.Program.GetInstructionById(id).(ssa.Value)
	if val == nil {
		return nil, utils.Errorf("instruction not found: %d", id)
	}
	if !ok {
		return nil, utils.Errorf("[%T] not an instruction node", val)
	}

	return NewValue(val), nil
}

func (p *Program) GetValueByIdMust(id int64) *Value {
	v, err := p.GetValueById(id)
	if err != nil {
		log.Errorf("GetValueByIdMust: %v", err)
	}
	return v
}

func (p *Program) GetInstructionById(id int64) ssa.Instruction {
	return p.Program.GetInstructionById(id)
}

func (p *Program) Ref(name string) Values {
	return lo.FilterMap(
		p.Program.GetInstructionsByName(name),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
}

func (p *Program) GetClassMember(className string, key string) *Value {
	if class, ok := p.Program.ClassBluePrint[className]; ok {
		if method, ok := class.Method[key]; ok {
			return NewValue(method)
		}
		if member, ok := class.NormalMember[key]; ok {
			return NewValue(member.Value)
		}
		if member, ok := class.StaticMember[key]; ok {
			return NewValue(member)
		}
	}

	return nil
}

func (p *Program) GetAllSymbols() map[string]Values {
	ret := make(map[string]Values, 0)
	p.Program.Cache.ForEachVariable(func(s string, insts []ssa.Instruction) {
		ret[s] = lo.FilterMap(insts, func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return NewValue(v), true
			}
			return nil, false
		})
	})
	return ret
}
