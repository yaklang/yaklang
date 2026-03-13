package sfvm

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type NativeCallActualParams struct {
	m map[string]any
}

func NewNativeCallActualParams(items ...*RecursiveConfigItem) *NativeCallActualParams {
	n := &NativeCallActualParams{
		m: make(map[string]any),
	}
	for _, i := range items {
		n.m[i.Key] = i.Value
	}
	return n
}

func (n *NativeCallActualParams) Existed(index any) bool {
	_, ok := n.m[codec.AnyToString(index)]
	return ok
}

func (n *NativeCallActualParams) GetString(index any, extra ...any) string {
	if n == nil {
		return ""
	}

	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.AnyToString(raw)
	}

	for _, name := range extra {
		raw, ok = n.m[codec.AnyToString(name)]
		if ok {
			return codec.AnyToString(raw)
		}
	}

	return ""
}

func (n *NativeCallActualParams) GetInt(index any, extra ...any) int {
	if n == nil {
		return -1
	}
	raw, ok := n.m[codec.AnyToString(index)]
	if ok {
		return codec.Atoi(codec.AnyToString(raw))
	}

	for _, name := range extra {
		raw, ok := n.m[codec.AnyToString(name)]
		if ok {
			return codec.Atoi(codec.AnyToString(raw))
		}
	}
	return -1
}

type NativeCallFunc func(v Values, frame *SFFrame, params *NativeCallActualParams) (bool, Values, error)

type ValueSingleNativeCallFunc func(operator ValueOperator, frame *SFFrame, params *NativeCallActualParams) (Values, error)
type ValuesNativeCallFunc func(group Values, template ValueOperator, frame *SFFrame, params *NativeCallActualParams) (Values, error)

var nativeCallTable = map[string]NativeCallFunc{}

func RegisterNativeCall(name string, f NativeCallFunc) {
	nativeCallTable[name] = f
}

func GetNativeCall(name string) (NativeCallFunc, error) {
	if f, ok := nativeCallTable[name]; ok {
		return f, nil
	}
	return nil, utils.Wrap(CriticalError, "native call not found: "+name)
}

type nativeCallGroup struct {
	index      int
	values     Values
	template   ValueOperator
	anchorBits *utils.BitVector
}

type nativeCallGroupResult struct {
	index  int
	values Values
}

func firstValue(values Values) ValueOperator {
	value, _ := values.First()
	return value
}

func nativeCallGroups(values Values, frame *SFFrame) []nativeCallGroup {
	scope, ok := frame.activeAnchorScope()
	if !ok || scope.anchorWidth <= 0 {
		template := firstValue(values)
		return []nativeCallGroup{{
			index:      0,
			values:     values,
			template:   template,
			anchorBits: nil,
		}}
	}

	grouped := make([]Values, scope.anchorWidth)
	_ = values.Recursive(func(operator ValueOperator) error {
		forEachAnchorIndexInScope(operator, scope.anchorBase, scope.anchorWidth, func(rel int) {
			grouped[rel] = append(grouped[rel], operator)
		})
		return nil
	})

	groups := make([]nativeCallGroup, 0, scope.anchorWidth)
	for idx := 0; idx < scope.anchorWidth; idx++ {
		var template ValueOperator
		if idx < len(scope.source) {
			template = scope.source[idx]
		}
		if utils.IsNil(template) {
			template = firstValue(grouped[idx])
		}

		var anchorBits *utils.BitVector
		if idx < len(scope.slotAnchorBits) {
			anchorBits = scope.slotAnchorBits[idx]
		}
		groups = append(groups, nativeCallGroup{
			index:      idx,
			values:     grouped[idx],
			template:   template,
			anchorBits: anchorBits,
		})
	}
	return groups
}

func ValueSingleNativeCall(f ValueSingleNativeCallFunc) NativeCallFunc {
	return func(v Values, frame *SFFrame, params *NativeCallActualParams) (bool, Values, error) {
		results, err := RunValueOperatorPipeline(v, ValuePipelineOptions{Frame: frame}, func(operator ValueOperator) (Values, error) {
			return f(operator, frame, params)
		})
		if err != nil {
			return false, nil, err
		}
		if results.IsEmpty() {
			return false, nil, utils.Error("no value found")
		}
		return true, results, nil
	}
}

func ValuesNativeCall(f ValuesNativeCallFunc) NativeCallFunc {
	return func(v Values, frame *SFFrame, params *NativeCallActualParams) (bool, Values, error) {
		groups := nativeCallGroups(v, frame)
		if len(groups) == 0 {
			return false, nil, utils.Error("no value found")
		}

		ctx := context.Background()
		if frame != nil {
			ctx = frame.GetContext()
		}

		pipe := pipeline.NewPipe(ctx, len(groups), func(group nativeCallGroup) (nativeCallGroupResult, error) {
			values, err := f(group.values, group.template, frame, params)
			if err != nil {
				return nativeCallGroupResult{}, err
			}
			if group.anchorBits != nil && !group.anchorBits.IsEmpty() {
				for _, value := range values {
					mergeAnchorBits(value, group.anchorBits)
				}
			}
			return nativeCallGroupResult{index: group.index, values: values}, nil
		})
		for _, group := range groups {
			pipe.Feed(group)
		}
		pipe.Close()

		results := make([]Values, len(groups))
		for result := range pipe.Out() {
			results[result.index] = result.values
		}
		merged := MergeValues(results...)
		if err := pipe.Error(); err != nil {
			return !merged.IsEmpty(), merged, err
		}
		if merged.IsEmpty() {
			return false, nil, utils.Error("no value found")
		}
		return true, merged, nil
	}
}
