package ssaapi

import (
	"github.com/gobwas/glob"
	"regexp"
	"sort"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Program struct {
	Program *ssa.Program
	config  *config
}

func (p *Program) GetNames() []string {
	return []string{p.Program.GetProgramName()}
}

func NewProgram(prog *ssa.Program, config *config) *Program {
	return &Program{
		Program: prog,
		config:  config,
	}
}

func (p *Program) Show() *Program {
	p.Program.Show()
	return p
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

func (g *Program) GlobRefRaw(rule string) Values {
	r, err := glob.Compile(rule)
	if err != nil {
		log.Warnf("GlobRef for %v compile failed: %s", rule, err)
		return nil
	}
	return g.GlobRef(r)
}

func (p *Program) GlobRef(r glob.Glob) Values {
	return lo.FilterMap(
		p.Program.GetInstructionsByGlob(r),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
}

func (p *Program) RegexpRefRaw(rule string) Values {
	r, err := regexp.Compile(rule)
	if err != nil {
		log.Warnf("RegexpRef for %v compile failed: %s", rule, err)
		return nil
	}
	return p.RegexpRef(r)
}

func (p *Program) RegexpRef(r *regexp.Regexp) Values {
	return lo.FilterMap(
		p.Program.GetInstructionsByRegexp(r),
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

func (p *Program) GetAllOffsetItemsBefore(offset int) []*ssa.OffsetItem {
	offsetSortedSlice := p.Program.OffsetSortedSlice
	index := sort.SearchInts(offsetSortedSlice, offset)
	if index < len(offsetSortedSlice) && offsetSortedSlice[index] > offset && index > 0 {
		index--
	}
	beforeSlice := offsetSortedSlice[:index]

	return lo.Filter(
		lo.Map(beforeSlice, func(offset int, _ int) *ssa.OffsetItem {
			return p.Program.OffsetMap[offset]
		}),
		func(v *ssa.OffsetItem, _ int) bool {
			return v.GetVariable() != nil
		},
	)
}
