package ssaapi

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

var _ sfvm.ValueOperator = &Program{}

func (p *Program) GetName() string {
	return p.Program.GetProgramName()
}

func (p *Program) IsMap() bool { return false }

func (p *Program) IsList() bool {
	//TODO implement me
	return false
}

func (p *Program) ExactMatch(s string) (bool, sfvm.ValueOperator, error) {
	vals := p.Ref(s)
	if len(vals) > 0 {
		return true, nil, nil
	}
	return false, nil, nil
}

func (p *Program) GlobMatch(glob glob.Glob) (bool, sfvm.ValueOperator, error) {
	return false, nil, utils.Error("ssa.Program is not supported glob match")
}

func (p *Program) RegexpMatch(regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	return false, nil, utils.Error("ssa.Program is not supported regexp match")
}

func (p *Program) GetMembers() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported get members")
}

func (p *Program) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported list index")
}

func (p *Program) GetCallActualParams() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported call actual params")
}

func (p *Program) GetSyntaxFlowTopDef() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}
