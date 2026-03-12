package ssaapi

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var nativeCallString = sfvm.ValueSingleNativeCall(func(operator sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (sfvm.Values, error) {
	val, ok := operator.(*Value)
	if !ok {
		return sfvm.NewEmptyValues(), nil
	}
	if val.IsConstInst() {
		return sfvm.ValuesOf(val), nil
	}
	result := val.NewConstValue(val.String(), val.GetRange())
	result.AppendPredecessor(val, frame.WithPredecessorContext("string"))
	return sfvm.ValuesOf(result), nil
})

var nativeCallStrLower = sfvm.ValueSingleNativeCall(func(operator sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (sfvm.Values, error) {
	val, ok := operator.(*Value)
	if !ok || !val.IsConstInst() {
		return sfvm.NewEmptyValues(), nil
	}
	ss := codec.AnyToString(val.GetConstValue())
	result := val.NewConstValue(strings.ToLower(ss), val.GetRange())
	result.AppendPredecessor(val, frame.WithPredecessorContext("str-lower"))
	return sfvm.ValuesOf(result), nil
})

var nativeCallStrUpper = sfvm.ValueSingleNativeCall(func(operator sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (sfvm.Values, error) {
	val, ok := operator.(*Value)
	if !ok || !val.IsConstInst() {
		return sfvm.NewEmptyValues(), nil
	}
	ss := codec.AnyToString(val.GetConstValue())
	result := val.NewConstValue(strings.ToUpper(ss), val.GetRange())
	result.AppendPredecessor(val, frame.WithPredecessorContext("str-upper"))
	return sfvm.ValuesOf(result), nil
})

var nativeCallRegexp = sfvm.ValueSingleNativeCall(func(operator sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (sfvm.Values, error) {
	val, ok := operator.(*Value)
	if !ok {
		return sfvm.NewEmptyValues(), nil
	}
	rules := params.GetString(0, "rule", "pattern")
	groupRaw := params.GetString("group", "groups", "capture")
	var groupInt []int
	if groupRaw != "" {
		groupInt = utils.ParseStringToInts(groupRaw)
	}

	re, err := regexp.Compile(rules)
	if err != nil {
		return nil, err
	}

	prog := val.ParentProgram
	if prog == nil {
		return sfvm.NewEmptyValues(), nil
	}

	raws := make([]string, 0, 1)
	if val.IsConstInst() {
		raws = append(raws, codec.AnyToString(val.GetConstValue()))
	} else {
		raws = append(raws, val.String())
	}

	results := make([]sfvm.ValueOperator, 0)
	for _, raw := range raws {
		for _, matched := range re.FindAllStringSubmatch(raw, -1) {
			if len(groupInt) > 0 {
				for _, group := range groupInt {
					if group < 0 || group >= len(matched) {
						continue
					}
					ret := prog.NewConstValue(matched[group])
					_ = ret.AppendPredecessor(val, frame.WithPredecessorContext("regexp group"))
					results = append(results, ret)
				}
				continue
			}
			if len(matched) == 0 {
				continue
			}
			ret := prog.NewConstValue(matched[0])
			_ = ret.AppendPredecessor(val, frame.WithPredecessorContext("regexp"))
			results = append(results, ret)
		}
	}
	return sfvm.NewValues(results), nil
})
