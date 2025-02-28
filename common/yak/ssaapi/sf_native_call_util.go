package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var nativeCallString = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	if isProgram(v) {
		return false, nil, utils.Error("string is not supported in program")
	}

	var vals []sfvm.ValueOperator
	v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}

		if val.IsConstInst() {
			vals = append(vals, val)
			return nil
		}

		results := val.NewValue(ssa.NewConstWithRange(val.String(), val.GetRange()))
		results.AppendPredecessor(val, frame.WithPredecessorContext("string"))
		vals = append(vals, results)
		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
}

var nativeCallStrLower = sfvm.NativeCallFunc(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if val.IsConstInst() {
			ss := codec.AnyToString(val.GetConstValue())
			results := val.NewValue(ssa.NewConstWithRange(strings.ToLower(ss), val.GetRange()))
			results.AppendPredecessor(val, frame.WithPredecessorContext("str-lower"))
			vals = append(vals, results)
			return nil
		}
		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
})

var nativeCallStrUpper = sfvm.NativeCallFunc(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	var vals []sfvm.ValueOperator
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if val.IsConstInst() {
			ss := codec.AnyToString(val.GetConstValue())
			results := val.NewValue(ssa.NewConstWithRange(strings.ToUpper(ss), val.GetRange()))
			results.AppendPredecessor(val, frame.WithPredecessorContext("str-upper"))
			vals = append(vals, results)
			return nil
		}
		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
})

var nativeCallRegexp = sfvm.NativeCallFunc(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
	if isProgram(v) {
		return false, nil, utils.Error("regexp is not supported in program")
	}

	rules := params.GetString(0, "rule", "pattern")
	groupRaw := params.GetString("group", "groups", "capture")
	var groupInt []int
	if groupRaw != "" {
		groupInt = utils.ParseStringToInts(groupRaw)
	}

	re, err := regexp.Compile(rules)
	if err != nil {
		return false, nil, err
	}

	var vals []string
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}

		if val.IsConstInst() {
			vals = append(vals, codec.AnyToString(val.GetConstValue()))
			return nil
		}

		next, calls, _ := nativeCallString(val, frame, nil)
		if next {
			_ = calls.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				if val.IsConstInst() {
					vals = append(vals, codec.AnyToString(val.GetConstValue()))
				}
				return nil
			})
		}
		return nil
	})

	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var results []sfvm.ValueOperator
	for _, raw := range vals {
		for _, i := range re.FindAllStringSubmatch(raw, -1) {
			if len(groupInt) > 0 {
				for _, j := range groupInt {
					if j >= 0 && j < len(i) {
						ret := prog.NewValue(ssa.NewConst(i[j]))
						_ = ret.AppendPredecessor(v, frame.WithPredecessorContext("regexp group"))
						results = append(results, ret)
					}
				}
			} else {
				if len(i) > 0 {
					ret := prog.NewValue(ssa.NewConst(i[0]))
					_ = ret.AppendPredecessor(v, frame.WithPredecessorContext("regexp"))
					results = append(results, ret)
				}
			}
		}
	}
	if len(results) > 0 {
		return true, sfvm.NewValues(results), nil
	}
	return false, nil, utils.Error("no value found")
})
