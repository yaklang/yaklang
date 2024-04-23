package ssaapi

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

var _ sfvm.ValueOperator = new(Values)

func (value Values) GetName() string {
	return value.String()
}

func (value Values) IsMap() bool {
	return false
}

func (value Values) IsList() bool {
	return true
}

func (value Values) ExactMatch(s string) (bool, sfvm.ValueOperator, error) {
	vals := value.Ref(s)
	if len(vals) > 0 {
		return true, vals, nil
	}
	return false, nil, nil
}

func (value Values) GlobMatch(glob glob.Glob) (bool, sfvm.ValueOperator, error) {
	//TODO implement me
	panic("implement me")
}

func (value Values) RegexpMatch(regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	//TODO implement me
	panic("implement me")
}

func (value Values) GetCallActualParams() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Values is not supported call actual params")
}

func (value Values) GetMembers() (sfvm.ValueOperator, error) {
	var vals Values
	for _, v := range value {
		if v.IsObject() {
			vals = append(vals, v.GetAllMember()...)
		}
	}
	return vals, nil
}

func (value Values) GetSyntaxFlowTopDef() (sfvm.ValueOperator, error) {
	return value.GetTopDefs(), nil
}

func (value Values) GetSyntaxFlowBottomUse() (sfvm.ValueOperator, error) {
	return value.GetBottomUses(), nil
}

func (value Values) ListIndex(i int) (sfvm.ValueOperator, error) {
	if i < 0 || i >= len(value) {
		return nil, utils.Error("index out of range")
	}
	return value[i], nil
}
