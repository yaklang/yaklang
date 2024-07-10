package ssaapi

import (
	"io/fs"

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

func (p *Program) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	// return nil will not change the predecessor
	// no not return any error here!!!!!
	return nil
}

func (p *Program) GetFields() (sfvm.ValueOperator, error) {
	return sfvm.NewValues(nil), nil
}

func (p *Program) IsList() bool {
	//TODO implement me
	return false
}

func (p *Program) GetOpcode() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (p *Program) Recursive(f func(operator sfvm.ValueOperator) error) error {
	return f(p)
}

func (p *Program) ExactMatch(mod int, s string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByExact(p.Program, mod, s),
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

func (p *Program) GlobMatch(mod int, g string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByGlob(p.Program, mod, g),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			}
			return nil, false
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) RegexpMatch(mod int, re string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByRegexp(p.Program, mod, re),
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

func (p *Program) Merge(...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported merge")
}

func (p *Program) Remove(...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported remove")
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
func (p *Program) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}

func (p *Program) GetCalled() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported called")
}

func (p *Program) FileFilter(fs.File, string, map[string]string, []string) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported file filter")
}
