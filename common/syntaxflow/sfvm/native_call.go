package sfvm

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type NativeCallActualParams struct {
	// m stores values per key in source order. Duplicate keys (e.g. two exclude_reachable
	// in native$call) must append, not overwrite.
	m map[string][]any
}

func NewNativeCallActualParams(items ...*RecursiveConfigItem) *NativeCallActualParams {
	n := &NativeCallActualParams{
		m: make(map[string][]any),
	}
	for _, i := range items {
		if i == nil {
			continue
		}
		k := i.Key
		n.m[k] = append(n.m[k], i.Value)
	}
	return n
}

func (n *NativeCallActualParams) Existed(index any) bool {
	if n == nil {
		return false
	}
	vs, ok := n.m[codec.AnyToString(index)]
	return ok && len(vs) > 0
}

func (n *NativeCallActualParams) getLastStringForKeys(keys ...any) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, key := range keys {
		ks := codec.AnyToString(key)
		vs, ok := n.m[ks]
		if !ok || len(vs) == 0 {
			continue
		}
		// Last element matches old map "last write wins" when duplicates existed.
		return codec.AnyToString(vs[len(vs)-1]), true
	}
	return "", false
}

func (n *NativeCallActualParams) GetString(index any, extra ...any) string {
	if n == nil {
		return ""
	}
	if s, ok := n.getLastStringForKeys(append([]any{index}, extra...)...); ok {
		return s
	}
	return ""
}

func (n *NativeCallActualParams) GetInt(index any, extra ...any) int {
	if n == nil {
		return -1
	}
	if s, ok := n.getLastStringForKeys(append([]any{index}, extra...)...); ok {
		return codec.Atoi(s)
	}
	return -1
}

type NativeCallFunc func(v Values, frame *SFFrame, params *NativeCallActualParams) (bool, Values, error)

type ValueSingleNativeCallFunc func(operator ValueOperator, frame *SFFrame, params *NativeCallActualParams) (Values, error)
type ValuesNativeCallFunc func(group Values, slotSource ValueOperator, frame *SFFrame, params *NativeCallActualParams) (Values, error)

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

type nativeCallGroupResult struct {
	index  int
	values Values
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
		ctx := context.Background()
		if frame != nil {
			ctx = frame.GetContext()
		}

		var groups []Values
		var slotSources []ValueOperator
		var slotAnchorBits []*utils.BitVector

		// Explicit nil-guard: native calls can be executed without an SFFrame (e.g. utility usage/tests).
		var scope anchorScopeState
		ok := false
		if frame != nil {
			scope, ok = frame.activeAnchorScope()
		}
		if !ok || scope.anchorWidth <= 0 {
			groups = []Values{v}
			slotSource, _ := v.First()
			slotSources = []ValueOperator{slotSource}
			slotAnchorBits = []*utils.BitVector{nil}
		} else {
			groups = v.AnchorGroups(scope.anchorBase, scope.anchorWidth)
			slotSources = make([]ValueOperator, scope.anchorWidth)
			for idx := 0; idx < scope.anchorWidth; idx++ {
				var slotSource ValueOperator
				if idx < len(scope.source) {
					slotSource = scope.source[idx]
				}
				if utils.IsNil(slotSource) {
					slotSource, _ = groups[idx].First()
				}
				slotSources[idx] = slotSource
			}
			slotAnchorBits = scope.slotAnchorBits
		}

		if len(groups) == 0 {
			return false, nil, utils.Error("no value found")
		}

		pipe := pipeline.NewPipe(ctx, len(groups), func(idx int) (nativeCallGroupResult, error) {
			values, err := f(groups[idx], slotSources[idx], frame, params)
			if err != nil {
				return nativeCallGroupResult{}, err
			}
			if idx < len(slotAnchorBits) && slotAnchorBits[idx] != nil && !slotAnchorBits[idx].IsEmpty() {
				for _, value := range values {
					mergeAnchorBits(value, slotAnchorBits[idx])
				}
			}
			return nativeCallGroupResult{index: idx, values: values}, nil
		})
		for idx := 0; idx < len(groups); idx++ {
			pipe.Feed(idx)
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
