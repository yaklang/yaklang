package sfvm

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/pipeline"

	"github.com/yaklang/yaklang/common/utils"
)

type Values []ValueOperator

func NewEmptyValues() Values {
	return Values{}
}
func NewValues(vs ...ValueOperator) Values {
	return Values(vs)
}

type ValueItem struct {
	value ValueOperator
	index int
}

func (v Values) pipeLineRun(f func(ValueOperator) (Values, error)) (Values, error) {
	length := len(v)
	resValue := make([]ValueOperator, length)
	ctx := context.Background()
	pipe := pipeline.NewPipe(ctx, length, func(item ValueItem) (Values, error) {
		value, err := f(item.value)
		if err != nil {
			return nil, err
		}
		return value, nil
	})
	for i, val := range v {
		_ = i
		val.Recursive(func(vo ValueOperator) error {
			pipe.Feed(ValueItem{
				value: vo,
				index: i,
			})
			return nil
		})
	}
	pipe.Close()
	for item := range pipe.Out() {
		// resValue[item.index] = item.value
		resValue = append(resValue, item...)
	}
	return resValue, pipe.Error()
}

// mapValuesWithBool maintains length and applies function that returns (ValueOperator, []bool)
func (v Values) mapValuesWithBool(f func(ValueOperator) (Values, bool)) (Values, []bool) {
	length := len(v)
	resBool := make([]bool, length)
	resValue := make([]ValueOperator, length)
	for i, val := range v {
		_ = i
		if utils.IsNil(val) {
			continue
		}
		val.Recursive(func(vo ValueOperator) error {
			resultValue, result := f(vo)
			resValue = append(resValue, resultValue...)
			resBool = append(resBool, result)
			return nil
		})
	}
	return resValue, resBool
}

func (v *Values) Merge(other ...Values) (Values, error) {
	var res Values
	for _, other := range other {
		res = append(res, other...)
	}
	return res, nil
}

func (v Values) IsEmpty() bool {
	for _, v := range v {
		if utils.IsNil(v) {
			continue
		} else {
			return false
		}
	}
	return true
}

func (v Values) ForEach(f func(ValueOperator) error) error {
	for _, v := range v {
		if err := f(v); err != nil {
			return err
		}
	}
	return nil
}

func (v *Values) Recursive(f func(ValueOperator) error) error {
	return v.ForEach(f)
}

func (vs Values) AppendPredecessor(predecessor ValueOperator, opts ...AnalysisContextOption) error {
	for _, v := range vs {
		if err := v.AppendPredecessor(predecessor, opts...); err != nil {
			return err
		}
	}
	return nil
}
