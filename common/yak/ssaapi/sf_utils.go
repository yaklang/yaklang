package ssaapi

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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
	var newValue Values
	add := func(v *Value) {
		v.AppendPredecessor(value, opt...)
		newValue = append(newValue, v)
	}
	check := func(value *Value) bool {
		if compare(value.GetName()) || compare(value.String()) {
			return true
		}

		if value.IsConstInst() && compare(codec.AnyToString(value.GetConstValue())) {
			return true
		}

		for name := range value.GetAllVariables() {
			if compare(name) {
				return true
			}
		}

		if key := value.GetKey(); key != nil {
			keyName := fmt.Sprint(key.GetConstValue())
			if keyName != "" && compare(keyName) {
				return true
			}
		}

		return false
	}
	if mod&ssadb.ConstType != 0 {
		if check(value) {
			add(value)
		}
	}
	if mod&ssadb.NameMatch != 0 {
		// handler self
		if check(value) {
			add(value)
		}
	}
	if mod&ssadb.KeyMatch != 0 {
		if value.IsObject() {
			allMember := value.node.GetAllMember()
			for k, v := range allMember {
				if check(value.NewValue(k)) {
					add(value.NewValue(v))
				}
			}

		}

		for _, ov := range value.node.FlatOccultation() {
			allMember := ov.GetAllMember()
			for k, v := range allMember {
				if check(value.NewValue(k)) {
					add(value.NewValue(v))
				}
			}
		}
	}

	return newValue
}
func WithSyntaxFlowResult(expected string, handler func(*Value) error) sfvm.Option {
	return sfvm.WithResultCaptured(func(name string, results sfvm.ValueOperator) error {
		if name != expected {
			return nil
		}
		return results.Recursive(func(operator sfvm.ValueOperator) error {
			result, ok := operator.(*Value)
			if !ok {
				return nil
			}
			err := handler(result)
			if err != nil {
				return err
			}
			return nil
		})
	})
}

func WithSyntaxFlowStrictMatch(stricts ...bool) sfvm.Option {
	strict := true
	if len(stricts) > 0 {
		strict = stricts[0]
	}
	return sfvm.WithStrictMatch(strict)
}

func SyntaxFlowVariableToValues(v sfvm.ValueOperator) Values {
	var vals Values
	err := v.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *Value:
			vals = append(vals, ret)
		case Values:
			vals = append(vals, ret...)
		case *sfvm.ValueList:
			values, err := SFValueListToValues(ret)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", operator, err)
			} else {
				vals = append(vals, values...)
			}
		default:
			log.Warnf("cannot handle type: %T", operator)
		}
		return nil
	})
	if err != nil {
		log.Warnf("SyntaxFlowToValues: %v", err)
	}
	return vals
}

func SFValueListToValues(list *sfvm.ValueList) (Values, error) {
	return _SFValueListToValues(0, list)
}

func _SFValueListToValues(count int, list *sfvm.ValueList) (Values, error) {
	if count > 1000 {
		return nil, utils.Errorf("too many nested ValueList: %d", count)
	}
	var vals Values
	list.ForEach(func(i any) {
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
