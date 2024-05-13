package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

var _ sfvm.ValueOperator = new(Values)

func (value Values) GetName() string {
	result := strings.ReplaceAll(value.String(), "\n", "; ")
	return strings.ReplaceAll(result, "\r", "")
}

func (value Values) GetCalled() (sfvm.ValueOperator, error) {
	var vv []sfvm.ValueOperator
	for _, i := range value {
		i, err := i.GetCalled()
		if err != nil {
			continue
		}
		vv = append(vv, i)
	}
	return sfvm.NewValues(vv), nil
}

func (value Values) GetNames() []string {
	var a []string
	for _, i := range value {
		a = append(a, i.GetNames()...)
	}
	return a
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

func (value Values) ExactMatch(s string) (bool, sfvm.ValueOperator, error) {
	var newValue Values
	for _, i := range value {
		for _, name := range i.GetNames() {
			if s == name {
				newValue = append(newValue, i)
			}
		}
	}
	return len(newValue) > 0, newValue, nil
}

func (value Values) GlobMatch(glob sfvm.Glob) (bool, sfvm.ValueOperator, error) {
	var newValue Values
	for _, i := range value {
		for _, name := range i.GetNames() {
			if glob.Match(name) {
				newValue = append(newValue, i)
			}
		}
	}
	return len(newValue) > 0, newValue, nil
}

func (value Values) RegexpMatch(regexp *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	//TODO implement me
	panic("implement me")
}

func (value Values) GetCallActualParams() (sfvm.ValueOperator, error) {
	var vv []sfvm.ValueOperator
	for _, i := range value {
		if i.IsCall() {
			vv = append(vv, i.GetCallArgs())
		}
	}
	return sfvm.NewValues(vv), nil
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
