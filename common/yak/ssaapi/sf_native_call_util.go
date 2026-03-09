package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func mergeAnchorBitVectorToResult(result sfvm.Values, source sfvm.ValueOperator) {
	sfvm.MergeAnchorBitVectorToResult(result, source)
}

var nativeCallString = func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
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

		results := val.NewConstValue(val.String(), val.GetRange())
		results.AppendPredecessor(val, frame.WithPredecessorContext("string"))
		mergeAnchorBitVectorToResult(sfvm.ValuesOf(results), val)
		vals = append(vals, results)
		return nil
	})
	if len(vals) > 0 {
		return true, sfvm.NewValues(vals), nil
	}
	return false, nil, utils.Error("no value found")
}

var nativeCallStrLower = sfvm.NativeCallFunc(func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	var vals []sfvm.ValueOperator
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if val.IsConstInst() {
			ss := codec.AnyToString(val.GetConstValue())
			results := val.NewConstValue(strings.ToLower(ss), val.GetRange())
			results.AppendPredecessor(val, frame.WithPredecessorContext("str-lower"))
			mergeAnchorBitVectorToResult(sfvm.ValuesOf(results), val)
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

var nativeCallStrUpper = sfvm.NativeCallFunc(func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	var vals []sfvm.ValueOperator
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if val.IsConstInst() {
			ss := codec.AnyToString(val.GetConstValue())
			results := val.NewConstValue(strings.ToUpper(ss), val.GetRange())
			results.AppendPredecessor(val, frame.WithPredecessorContext("str-upper"))
			mergeAnchorBitVectorToResult(sfvm.ValuesOf(results), val)
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

var nativeCallRegexp = sfvm.NativeCallFunc(func(v sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
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

	prog, err := fetchProgram(v)
	if err != nil {
		return false, nil, err
	}

	var results []sfvm.ValueOperator
	_ = v.Recursive(func(operator sfvm.ValueOperator) error {
		val, ok := operator.(*Value)
		if !ok {
			return nil
		}

		var raws []string
		if val.IsConstInst() {
			raws = append(raws, codec.AnyToString(val.GetConstValue()))
		} else {
			next, calls, _ := nativeCallString(sfvm.ValuesOf(val), frame, nil)
			if next {
				_ = calls.Recursive(func(op sfvm.ValueOperator) error {
					rawVal, ok := op.(*Value)
					if !ok || !rawVal.IsConstInst() {
						return nil
					}
					raws = append(raws, codec.AnyToString(rawVal.GetConstValue()))
					return nil
				})
			}
		}

		for _, raw := range raws {
			for _, matched := range re.FindAllStringSubmatch(raw, -1) {
				if len(groupInt) > 0 {
					for _, group := range groupInt {
						if group < 0 || group >= len(matched) {
							continue
						}
						ret := prog.NewConstValue(matched[group])
						_ = ret.AppendPredecessor(val, frame.WithPredecessorContext("regexp group"))
						mergeAnchorBitVectorToResult(sfvm.ValuesOf(ret), val)
						results = append(results, ret)
					}
					continue
				}
				if len(matched) == 0 {
					continue
				}
				ret := prog.NewConstValue(matched[0])
				_ = ret.AppendPredecessor(val, frame.WithPredecessorContext("regexp"))
				mergeAnchorBitVectorToResult(sfvm.ValuesOf(ret), val)
				results = append(results, ret)
			}
		}
		return nil
	})
	if len(results) > 0 {
		return true, sfvm.NewValues(results), nil
	}
	return false, nil, utils.Error("no value found")
})
