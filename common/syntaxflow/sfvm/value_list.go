package sfvm

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"strings"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ ValueOperator = (*ValueList)(nil)

func NewValues(values []ValueOperator) ValueOperator {
	zero := NewEmptyValues()
	ret, err := zero.Merge(values...)
	if err != nil {
		return zero
	}
	return ret
}

func (v *ValueList) IsEmpty() bool {
	return ValuesLen(v) == 0
}

func NewEmptyValues() ValueOperator {
	return &ValueList{Values: nil}
}

type ValueList struct {
	Values []ValueOperator
}

func (v *ValueList) CompareConst(comparator *ConstComparator) []bool {
	var res []bool
	v.Recursive(func(operator ValueOperator) error {
		result := operator.CompareConst(comparator)
		res = append(res, result...)
		return nil
	})
	return res
}

func (v *ValueList) NewConst(i any, rng ...memedit.RangeIf) ValueOperator {
	var result ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		result = operator.NewConst(i, rng...)
		return nil
	})
	return result
}

func (v *ValueList) CompareOpcode(comparator *OpcodeComparator) (ValueOperator, []bool) {
	var res []bool
	v.Recursive(func(operator ValueOperator) error {
		_, result := operator.CompareOpcode(comparator)
		res = append(res, result...)
		return nil
	})
	return nil, res
}

func (v *ValueList) CompareString(comparator *StringComparator) (ValueOperator, []bool) {
	var res []bool
	v.Recursive(func(operator ValueOperator) error {
		_, result := operator.CompareString(comparator)
		res = append(res, result...)
		return nil
	})
	return nil, res
}

func (v *ValueList) AppendPredecessor(value ValueOperator, opts ...AnalysisContextOption) error {
	return v.Recursive(func(operator ValueOperator) error {
		return operator.AppendPredecessor(value, opts...)
	})
}

func (v *ValueList) Merge(values ...ValueOperator) (ValueOperator, error) {
	if v.IsEmpty() && len(values) == 0 {
		return NewEmptyValues(), nil
	}
	var res []ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		res = append(res, operator)
		return nil
	})
	for _, value := range values {
		if utils.IsNil(value) {
			continue
		}
		value.Recursive(func(vo ValueOperator) error {
			res = append(res, vo)
			return nil
		})
	}
	if len(res) > 1 {
		if ret, err := res[0].Merge(res[1:]...); err == nil {
			return ret, nil
		} else {
			return &ValueList{Values: res}, nil
		}
	} else if len(res) == 1 {
		return res[0], nil
	} else {
		return NewEmptyValues(), nil
	}
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
	if v == nil {
		return utils.Errorf("value is nil")
	}
	for _, sub := range v.Values {
		err := sub.Recursive(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *ValueList) GetCalled() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.Values {
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
	for _, v := range v.Values {
		fields, err := v.GetFields()
		if err != nil {
			continue
		}
		res = append(res, fields)
	}
	return NewValues(res), nil
}

func (v *ValueList) ForEach(h func(i any)) {
	funk.ForEach(v.Values, func(i any) {
		h(i)
	})
}

func (v *ValueList) GetBinaryOperator() string {
	return ""
}

func (v *ValueList) GetUnaryOperator() string {
	return ""
}

func (v *ValueList) String() string {
	var res []string
	for _, v := range v.Values {
		res = append(res, v.String())
	}
	return strings.Join(res, "; ")
}

func (v *ValueList) GetCallActualParams(i int, b bool) (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.Values {
		def, err := v.GetCallActualParams(i, b)
		if err != nil {
			return nil, err
		}
		res = append(res, def)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetSyntaxFlowDef() (ValueOperator, error) {
	var res []ValueOperator
	for _, v := range v.Values {
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
	for _, v := range v.Values {
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
	for _, v := range v.Values {
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
	for _, v := range v.Values {
		bottomUse, err := v.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
		if err != nil {
			return nil, err
		}
		res = append(res, bottomUse)
	}
	return NewValues(res), nil
}

func (v *ValueList) ListIndex(i int) (ValueOperator, error) {
	if i < 0 || i >= len(v.Values) {
		return nil, utils.Error("index out of range")
	}
	return v.Values[i], nil
}

func (v *ValueList) IsMap() bool {
	return false
}

func (v *ValueList) IsList() bool {
	return true
}

func (v *ValueList) ExactMatch(ctx context.Context, mod int, s string) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.Values {
		match, next, err := value.ExactMatch(ctx, mod, s)
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

func (v *ValueList) GlobMatch(ctx context.Context, mod int, s string) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.Values {
		match, next, err := value.GlobMatch(ctx, mod, s)
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

func (v *ValueList) RegexpMatch(ctx context.Context, mod int, regexp string) (bool, ValueOperator, error) {
	var res []ValueOperator
	for _, value := range v.Values {
		match, next, err := value.RegexpMatch(ctx, mod, regexp)
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

func (v *ValueList) FileFilter(path string, mode string, rule1 map[string]string, rule2 []string) (ValueOperator, error) {
	var res []ValueOperator
	var errs error
	for _, value := range v.Values {
		filtered, err := value.FileFilter(path, mode, rule1, rule2)
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			// return nil, err
			continue
		}
		if filtered != nil {
			res = append(res, filtered)
		}
	}
	if errs != nil {
		return nil, errs
	}
	return NewValues(res), nil
}
