package sfvm

import (
	"fmt"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

var _ ValueOperator = &ValueList{}

func MergeValues(values ...ValueOperator) ValueOperator {
	return NewValues(values)
}

func NewValues(values []ValueOperator) ValueOperator {
	return &ValueList{values: values}
}

type ValueList struct {
	values []ValueOperator
}

func (v *ValueList) GetCallActualParams() (ValueOperator, error) {
	return nil, utils.Error("list cannot be handled in ValueList.GetCallActualParams")
}

func (v *ValueList) GetSyntaxFlowTopDef() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		topDef, err := v.GetSyntaxFlowTopDef()
		if err != nil {
			return nil, err
		}
		res = append(res, topDef)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetSyntaxFlowBottomUse() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		bottomUse, err := v.GetSyntaxFlowBottomUse()
		if err != nil {
			return nil, err
		}
		res = append(res, bottomUse)
	}
	return NewValues(res), nil
}

func (v *ValueList) ListIndex(i int) (ValueOperator, error) {
	if i < 0 || i >= len(v.values) {
		return nil, utils.Error("index out of range")
	}
	return v.values[i], nil
}

func (v *ValueList) GetName() string {
	return fmt.Sprintf("%v", v.values)
}

func (v *ValueList) IsMap() bool {
	return false
}

func (v *ValueList) IsList() bool {
	return true
}

func (v *ValueList) ExactMatch(s string) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.ExactMatch(s)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			} else {
				res = append(res, value)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}

func (v *ValueList) GlobMatch(s glob.Glob) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.GlobMatch(s)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			} else {
				res = append(res, value)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}

func (v *ValueList) RegexpMatch(regexp *regexp.Regexp) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.RegexpMatch(regexp)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			} else {
				res = append(res, value)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}

func (v *ValueList) GetMembers() (ValueOperator, error) {
	var result []ValueOperator
	for _, k := range v.values {
		result = append(result, k)
	}
	return MergeValues(result...), nil
}
