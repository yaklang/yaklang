package ssaapi

import (
	"regexp"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ sfvm.ValueOperator = &Program{}

func (p *Program) String() string {
	return p.Program.GetProgramName()
}

func (p *Program) IsMap() bool { return false }

func (p *Program) IsList() bool {
	//TODO implement me
	return false
}

func (p *Program) GetOpcode() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (p *Program) Recursive(func(operator sfvm.ValueOperator) error) error {
	return nil
}

func (p *Program) ExactMatch(mod int, s string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		p.DBCache.GetByVariableExact(mod, s),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) GlobMatch(mod int, g ssa.Glob) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		p.DBCache.GetByVariableGlob(mod, g),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			}
			return nil, false
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) RegexpMatch(mod int, re *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		p.DBCache.GetByVariableRegexp(mod, re),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported list index")
}

func (p *Program) GetAllCallActualParams() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported call actual params")
}
func (p *Program) GetCallActualParams(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported call all actual params")
}

func (p *Program) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow def")
}
func (p *Program) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow use")
}
func (p *Program) GetSyntaxFlowTopDef(config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse(config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}

func (p *Program) GetCalled() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported called")
}
