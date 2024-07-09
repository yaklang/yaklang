package sfvm

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ ValueOperator = &ValueList{}

func MergeValues(values ...ValueOperator) ValueOperator {
	return NewValues(values)
}

func NewValues(values []ValueOperator) ValueOperator {
	v := &ValueList{values: values}
	vs := make([]ValueOperator, 0, len(values))
	v.Recursive(func(operator ValueOperator) error {
		vs = append(vs, operator)
		return nil
	})
	// flat
	return &ValueList{values: vs}
}

func NewEmptyValues() ValueOperator {
	return NewValues(nil)
}

type ValueList struct {
	values []ValueOperator
}

func (v *ValueList) AppendPredecessor(value ValueOperator, opts ...AnalysisContextOption) error {
	return v.Recursive(func(operator ValueOperator) error {
		return operator.AppendPredecessor(value, opts...)
	})
}

func (v *ValueList) Merge(values ...ValueOperator) (ValueOperator, error) {
	return MergeValues(append(v.values, values...)...), nil
}

func (v *ValueList) Remove(values ...ValueOperator) (ValueOperator, error) {
	var filter = make(map[int64]ValueOperator)
	_ = v.Recursive(func(operator ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			_, existed := filter[raw.GetId()]
			if !existed {
				filter[raw.GetId()] = operator
			}
		}
		return nil
	})
	NewValues(values).Recursive(func(operator ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			delete(filter, raw.GetId())
		}
		return nil
	})
	var res []ValueOperator
	for _, v := range filter {
		res = append(res, v)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetOpcode() string {
	return ""
}

func (v *ValueList) Recursive(f func(operator ValueOperator) error) error {
	if len(v.values) > 0 {
		for _, sub := range v.values {
			err := sub.Recursive(f)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *ValueList) GetCalled() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		called, err := v.GetCalled()
		if err != nil {
			continue
		}
		res = append(res, called)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetFields() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		fields, err := v.GetFields()
		if err != nil {
			continue
		}
		res = append(res, fields)
	}
	return NewValues(res), nil
}

func (v *ValueList) ForEach(h func(i any)) {
	funk.ForEach(v.values, func(i any) {
		h(i)
	})
}

func (v *ValueList) String() string {
	var res []string
	for _, v := range v.values {
		res = append(res, v.String())
	}
	return strings.Join(res, "; ")
}

func (v *ValueList) GetCallActualParams(i int) (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		def, err := v.GetCallActualParams(i)
		if err != nil {
			return nil, err
		}
		res = append(res, def)
	}
	return NewValues(res), nil
}
func (v *ValueList) GetAllCallActualParams() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		def, err := v.GetAllCallActualParams()
		if err != nil {
			return nil, err
		}
		res = append(res, def)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetSyntaxFlowDef() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		def, err := v.GetSyntaxFlowDef()
		if err != nil {
			return nil, err
		}
		res = append(res, def)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetSyntaxFlowUse() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		use, err := v.GetSyntaxFlowUse()
		if err != nil {
			return nil, err
		}
		res = append(res, use)
	}
	return NewValues(res), nil
}
func (v *ValueList) GetSyntaxFlowTopDef(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		topDef, err := v.GetSyntaxFlowTopDef(sfResult, sfConfig, config...)
		if err != nil {
			return nil, err
		}
		res = append(res, topDef)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetSyntaxFlowBottomUse(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.values {
		bottomUse, err := v.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
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

func (v *ValueList) IsMap() bool {
	return false
}

func (v *ValueList) IsList() bool {
	return true
}

func (v *ValueList) ExactMatch(mod int, s string) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.ExactMatch(mod, s)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}

func (v *ValueList) GlobMatch(mod int, s ssa.Glob) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.GlobMatch(mod, s)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}

func (v *ValueList) RegexpMatch(mod int, regexp *regexp.Regexp) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.values {
		match, next, err := value.RegexpMatch(mod, regexp)
		if err != nil {
			return false, nil, err
		}
		if match {
			if next != nil {
				res = append(res, next)
			}
		}
	}
	return len(res) > 0, NewValues(res), nil
}
