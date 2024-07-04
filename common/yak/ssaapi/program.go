package ssaapi

import (
	"sort"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type Program struct {
	// TODO: one program may have multiple program,
	// 	 	 only one Application and multiple Library
	Program *ssa.Program
	// DBCache *ssa.Cache
	config *config

	// come from database will affect search operation
	comeFromDatabase bool
}

func (p *Program) GetNames() []string {
	return []string{p.Program.GetProgramName()}
}

func (p *Program) GetProgramKind() ssa.ProgramKind {
	return p.Program.ProgramKind
}

func NewProgram(prog *ssa.Program, config *config) *Program {
	p := &Program{
		Program: prog,
		config:  config,
	}

	// if config.DatabaseProgramName == "" {
	// 	p.DBCache = prog.Cache
	// } else {
	// 	p.DBCache = ssa.GetCacheFromPool(config.DatabaseProgramName)
	// }
	return p
}

func (p *Program) DBDebug() {
	if p == nil || p.Program == nil {
		return
	}
	p.Program.Cache.DB = p.Program.Cache.DB.Debug()
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

	return p.NewValue(val), nil
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
		ssa.MatchInstructionByExact(p.Program, ssadb.NameMatch, name),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
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
