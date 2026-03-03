package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ConditionMode uint8

const (
	ConditionModeMask ConditionMode = iota
	ConditionModeCandidate
)

type ConditionEntry struct {
	Mode      ConditionMode
	Mask      []bool
	Candidate ValueOperator
	Matched   bool
}

func newMaskCondition(mask []bool, candidate ValueOperator) ConditionEntry {
	return ConditionEntry{
		Mode:      ConditionModeMask,
		Mask:      mask,
		Candidate: candidate,
	}
}

func newCandidateCondition(matched bool, candidate ValueOperator) ConditionEntry {
	return ConditionEntry{
		Mode:      ConditionModeCandidate,
		Candidate: candidate,
		Matched:   matched,
	}
}

func anyTrue(values []bool) bool {
	for _, ok := range values {
		if ok {
			return true
		}
	}
	return false
}

func hasNonEmptyValue(value ValueOperator) bool {
	if utils.IsNil(value) {
		return false
	}
	matched := false
	_ = value.Recursive(func(operator ValueOperator) error {
		if utils.IsNil(operator) || operator.IsEmpty() {
			return nil
		}
		matched = true
		return nil
	})
	return matched
}

func conditionModeFromSource(source ValueOperator) ConditionMode {
	sourceVals := flattenValues(source)
	if len(sourceVals) != 1 {
		return ConditionModeMask
	}
	if utils.IsNil(sourceVals[0]) {
		return ConditionModeMask
	}
	if sourceVals[0].ShouldUseConditionCandidate() {
		return ConditionModeCandidate
	}
	return ConditionModeMask
}

func flattenValues(value ValueOperator) []ValueOperator {
	if utils.IsNil(value) {
		return nil
	}
	var result []ValueOperator
	_ = value.Recursive(func(operator ValueOperator) error {
		result = append(result, operator)
		return nil
	})
	return result
}

func ensureSourceBitVector(source []ValueOperator) {
	for idx, src := range source {
		if utils.IsNil(src) {
			continue
		}
		bits := src.GetSourceBitVector()
		if bits == nil {
			bits = utils.NewBitVector()
		}
		bits.Set(idx)
		src.SetSourceBitVector(bits)
	}
}

func markMaskByBitVector(mask []bool, bits *utils.BitVector) bool {
	if bits == nil || bits.IsEmpty() {
		return false
	}
	matched := false
	bits.ForEach(func(index int) {
		if index >= 0 && index < len(mask) {
			mask[index] = true
			matched = true
		}
	})
	return matched
}

func mergeSourceBitVector(dst ValueOperator, src ValueOperator) {
	if utils.IsNil(dst) || utils.IsNil(src) {
		return
	}
	srcBits := src.GetSourceBitVector()
	if srcBits == nil || srcBits.IsEmpty() {
		return
	}
	dstBits := dst.GetSourceBitVector()
	if dstBits == nil {
		dst.SetSourceBitVector(srcBits)
		return
	}
	merged := dstBits.Clone()
	merged.Or(srcBits)
	dst.SetSourceBitVector(merged)
}

func buildValueByID(values []ValueOperator) map[int64]ValueOperator {
	res := make(map[int64]ValueOperator, len(values))
	for _, value := range values {
		if utils.IsNil(value) {
			continue
		}
		if idGetter, ok := value.(ssa.GetIdIF); ok {
			res[idGetter.GetId()] = value
		}
	}
	return res
}

func normalizeConditionAgainstSource(source ValueOperator, result ValueOperator, cond []bool) ([]bool, error) {
	sourceVals := flattenValues(source)
	width := len(sourceVals)
	if width == 0 {
		return nil, nil
	}
	ensureSourceBitVector(sourceVals)
	if len(cond) == width {
		return cond, nil
	}

	// Program/overlay-like singleton source: cond can be omitted and derived from result existence.
	if width == 1 {
		matched := false
		for _, ok := range cond {
			if ok {
				matched = true
				break
			}
		}
		if !matched && !utils.IsNil(result) && !result.IsEmpty() {
			matched = ValuesLen(result) > 0
		}
		return []bool{matched}, nil
	}

	// General case: map result values back to source by bitvector only.
	mask := make([]bool, width)
	hasTruthCondition := len(cond) == 0
	resIndex := 0
	if !utils.IsNil(result) {
		if err := result.Recursive(func(operator ValueOperator) error {
			if utils.IsNil(operator) {
				resIndex++
				return nil
			}
			truthy := len(cond) == 0
			if len(cond) > 0 {
				truthy = resIndex < len(cond) && cond[resIndex]
			}
			resIndex++
			if !truthy {
				return nil
			}
			hasTruthCondition = true
			markMaskByBitVector(mask, operator.GetSourceBitVector())
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if !hasTruthCondition {
		return mask, nil
	}
	return mask, nil
}

func valueSetFromValues(values []ValueOperator) *ValueSet {
	set := NewValueSet()
	for _, v := range values {
		if idGetter, ok := v.(ssa.GetIdIF); ok {
			set.Add(idGetter.GetId(), v)
		}
	}
	return set
}

func intersectValuesByString(left []ValueOperator, right []ValueOperator) []ValueOperator {
	rightByString := make(map[string]struct{}, len(right))
	for _, rv := range right {
		rightByString[rv.String()] = struct{}{}
	}
	var out []ValueOperator
	for _, lv := range left {
		if _, ok := rightByString[lv.String()]; ok {
			out = append(out, lv)
		}
	}
	return out
}

func mergeValuesByID(left ValueOperator, right ValueOperator, andMode bool) ValueOperator {
	leftEmpty := utils.IsNil(left) || left.IsEmpty()
	rightEmpty := utils.IsNil(right) || right.IsEmpty()
	if utils.IsNil(left) && utils.IsNil(right) {
		return nil
	}
	if andMode {
		if leftEmpty && rightEmpty {
			return NewEmptyValues()
		}
		if leftEmpty {
			return right
		}
		if rightEmpty {
			return left
		}
	}

	leftVals := flattenValues(left)
	rightVals := flattenValues(right)
	leftSet := valueSetFromValues(leftVals)
	rightSet := valueSetFromValues(rightVals)
	leftByIDMap := buildValueByID(leftVals)
	rightByIDMap := buildValueByID(rightVals)
	leftByID := leftSet.List()
	rightByID := rightSet.List()

	// Fallback for non-id values: keep existing side in OR mode.
	if len(leftByID) == 0 || len(rightByID) == 0 {
		if andMode {
			return NewValues(intersectValuesByString(leftVals, rightVals))
		}
		if utils.IsNil(left) {
			return right
		}
		if utils.IsNil(right) {
			return left
		}
		merged, err := left.Merge(right)
		if err != nil {
			return left
		}
		return merged
	}

	var out []ValueOperator
	if andMode {
		andSet := leftSet.And(rightSet)
		if andSet != nil {
			out = andSet.List()
		}
		if len(out) == 0 {
			out = intersectValuesByString(leftVals, rightVals)
		}
		if len(out) == 0 {
			// Program-like compare candidates may not share stable IDs/strings.
			// Keep non-empty side instead of dropping everything.
			if len(rightVals) > 0 {
				return right
			}
			if len(leftVals) > 0 {
				return left
			}
		}
	} else {
		orSet := leftSet.Or(rightSet)
		if orSet != nil {
			out = orSet.List()
		}
	}
	for _, value := range out {
		idGetter, ok := value.(ssa.GetIdIF)
		if !ok {
			continue
		}
		if leftValue, ok := leftByIDMap[idGetter.GetId()]; ok {
			mergeSourceBitVector(value, leftValue)
		}
		if rightValue, ok := rightByIDMap[idGetter.GetId()]; ok {
			mergeSourceBitVector(value, rightValue)
		}
	}
	return NewValues(out)
}

func buildConditionEntry(source ValueOperator, cond []bool, candidate ValueOperator, fromCompare bool) (ConditionEntry, error) {
	mode := conditionModeFromSource(source)
	matched := anyTrue(cond)
	if fromCompare && !matched && hasNonEmptyValue(candidate) {
		matched = true
	}
	if mode == ConditionModeCandidate {
		return newCandidateCondition(matched, candidate), nil
	}
	if fromCompare {
		mask, err := normalizeConditionAgainstSource(source, candidate, cond)
		if err != nil {
			return ConditionEntry{}, err
		}
		return newMaskCondition(mask, candidate), nil
	} else {
		return newMaskCondition(cond, candidate), nil
	}
}

func (s *SFFrame) pushCondition(source ValueOperator, cond []bool, candidate ValueOperator, fromCompare bool) error {
	entry, err := buildConditionEntry(source, cond, candidate, fromCompare)
	if err != nil {
		return err
	}
	s.conditionStack.Push(entry)
	return nil
}

func (s *SFFrame) popCondition() ConditionEntry {
	return s.conditionStack.Pop()
}

func invertMask(mask []bool) []bool {
	out := make([]bool, len(mask))
	for i := 0; i < len(mask); i++ {
		out[i] = !mask[i]
	}
	return out
}

func (s *SFFrame) applyLogicBangCondition() error {
	entry := s.popCondition()
	switch entry.Mode {
	case ConditionModeCandidate:
		// Candidate-mode "!" stays conservative and only flips truthiness.
		s.conditionStack.Push(newCandidateCondition(!entry.Matched, nil))
		return nil
	case ConditionModeMask:
		s.conditionStack.Push(newMaskCondition(invertMask(entry.Mask), nil))
		return nil
	default:
		return utils.Wrapf(CriticalError, "condition failed: invalid mode %v", entry.Mode)
	}
}

func (s *SFFrame) applyLogicBinaryCondition(andMode bool) error {
	left := s.popCondition()
	right := s.popCondition()
	if left.Mode != right.Mode {
		return utils.Wrapf(CriticalError, "condition failed: mode mismatch (%v vs %v)", left.Mode, right.Mode)
	}

	mergedCandidate := mergeValuesByID(right.Candidate, left.Candidate, andMode)
	switch left.Mode {
	case ConditionModeCandidate:
		matched := left.Matched && right.Matched
		if !andMode {
			matched = left.Matched || right.Matched
		}
		s.conditionStack.Push(newCandidateCondition(matched, mergedCandidate))
		return nil
	case ConditionModeMask:
		if len(left.Mask) != len(right.Mask) {
			return utils.Wrapf(CriticalError, "condition failed: conds1(%v) vs conds2(%v)", len(left.Mask), len(right.Mask))
		}

		res := make([]bool, len(left.Mask))
		for idx := 0; idx < len(left.Mask); idx++ {
			if andMode {
				res[idx] = left.Mask[idx] && right.Mask[idx]
			} else {
				res[idx] = left.Mask[idx] || right.Mask[idx]
			}
		}
		s.conditionStack.Push(newMaskCondition(res, mergedCandidate))
		return nil
	default:
		return utils.Wrapf(CriticalError, "condition failed: invalid mode %v", left.Mode)
	}
}

func buildFilterMask(source ValueOperator, cond ValueOperator) ([]bool, error) {
	srcValues := flattenValues(source)
	ensureSourceBitVector(srcValues)
	mask := make([]bool, len(srcValues))

	if err := cond.Recursive(func(operator ValueOperator) error {
		if utils.IsNil(operator) {
			return nil
		}
		if operator.IsEmpty() {
			return nil
		}
		markMaskByBitVector(mask, operator.GetSourceBitVector())
		return nil
	}); err != nil {
		return nil, err
	}

	return mask, nil
}

func (s *SFFrame) pushFilterCondition(source ValueOperator, cond ValueOperator) error {
	if conditionModeFromSource(source) == ConditionModeCandidate {
		return s.pushCondition(source, []bool{hasNonEmptyValue(cond)}, cond, false)
	} else {
		mask, err := buildFilterMask(source, cond)
		if err != nil {
			return err
		}
		return s.pushCondition(source, mask, cond, false)
	}
}

func filterValueByMask(value ValueOperator, cond []bool) (ValueOperator, error) {
	if len(cond) != ValuesLen(value) {
		return nil, utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", ValuesLen(value), len(cond))
	}
	filtered := make([]ValueOperator, 0, ValuesLen(value))
	for idx := 0; idx < len(cond); idx++ {
		if !cond[idx] {
			continue
		}
		if v, err := value.ListIndex(idx); err == nil {
			filtered = append(filtered, v)
		}
	}
	return NewValues(filtered), nil
}

func (s *SFFrame) applyCondition(value ValueOperator) (ValueOperator, error) {
	entry := s.popCondition()
	expectedMode := conditionModeFromSource(value)
	if entry.Mode != expectedMode {
		return nil, utils.Wrapf(CriticalError, "condition failed: mode mismatch (%v vs %v)", entry.Mode, expectedMode)
	}

	switch entry.Mode {
	case ConditionModeCandidate:
		if entry.Matched && !utils.IsNil(entry.Candidate) && !entry.Candidate.IsEmpty() {
			return entry.Candidate, nil
		}
		return NewEmptyValues(), nil
	case ConditionModeMask:
		return filterValueByMask(value, entry.Mask)
	default:
		return nil, utils.Wrapf(CriticalError, "condition failed: invalid mode %v", entry.Mode)
	}
}
