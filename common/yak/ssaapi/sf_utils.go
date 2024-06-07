package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func _SearchValues(values Values, mod int, handler func(string) bool) Values {
	var newValue Values
	for _, value := range values {
		result := _SearchValue(value, mod, handler)
		newValue = append(newValue, result...)
	}

	return lo.UniqBy(newValue, func(v *Value) int { return int(v.GetId()) })
	// return newValue
}

func _SearchValue(value *Value, mod int, handler func(string) bool) Values {
	var newValue Values
	check := func(value *Value) bool {
		log.Infof("handler: %s(%v)  %s(%v)", value.GetName(), handler(value.GetName()), value.String(), handler(value.String()))
		if handler(value.GetName()) || handler(value.String()) {
			return true
		}

		if value.IsConstInst() && handler(codec.AnyToString(value.GetConstValue())) {
			return true
		}

		for name := range value.GetAllVariables() {
			if handler(name) {
				return true
			}
		}
		return false
	}

	if mod&ssadb.NameMatch != 0 {
		// handler self
		if check(value) {
			newValue = append(newValue, value)
		}
	}
	if mod&ssadb.KeyMatch != 0 {
		if value.IsObject() {
			allMember := value.node.GetAllMember()
			for k, v := range allMember {
				if check(value.NewValue(k)) {
					newValue = append(newValue, value.NewValue(v))
				}
			}
		}
	}
	return newValue
}

func (p *Program) SyntaxFlow(i string, opts ...sfvm.Option) map[string]Values {
	vals, err := p.SyntaxFlowWithError(i, opts...)
	if err != nil {
		log.Warnf("exec syntaxflow: %#v failed: %v", i, err)
		return nil
	}
	return vals
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

func (p *Program) SyntaxFlowChain(i string, opts ...sfvm.Option) Values {
	var results Values
	vals, err := p.SyntaxFlowWithError(i, opts...)
	if err != nil {
		log.Warnf("syntax_flow_chain_failed: %s", err)
	}
	if vals == nil {
		return results
	}
	for _, element := range vals {
		results = append(results, element...)
	}
	return results
}

func (p *Program) SyntaxFlowWithError(i string, opts ...sfvm.Option) (map[string]Values, error) {
	return SyntaxFlowWithError(p, i, opts...)
}

func SyntaxFlowWithError(p sfvm.ValueOperator, i string, opts ...sfvm.Option) (map[string]Values, error) {
	vm := sfvm.NewSyntaxFlowVirtualMachine(opts...)
	err := vm.Compile(i)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow compile %#v failed: %v", i, err)
	}
	results := vm.Feed(p)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow feed %#v failed: %v", i, err)
	}

	// var vals ake([]*Value
	ret := make(map[string]Values)
	for key, v := range results.GetMap() {
		vals, ok := ret[key]
		if !ok {
			vals = make(Values, 0)
		}
		switch ret := v.(type) {
		case *Value:
			vals = append(vals, ret)
		case Values:
			vals = append(vals, ret...)
		case *sfvm.ValueList:
			values, err := SFValueListToValues(ret)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", v, err)
			}
			vals = append(vals, values...)
		default:
			log.Warnf("cannot handle type(raw): %T", i)
		}
		// uniq
		ret[key] = lo.UniqBy(vals, func(v *Value) int { return int(v.GetId()) })
	}
	return ret, nil
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
