package sfvm

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type Values []ValueOperator

func NewValues(values []ValueOperator) Values {
	if len(values) == 0 {
		return NewEmptyValues()
	}
	return append(Values{}, values...)
}

func NewEmptyValues() Values {
	return Values{}
}

func ValuesOf(values ...ValueOperator) Values {
	return NewValues(values)
}

func (v Values) Clone() Values {
	if len(v) == 0 {
		return NewEmptyValues()
	}
	return append(Values{}, v...)
}

func (v Values) Count() int {
	return len(v)
}

func (v Values) IsEmpty() bool {
	return len(v) == 0
}

func (v Values) Recursive(f func(operator ValueOperator) error) error {
	for _, sub := range v {
		if utils.IsNil(sub) {
			continue
		}
		if err := f(sub); err != nil {
			return err
		}
	}
	return nil
}

func (v Values) String() string {
	var res []string
	for _, item := range v {
		if utils.IsNil(item) {
			continue
		}
		res = append(res, item.String())
	}
	return strings.Join(res, "; ")
}

func (v Values) ListIndex(i int) (ValueOperator, error) {
	if i < 0 || i >= len(v) {
		return nil, utils.Error("index out of range")
	}
	return v[i], nil
}

func (v Values) First() (ValueOperator, bool) {
	if len(v) == 0 {
		return nil, false
	}
	return v[0], true
}

func (v Values) AppendPredecessor(value ValueOperator, opts ...AnalysisContextOption) error {
	return v.Recursive(func(operator ValueOperator) error {
		return operator.AppendPredecessor(value, opts...)
	})
}

func (v Values) CompareConst(comparator *ConstComparator) []bool {
	res := make([]bool, 0, len(v))
	for _, operator := range v {
		if utils.IsNil(operator) {
			res = append(res, false)
			continue
		}
		res = append(res, operator.CompareConst(comparator))
	}
	return res
}

func (v Values) NewConst(i any, rng ...*memedit.Range) ValueOperator {
	operator, ok := v.First()
	if !ok || utils.IsNil(operator) {
		return nil
	}
	return operator.NewConst(i, rng...)
}

func (v Values) pipeLineRun(f func(ValueOperator) (Values, error)) (Values, error) {
	ctx := context.Background()
	size := len(v)
	pipe := pipeline.NewPipe(ctx, size, func(operator ValueOperator) (Values, error) {
		value, err := f(operator)
		if err == nil {
			mergeAnchorBitVectorToResult(value, operator)
		}
		return value, err
	})
	_ = v.Recursive(func(operator ValueOperator) error {
		pipe.Feed(operator)
		return nil
	})
	pipe.Close()
	return MergeValues(lo.ChannelToSlice(pipe.Out())...), nil
}

func (v Values) CompareOpcode(comparator *OpcodeComparator) (Values, []bool) {
	var res []bool
	var candidates []ValueOperator
	_ = v.Recursive(func(operator ValueOperator) error {
		matched, result := operator.CompareOpcode(comparator)
		res = append(res, result...)
		filtered := pickCandidateByMask(matched, result)
		mergeAnchorBitVectorToResult(filtered, operator)
		candidates = append(candidates, filtered...)
		return nil
	})
	return NewValues(candidates), res
}

func (v Values) CompareString(comparator *StringComparator) (Values, []bool) {
	var res []bool
	var candidates []ValueOperator
	_ = v.Recursive(func(operator ValueOperator) error {
		matched, result := operator.CompareString(comparator)
		res = append(res, result...)
		filtered := pickCandidateByMask(matched, result)
		mergeAnchorBitVectorToResult(filtered, operator)
		candidates = append(candidates, filtered...)
		return nil
	})
	return NewValues(candidates), res
}

func pickCandidateByMask(candidate Values, cond []bool) Values {
	if candidate.IsEmpty() {
		return NewEmptyValues()
	}
	if len(cond) == 0 {
		return candidate
	}

	if len(candidate) != len(cond) {
		if anyTrue(cond) {
			return candidate
		}
		return NewEmptyValues()
	}

	filtered := make([]ValueOperator, 0, len(cond))
	for idx, ok := range cond {
		if !ok {
			continue
		}
		filtered = append(filtered, candidate[idx])
	}
	return NewValues(filtered)
}

func (v Values) GetCalled() (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetCalled()
	})
}

func (v Values) GetFields() (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetFields()
	})
}

func (v Values) GetCallActualParams(i int, contain bool) (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetCallActualParams(i, contain)
	})
}

func (v Values) GetSyntaxFlowDef() (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetSyntaxFlowDef()
	})
}

func (v Values) GetSyntaxFlowUse() (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetSyntaxFlowUse()
	})
}

func (v Values) GetSyntaxFlowTopDef(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetSyntaxFlowTopDef(sfResult, sfConfig, config...)
	})
}

func (v Values) GetSyntaxFlowBottomUse(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
	})
}

func (v Values) ExactMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, Values, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		_, nextValue, err := vo.ExactMatch(ctx, mod, s)
		return nextValue, err
	})
	return !ret.IsEmpty(), ret, err
}

func (v Values) GlobMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, Values, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		_, nextValue, err := vo.GlobMatch(ctx, mod, s)
		return nextValue, err
	})
	return !ret.IsEmpty(), ret, err
}

func (v Values) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, pattern string) (bool, Values, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		_, nextValue, err := vo.RegexpMatch(ctx, mod, pattern)
		return nextValue, err
	})
	return !ret.IsEmpty(), ret, err
}

func (v Values) FileFilter(path string, mode string, rule1 map[string]string, rule2 []string) (Values, error) {
	return v.pipeLineRun(func(vo ValueOperator) (Values, error) {
		return vo.FileFilter(path, mode, rule1, rule2)
	})
}

func MergeValues(groups ...Values) Values {
	if len(groups) == 0 {
		return NewEmptyValues()
	}
	result := make(Values, 0)
	indexByKey := make(map[string]int)
	for _, group := range groups {
		for _, value := range group {
			if utils.IsNil(value) || value.IsEmpty() {
				continue
			}
			key := valueCollectionKey(value)
			if idx, ok := indexByKey[key]; ok {
				mergeAnchorBitVector(result[idx], value)
				continue
			}
			indexByKey[key] = len(result)
			result = append(result, value)
		}
	}
	return result
}

func RemoveValues(base Values, removed ...Values) Values {
	if base.IsEmpty() {
		return NewEmptyValues()
	}
	removedSet := make(map[string]struct{})
	for _, group := range removed {
		for _, value := range group {
			if utils.IsNil(value) {
				continue
			}
			removedSet[valueCollectionKey(value)] = struct{}{}
		}
	}
	result := make(Values, 0, len(base))
	for _, value := range base {
		if utils.IsNil(value) {
			continue
		}
		if _, ok := removedSet[valueCollectionKey(value)]; ok {
			continue
		}
		result = append(result, value)
	}
	return result
}

func IntersectValues(left Values, right Values) Values {
	if left.IsEmpty() || right.IsEmpty() {
		return NewEmptyValues()
	}
	rightIndex := make(map[string]ValueOperator, len(right))
	for _, value := range right {
		if utils.IsNil(value) {
			continue
		}
		rightIndex[valueCollectionKey(value)] = value
	}
	result := make(Values, 0)
	for _, value := range left {
		if utils.IsNil(value) {
			continue
		}
		matched, ok := rightIndex[valueCollectionKey(value)]
		if !ok {
			continue
		}
		mergeAnchorBitVector(value, matched)
		result = append(result, value)
	}
	return result
}

func valueCollectionKey(value ValueOperator) string {
	if utils.IsNil(value) {
		return ""
	}
	if id, ok := fetchId(value); ok {
		return fmt.Sprintf("id:%d", id)
	}
	if hasher, ok := value.(interface{ Hash() (string, bool) }); ok {
		if hash, ok := hasher.Hash(); ok {
			return "hash:" + hash
		}
	}
	return fmt.Sprintf("%T:%p", value, value)
}
