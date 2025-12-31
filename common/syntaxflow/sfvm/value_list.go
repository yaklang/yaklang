package sfvm

import (
	"context"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/pipeline"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Values []ValueOperator

func NewFlatValues(vs ...ValueOperator) Values {
	ret := make([]ValueOperator, 0, len(vs))
	for _, v := range vs {
		v.Recursive(func(vo ValueOperator) error {
			ret = append(ret, vo)
			return nil
		})
	}
	return vs
}

func (vs Values) ToList() *ValueList {
	return &ValueList{vs}
}

func (v Values) pipeLineRun(f func(ValueOperator) (ValueOperator, error)) (*ValueList, error) {
	length := len(v)
	resValue := make([]ValueOperator, length)
	ctx := context.Background()
	pipe := pipeline.NewPipe(ctx, length, func(val ValueItem) (ValueItem, error) {
		value, err := f(val.value)
		if err != nil {
			return ValueItem{}, err
		}
		val.value = value // update
		return val, nil
	})
	for i, val := range v {
		pipe.Feed(ValueItem{
			value: val,
			index: i,
		})
	}
	pipe.Close()
	for item := range pipe.Out() {
		resValue[item.index] = item.value
	}
	return NewValueList(resValue), pipe.Error()
}

type ValueList struct {
	Values
}

var _ ValueOperator = (*ValueList)(nil)

func NewValueList(values []ValueOperator) *ValueList {
	return &ValueList{Values: values}
}

func (v *ValueList) IsEmpty() bool {
	if v == nil || len(v.Values) == 0 {
		return true
	}
	return false
}

func NewEmptyValues() *ValueList {
	return &ValueList{Values: nil}
}

func (v *ValueList) Count() int {
	return len(v.Values)
}

type ValueItem struct {
	value ValueOperator
	index int
}

// mapValuesWithBool maintains length and applies function that returns (ValueOperator, []bool)
func (v *ValueList) mapValuesWithBool(f func(ValueOperator) (ValueOperator, bool)) (*ValueList, []bool) {
	length := len(v.Values)
	resBool := make([]bool, length)
	resValue := make([]ValueOperator, length)
	for i, val := range v.Values {
		resultValue, result := f(val)
		resBool[i] = result
		resValue[i] = resultValue
	}
	return NewValueList(resValue), resBool
}

func (v *ValueList) NewConst(i any, rng ...*memedit.Range) ValueOperator {
	result, _ := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.NewConst(i, rng...), nil
	})
	return result
}

func (v *ValueList) CompareConst(comparator *ConstComparator) bool {
	var res bool
	res = false
	for _, operator := range v.Values {
		result := operator.CompareConst(comparator)
		res = res || result
	}
	return res
}

func (v *ValueList) CompareOpcode(comparator *OpcodeComparator) (ValueOperator, bool) {
	val, condition := v.mapValuesWithBool(func(vo ValueOperator) (ValueOperator, bool) {
		return vo.CompareOpcode(comparator)
	})
	return val, slices.Contains(condition, true)
}

func (v *ValueList) CompareString(comparator *StringComparator) (ValueOperator, bool) {
	val, condition := v.mapValuesWithBool(func(vo ValueOperator) (ValueOperator, bool) {
		return vo.CompareString(comparator)
	})
	return val, slices.Contains(condition, true)
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
	// maintain length - collect from current list
	var res []ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		res = append(res, operator)
		return nil
	})
	// collect from values to merge
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
	// maintain length - collect from current list
	length := len(v.Values)
	for i := 0; i < length; i++ {
		if raw, ok := v.Values[i].(ssa.GetIdIF); ok {
			_, existed := filter[raw.GetId()]
			if !existed {
				filter[raw.GetId()] = v.Values[i]
			}
		}
	}
	NewValueList(values).Recursive(func(operator ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			delete(filter, raw.GetId())
		}
		return nil
	})
	var res []ValueOperator
	for _, val := range filter {
		res = append(res, val)
	}
	return NewValueList(res), nil
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
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetCalled()
	})
}

func (v *ValueList) GetFields() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetFields()
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
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetCallActualParams(i, b)
	})
}

func (v *ValueList) GetSyntaxFlowDef() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowDef()
	})
}

func (v *ValueList) GetSyntaxFlowUse() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowUse()
	})
}

func (v *ValueList) GetSyntaxFlowTopDef(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowTopDef(sfResult, sfConfig, config...)
	})
}

func (v *ValueList) GetSyntaxFlowBottomUse(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
	})
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

func (v *ValueList) ExactMatch(ctx context.Context, mod int, s string) ValueOperator {
	ret, _ := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		nextValue := vo.ExactMatch(ctx, mod, s)
		return nextValue, nil
	})
	return ret
}

func (v *ValueList) GlobMatch(ctx context.Context, mod int, s string) ValueOperator {
	ret, _ := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		nextValue := vo.GlobMatch(ctx, mod, s)
		return nextValue, nil
	})
	return ret
}

func (v *ValueList) RegexpMatch(ctx context.Context, mod int, regexp string) ValueOperator {
	ret, _ := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		nextValue := vo.RegexpMatch(ctx, mod, regexp)
		return nextValue, nil
	})
	return ret
}

func (v *ValueList) FileFilter(path string, mode string, rule1 map[string]string, rule2 []string) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.FileFilter(path, mode, rule1, rule2)
	})
}

func (v *ValueList) Foreach(h func(value ValueOperator)) {

}
