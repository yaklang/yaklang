package ssaapi

import (
	"regexp"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ sfvm.ValueOperator = new(Values)

func (value Values) GetCalled() (sfvm.ValueOperator, error) {
	var vv Values
	for _, i := range value {
		i, err := i.GetCalled()
		if err != nil {
			continue
		}
		if vs, ok := i.(Values); ok {
			vv = append(vv, vs...)
		} else if v, ok := i.(*Value); ok {
			vv = append(vv, v)
		}
	}
	return vv, nil
}

func (value Values) IsMap() bool {
	return false
}

func (value Values) IsList() bool {
	return true
}

func (value Values) Len() int {
	return len(value)
}

func (values Values) ExactMatch(isMember bool, want string) (bool, sfvm.ValueOperator, error) {
	log.Infof("ExactMatch: %v %v", isMember, want)
	newValue := _SearchValues(values, isMember, func(s string) bool { return s == want })
	return len(newValue) > 0, newValue, nil
}

func (values Values) GlobMatch(isMember bool, glob sfvm.Glob) (bool, sfvm.ValueOperator, error) {
	newValue := _SearchValues(values, isMember, glob.Match)
	return len(newValue) > 0, newValue, nil
}

func (values Values) RegexpMatch(isMember bool, regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	newValue := _SearchValues(values, isMember, regexp.MatchString)
	return len(newValue) > 0, newValue, nil
}

func (value Values) GetCallActualParams(index int) (sfvm.ValueOperator, error) {
	var ret Values
	for _, i := range value {
		if c, ok := ssa.ToCall(i.node); ok {
			if len(c.Args) > index {
				ret = append(ret, NewValue(c.Args[index]))
			}
		}
	}
	if len(ret) == 0 {
		return nil, utils.Errorf("ssa.Values no this argument by index %d", index)
	} else {
		return ret, nil
	}
}

func (value Values) GetAllCallActualParams() (sfvm.ValueOperator, error) {
	var vv Values
	for _, i := range value {
		if i.IsCall() {
			vv = append(vv, i.GetCallArgs()...)
		}
	}
	return vv, nil
}

func (value Values) GetMembersByString(key string) (sfvm.ValueOperator, error) {
	var vals Values
	for _, v := range value {
		if !v.IsObject() {
			continue
		}
		if v.IsMap() || v.IsList() || v.IsObject() {
			res := v.GetMember(NewValue(ssa.NewConst(key)))
			vals = append(vals, res)
		}
	}
	return vals, nil
}

func (value Values) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return value.GetUsers(), nil
}
func (value Values) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return value.GetOperands(), nil
}
func (value Values) GetSyntaxFlowTopDef(config ...*sfvm.ConfigItem) (sfvm.ValueOperator, error) {
	return value.GetTopDefs(), nil
}

func (value Values) GetSyntaxFlowBottomUse(config ...*sfvm.ConfigItem) (sfvm.ValueOperator, error) {
	return value.GetBottomUses(), nil
}

func (value Values) ListIndex(i int) (sfvm.ValueOperator, error) {
	if i < 0 || i >= len(value) {
		return nil, utils.Error("index out of range")
	}
	return value[i], nil
}
