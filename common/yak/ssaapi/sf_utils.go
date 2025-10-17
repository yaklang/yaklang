package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

func _SearchValues(values Values, mod int, handler func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	for _, value := range values {
		result := _SearchValue(value, mod, handler, opt...)
		newValue = append(newValue, result...)
	}

	return lo.UniqBy(newValue, func(v *Value) int { return int(v.GetId()) })
	// return newValue
}

func _SearchValue(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	newValue := make([]*Value, 0)
	newValue = append(newValue, SearchWithValue(value, mod, compare, opt...)...)
	newValue = append(newValue, SearchWithCFG(value, mod, compare, opt...)...)
	return newValue
}

func _SearchValuesByOpcode(values Values, opcode string, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	if values.IsEmpty() {
		return newValue
	}

	for _, value := range values {
		if value.GetOpcode() == opcode {
			value.AppendPredecessor(value, opt...)
			newValue = append(newValue, value)
		}
	}
	return newValue
}

func SyntaxFlowVariableToValues(vs ...sfvm.ValueOperator) Values {
	var rets Values
	for _, v := range vs {
		if utils.IsNil(v) {
			continue
		}
		err := v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				rets = append(rets, ret)
			case Values:
				rets = append(rets, ret...)
			case *sfvm.ValueList:
				values, err := SFValueListToValues(ret)
				if err != nil {
					log.Warnf("cannot handle type: %T error: %v", operator, err)
				} else {
					rets = append(rets, values...)
				}
			default:
				log.Warnf("cannot handle type: %T", operator)
			}
			return nil
		})
		if err != nil {
			log.Warnf("SyntaxFlowToValues: %v", err)
		}
	}
	return rets
}

func SFValueListToValues(list *sfvm.ValueList) (Values, error) {
	return _SFValueListToValues(0, list)
}

func _SFValueListToValues(count int, list *sfvm.ValueList) (Values, error) {
	if count > 1000 {
		return nil, utils.Errorf("too many nested ValueList: %d", count)
	}
	var vals Values
	list.Recursive(func(i sfvm.ValueOperator) error {
		switch element := i.(type) {
		case *Value:
			vals = append(vals, element)
		case Values:
			vals = append(vals, element...)
		case *sfvm.ValueList:
			ret, err := _SFValueListToValues(count+1, element)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", i, err)
			} else {
				vals = append(vals, ret...)
			}
		default:
			log.Warnf("cannot handle type: %T", i)
		}
		return nil
	})
	return vals, nil
}

func ValuesToSFValueList(values Values) sfvm.ValueOperator {
	var list []sfvm.ValueOperator
	for _, value := range values {
		list = append(list, value)
	}
	return sfvm.NewValues(list)
}

func MergeSFValueOperator(sfv ...sfvm.ValueOperator) sfvm.ValueOperator {
	ret := []sfvm.ValueOperator{}
	values := make(Values, 0)
	for _, item := range sfv {
		item.Recursive(func(vo sfvm.ValueOperator) error {
			switch v := vo.(type) {
			case *Program:
				ret = append(ret, v)
			case *Value:
				values = append(values, v)
			}
			return nil
		})
	}
	for _, v := range MergeValues(values) {
		ret = append(ret, v)
	}
	return &sfvm.ValueList{Values: ret}
}
