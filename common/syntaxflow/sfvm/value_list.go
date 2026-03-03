package sfvm

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/pipeline"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var _ ValueOperator = (*ValueList)(nil)

func NewValues(values []ValueOperator) ValueOperator {
	zero := NewEmptyValues()
	ret, err := zero.Merge(values...)
	if err != nil {
		return zero
	}
	return ret
}

func (v *ValueList) IsEmpty() bool {
	if len(v.Values) == 0 {
		return true
	}
	return false
}

func (v *ValueList) ShouldUseConditionCandidate() bool {
	return false
}

func (v *ValueList) GetSourceBitVector() *utils.BitVector {
	return nil
}

func (v *ValueList) SetSourceBitVector(*utils.BitVector) {}

func NewEmptyValues() ValueOperator {
	return &ValueList{Values: nil}
}

type ValueList struct {
	Values []ValueOperator
}

func (v *ValueList) Count() int {
	return len(v.Values)
}

func (v *ValueList) ensureSourceBitVector() {
	for idx, operator := range v.Values {
		if utils.IsNil(operator) {
			continue
		}
		bits := operator.GetSourceBitVector()
		if bits == nil {
			bits = utils.NewBitVector()
		}
		bits.Set(idx)
		operator.SetSourceBitVector(bits)
	}
}

func mergeSourceBitVectorForValue(value ValueOperator, sourceBits *utils.BitVector) {
	if utils.IsNil(value) || sourceBits == nil || sourceBits.IsEmpty() {
		return
	}
	_ = value.Recursive(func(operator ValueOperator) error {
		if utils.IsNil(operator) {
			return nil
		}
		bits := operator.GetSourceBitVector()
		if bits == nil {
			operator.SetSourceBitVector(sourceBits)
			return nil
		}
		merged := bits.Clone()
		merged.Or(sourceBits)
		operator.SetSourceBitVector(merged)
		return nil
	})
}

func (v *ValueList) pipeLineRun(f func(ValueOperator) (ValueOperator, error)) (ValueOperator, error) {
	ctx := context.Background()
	v.ensureSourceBitVector()
	size := v.Count()
	pipe := pipeline.NewPipe(ctx, size, func(v ValueOperator) (ValueOperator, error) {
		var err error
		var value ValueOperator
		value, err = f(v)
		if err == nil {
			mergeSourceBitVectorForValue(value, v.GetSourceBitVector())
		}
		return value, err
	})
	v.Recursive(func(operator ValueOperator) error {
		pipe.Feed(operator)
		return nil
	})
	pipe.Close()
	data := NewValues(lo.ChannelToSlice(pipe.Out()))
	return data, nil
}

func (v *ValueList) CompareConst(comparator *ConstComparator) []bool {
	var res []bool
	v.Recursive(func(operator ValueOperator) error {
		result := operator.CompareConst(comparator)
		res = append(res, result...)
		return nil
	})
	return res
}

func (v *ValueList) NewConst(i any, rng ...*memedit.Range) ValueOperator {
	var result ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		result = operator.NewConst(i, rng...)
		return nil
	})
	return result
}

func (v *ValueList) CompareOpcode(comparator *OpcodeComparator) (ValueOperator, []bool) {
	v.ensureSourceBitVector()
	var res []bool
	var candidates []ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		matched, result := operator.CompareOpcode(comparator)
		res = append(res, result...)
		filtered := pickCandidateByMask(matched, result)
		mergeSourceBitVectorForValue(filtered, operator.GetSourceBitVector())
		if !utils.IsNil(filtered) && !filtered.IsEmpty() {
			candidates = append(candidates, filtered)
		}
		return nil
	})
	if len(candidates) == 0 {
		return NewEmptyValues(), res
	}
	return NewValues(candidates), res
}

func (v *ValueList) CompareString(comparator *StringComparator) (ValueOperator, []bool) {
	v.ensureSourceBitVector()
	var res []bool
	var candidates []ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		matched, result := operator.CompareString(comparator)
		res = append(res, result...)
		filtered := pickCandidateByMask(matched, result)
		mergeSourceBitVectorForValue(filtered, operator.GetSourceBitVector())
		if !utils.IsNil(filtered) && !filtered.IsEmpty() {
			candidates = append(candidates, filtered)
		}
		return nil
	})
	if len(candidates) == 0 {
		return NewEmptyValues(), res
	}
	return NewValues(candidates), res
}

func pickCandidateByMask(candidate ValueOperator, cond []bool) ValueOperator {
	if utils.IsNil(candidate) || candidate.IsEmpty() {
		return NewEmptyValues()
	}
	if len(cond) == 0 {
		return candidate
	}

	// If condition width cannot align with candidate width, keep the candidate
	// only when there is at least one true condition.
	if ValuesLen(candidate) != len(cond) {
		for _, ok := range cond {
			if ok {
				return candidate
			}
		}
		return NewEmptyValues()
	}

	filtered := make([]ValueOperator, 0, len(cond))
	for idx, ok := range cond {
		if !ok {
			continue
		}
		sub, err := candidate.ListIndex(idx)
		if err != nil || utils.IsNil(sub) {
			continue
		}
		filtered = append(filtered, sub)
	}
	if len(filtered) == 0 {
		return NewEmptyValues()
	}
	return NewValues(filtered)
}

func (v *ValueList) AppendPredecessor(value ValueOperator, opts ...AnalysisContextOption) error {
	return v.Recursive(func(operator ValueOperator) error {
		return operator.AppendPredecessor(value, opts...)
	})
}

func (v *ValueList) Merge(values ...ValueOperator) (ValueOperator, error) {
	if v.IsEmpty() && len(values) == 0 {
		return NewEmptyValues(), nil
	}
	var res []ValueOperator
	v.Recursive(func(operator ValueOperator) error {
		res = append(res, operator)
		return nil
	})
	for _, value := range values {
		if utils.IsNil(value) {
			continue
		}
		value.Recursive(func(vo ValueOperator) error {
			res = append(res, vo)
			return nil
		})
	}
	if len(res) > 1 {
		if ret, err := res[0].Merge(res[1:]...); err == nil {
			return ret, nil
		} else {
			return &ValueList{Values: res}, nil
		}
	} else if len(res) == 1 {
		return res[0], nil
	} else {
		return NewEmptyValues(), nil
	}
}

func (v *ValueList) Remove(values ...ValueOperator) (ValueOperator, error) {
	var filter = make(map[int64]ValueOperator)
	_ = v.Recursive(func(operator ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			_, existed := filter[raw.GetId()]
			if !existed {
				filter[raw.GetId()] = operator
			}
		}
		return nil
	})
	NewValues(values).Recursive(func(operator ValueOperator) error {
		if raw, ok := operator.(ssa.GetIdIF); ok {
			delete(filter, raw.GetId())
		}
		return nil
	})
	var res []ValueOperator
	for _, v := range filter {
		res = append(res, v)
	}
	return NewValues(res), nil
}

func (v *ValueList) GetOpcode() string {
	return ""
}

func (v *ValueList) Recursive(f func(operator ValueOperator) error) error {
	if v == nil {
		return utils.Errorf("value is nil")
	}
	for _, sub := range v.Values {
		err := sub.Recursive(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *ValueList) GetCalled() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetCalled()
	})
}

func (v *ValueList) GetFields() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetFields()
	})
}

func (v *ValueList) GetBinaryOperator() string {
	return ""
}

func (v *ValueList) GetUnaryOperator() string {
	return ""
}

func (v *ValueList) String() string {
	var res []string
	for _, v := range v.Values {
		res = append(res, v.String())
	}
	return strings.Join(res, "; ")
}

func (v *ValueList) GetCallActualParams(i int, b bool) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetCallActualParams(i, b)
	})
}

func (v *ValueList) GetSyntaxFlowDef() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowDef()
	})
}

func (v *ValueList) GetSyntaxFlowUse() (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowUse()
	})
}

func (v *ValueList) GetSyntaxFlowTopDef(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowTopDef(sfResult, sfConfig, config...)
	})
}

func (v *ValueList) GetSyntaxFlowBottomUse(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
	})
}

func (v *ValueList) ListIndex(i int) (ValueOperator, error) {
	if i < 0 || i >= len(v.Values) {
		return nil, utils.Error("index out of range")
	}
	return v.Values[i], nil
}

func (v *ValueList) IsMap() bool {
	return false
}

func (v *ValueList) IsList() bool {
	return true
}

func (v *ValueList) ExactMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, ValueOperator, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		match, nextValue, err := vo.ExactMatch(ctx, mod, s)
		_ = match
		return nextValue, err
	})
	return ValuesLen(ret) > 0, ret, err
}

func (v *ValueList) GlobMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, ValueOperator, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		match, nextValue, err := vo.GlobMatch(ctx, mod, s)
		_ = match
		return nextValue, err
	})
	return ValuesLen(ret) > 0, ret, err
}

func (v *ValueList) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, regexp string) (bool, ValueOperator, error) {
	ret, err := v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		match, nextValue, err := vo.RegexpMatch(ctx, mod, regexp)
		_ = match
		return nextValue, err
	})
	return ValuesLen(ret) > 0, ret, err
}

func (v *ValueList) FileFilter(path string, mode string, rule1 map[string]string, rule2 []string) (ValueOperator, error) {
	return v.pipeLineRun(func(vo ValueOperator) (ValueOperator, error) {
		return vo.FileFilter(path, mode, rule1, rule2)
	})
}
