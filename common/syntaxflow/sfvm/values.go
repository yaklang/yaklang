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

// inlinePipelineThreshold is the max element count for which RunValueOperatorPipeline
// processes inline (single loop, no pipeline/cancelCtx/goroutine allocation). Above this
// the concurrency benefit outweighs the channel/cancelCtx overhead. 256 was measured as
// the sweet spot: per-opcode pipe creation was the #2 allocator (~18% / ~1GB of
// cancelCtx+Done channels) and a major GC driver on large projects; for small sets the
// concurrency is worthless and the overhead dominates.
const inlinePipelineThreshold = 256

type ValuePipelineOptions struct {
	Frame            *SFFrame
	PredecessorLabel string
}

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

// AnchorGroups groups leaf values by their anchor bits within the active scope range
// [base, base+width). The returned slice always has length = width (empty groups are kept).
//
// A single value can belong to multiple groups if it carries multiple in-scope anchor bits.
func (v Values) AnchorGroups(base int, width int) []Values {
	if width <= 0 {
		return nil
	}
	groups := make([]Values, width)
	end := base + width
	_ = v.Recursive(func(operator ValueOperator) error {
		if utils.IsNil(operator) || operator.IsEmpty() {
			return nil
		}
		bits := operator.GetAnchorBitVector()
		if bits == nil || bits.IsEmpty() {
			return nil
		}
		bits.ForEach(func(index int) {
			if index < base || index >= end {
				return
			}
			groups[index-base] = append(groups[index-base], operator)
		})
		return nil
	})
	return groups
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

func RunValueOperatorPipeline(values Values, opts ValuePipelineOptions, f func(ValueOperator) (Values, error)) (Values, error) {
	if values.IsEmpty() {
		return NewEmptyValues(), nil
	}

	ctx := context.Background()
	if opts.Frame != nil {
		ctx = opts.Frame.GetContext()
	}
	propagateAnchors := false
	if opts.Frame != nil {
		_, _, propagateAnchors = opts.Frame.ActiveAnchorScope()
	}

	// Inline fast path for small value sets (see inlinePipelineThreshold).
	size := len(values)
	if size <= inlinePipelineThreshold {
		out := make([]ValueOperator, 0, size)
		var firstErr error
		_ = values.Recursive(func(operator ValueOperator) error {
			value, err := f(operator)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return err
			}
			if opts.PredecessorLabel != "" && opts.Frame != nil {
				_ = value.AppendPredecessor(operator, opts.Frame.WithPredecessorContext(opts.PredecessorLabel))
			}
			if propagateAnchors {
				MergeAnchor(operator, value...)
			}
			out = append(out, value...)
			return nil
		})
		return NewValues(out), firstErr
	}

	pipe := pipeline.NewPipe(ctx, size, func(operator ValueOperator) (Values, error) {
		value, err := f(operator)
		if err != nil {
			return value, err
		}
		if opts.PredecessorLabel != "" && opts.Frame != nil {
			_ = value.AppendPredecessor(operator, opts.Frame.WithPredecessorContext(opts.PredecessorLabel))
		}
		if propagateAnchors {
			MergeAnchor(operator, value...)
		}
		return value, nil
	})
	_ = values.Recursive(func(operator ValueOperator) error {
		pipe.Feed(operator)
		return nil
	})
	pipe.Close()
	results := MergeValues(lo.ChannelToSlice(pipe.Out())...)
	if err := pipe.Error(); err != nil {
		return results, err
	}
	return results, nil
}

func (v Values) pipeLineRun(f func(ValueOperator) (Values, error)) (Values, error) {
	return RunValueOperatorPipeline(v, ValuePipelineOptions{}, f)
}

func (s *SFFrame) runValueOperatorPipeline(source Values, predecessorLabel string, f func(ValueOperator) (Values, error)) (Values, error) {
	return RunValueOperatorPipeline(source, ValuePipelineOptions{
		Frame:            s,
		PredecessorLabel: predecessorLabel,
	}, f)
}

func (v Values) CompareOpcode(comparator *OpcodeComparator) (Values, []bool) {
	var res []bool
	var candidates []ValueOperator
	_ = v.Recursive(func(operator ValueOperator) error {
		matched, result := operator.CompareOpcode(comparator)
		res = append(res, result...)
		filtered := pickCandidateByMask(matched, result)
		MergeAnchor(operator, filtered...)
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
		MergeAnchor(operator, filtered...)
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

// dedupKey is the key type used by the value-set dedup maps (MergeValues /
// RemoveValues / IntersectValues). It avoids allocating a string per element
// (the old valueCollectionKey did `fmt.Sprintf("id:%d", id)` on every element
// of every value set on every opcode — ~22% of all allocations / ~480M calls
// on a large project, a top GC driver).
//
// Layout:
//   - kind == 1: numeric id fast path (the overwhelmingly common case — every
//     ssaapi.Value implements ssa.GetIdIF). No string allocation.
//   - kind == 2: a stable hash string (ValueOperators implementing Hash()).
//   - kind == 3: type+pointer fallback (the rare nil-id case), string kept.
type dedupKey struct {
	kind byte
	id   int64
	str  string
}

// valueDedupKey computes the dedup key for a value. It mirrors the old
// valueCollectionKey precedence (id → hash → type:%p) but returns a value type
// instead of a formatted string.
func valueDedupKey(value ValueOperator) (dedupKey, bool) {
	if utils.IsNil(value) {
		return dedupKey{}, false
	}
	if id, ok := fetchId(value); ok {
		return dedupKey{kind: 1, id: id}, true
	}
	if hasher, ok := value.(interface{ Hash() (string, bool) }); ok {
		if hash, ok := hasher.Hash(); ok {
			return dedupKey{kind: 2, str: hash}, true
		}
	}
	return dedupKey{kind: 3, str: fmt.Sprintf("%T:%p", value, value)}, true
}

func MergeValues(groups ...Values) Values {
	if len(groups) == 0 {
		return NewEmptyValues()
	}
	type provenanceMerger interface {
		MergeProvenanceFrom(ValueOperator)
	}

	result := make(Values, 0)
	indexByKey := make(map[dedupKey]int)
	for _, group := range groups {
		for _, value := range group {
			if utils.IsNil(value) || value.IsEmpty() {
				continue
			}
			key, ok := valueDedupKey(value)
			if !ok {
				continue
			}
			if idx, ok := indexByKey[key]; ok {
				if merger, ok := result[idx].(provenanceMerger); ok {
					merger.MergeProvenanceFrom(value)
				}
				MergeAnchor(value, result[idx])
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
	removedSet := make(map[dedupKey]struct{})
	for _, group := range removed {
		for _, value := range group {
			if utils.IsNil(value) {
				continue
			}
			if key, ok := valueDedupKey(value); ok {
				removedSet[key] = struct{}{}
			}
		}
	}
	result := make(Values, 0, len(base))
	for _, value := range base {
		if utils.IsNil(value) {
			continue
		}
		key, ok := valueDedupKey(value)
		if !ok {
			result = append(result, value)
			continue
		}
		if _, ok := removedSet[key]; ok {
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
	rightIndex := make(map[dedupKey]ValueOperator, len(right))
	for _, value := range right {
		if utils.IsNil(value) {
			continue
		}
		if key, ok := valueDedupKey(value); ok {
			rightIndex[key] = value
		}
	}
	result := make(Values, 0)
	for _, value := range left {
		if utils.IsNil(value) {
			continue
		}
		key, ok := valueDedupKey(value)
		if !ok {
			continue
		}
		matched, ok := rightIndex[key]
		if !ok {
			continue
		}
		MergeAnchor(matched, value)
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
