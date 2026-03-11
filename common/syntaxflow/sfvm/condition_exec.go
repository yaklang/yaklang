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
	Candidate Values
	Matched   bool
}

func newMaskCondition(mask []bool, candidate Values) ConditionEntry {
	return ConditionEntry{
		Mode:      ConditionModeMask,
		Mask:      mask,
		Candidate: candidate,
	}
}

func newCandidateCondition(matched bool, candidate Values) ConditionEntry {
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

func hasNonEmptyValue(value Values) bool {
	return !value.IsEmpty()
}

func conditionModeFromSource(source Values) ConditionMode {
	if len(source) != 1 {
		return ConditionModeMask
	}
	if utils.IsNil(source[0]) {
		return ConditionModeMask
	}
	if source[0].ShouldUseConditionCandidate() {
		return ConditionModeCandidate
	}
	return ConditionModeMask
}

func buildValueByID(values Values) map[int64]ValueOperator {
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

func normalizeConditionAgainstSource(scope conditionScopeState, result Values, cond []bool) ([]bool, error) {
	width := scope.anchorWidth
	if width == 0 {
		return nil, nil
	}
	if len(cond) == width {
		return cond, nil
	}

	// Program/overlay-like singleton source: cond can be omitted and derived from result existence.
	if width == 1 {
		matched := anyTrue(cond)
		if !matched && !result.IsEmpty() {
			matched = len(result) > 0
		}
		return []bool{matched}, nil
	}

	// General case: map result values back to source by anchor bits.
	mask := make([]bool, width)
	if !result.IsEmpty() {
		for _, operator := range result {
			if utils.IsNil(operator) || operator.IsEmpty() {
				continue
			}
			bits := operator.GetAnchorBitVector()
			if bits == nil || bits.IsEmpty() {
				return nil, utils.Wrapf(
					CriticalError,
					"condition failed: missing anchor bits for result value %T(%s)",
					operator,
					operator.String(),
				)
			}
			matched := markMaskByBitVector(mask, bits, scope.anchorBase)
			if !matched {
				return nil, utils.Wrapf(
					CriticalError,
					"condition failed: anchor bits out of active scope (base=%d,width=%d) for result value %T(%s)",
					scope.anchorBase,
					scope.anchorWidth,
					operator,
					operator.String(),
				)
			}
		}
	}
	return mask, nil
}

func valueSetFromValues(values Values) *ValueSet {
	set := NewValueSet()
	for _, v := range values {
		if idGetter, ok := v.(ssa.GetIdIF); ok {
			set.Add(idGetter.GetId(), v)
		}
	}
	return set
}

func intersectValuesByString(left Values, right Values) Values {
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
	return NewValues(out)
}

func mergeValuesByID(left Values, right Values, andMode bool) Values {
	leftEmpty := left.IsEmpty()
	rightEmpty := right.IsEmpty()
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

	leftSet := valueSetFromValues(left)
	rightSet := valueSetFromValues(right)
	leftByIDMap := buildValueByID(left)
	rightByIDMap := buildValueByID(right)
	leftByID := leftSet.List()
	rightByID := rightSet.List()

	// Fallback for non-id values: keep existing side in OR mode.
	if len(leftByID) == 0 || len(rightByID) == 0 {
		if andMode {
			return intersectValuesByString(left, right)
		}
		return MergeValues(left, right)
	}

	var out []ValueOperator
	if andMode {
		andSet := leftSet.And(rightSet)
		if andSet != nil {
			out = andSet.List()
		}
		if len(out) == 0 {
			out = intersectValuesByString(left, right)
		}
		if len(out) == 0 {
			// Program-like compare candidates may not share stable IDs/strings.
			// Keep non-empty side instead of dropping everything.
			if len(right) > 0 {
				return right
			}
			if len(left) > 0 {
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
			mergeAnchorBitVector(value, leftValue)
		}
		if rightValue, ok := rightByIDMap[idGetter.GetId()]; ok {
			mergeAnchorBitVector(value, rightValue)
		}
	}
	return NewValues(out)
}

func buildConditionEntry(scope conditionScopeState, cond []bool, candidate Values, fromCompare bool) (ConditionEntry, error) {
	mode := scope.mode
	matched := anyTrue(cond)
	if fromCompare && !matched && hasNonEmptyValue(candidate) {
		matched = true
	}
	if mode == ConditionModeCandidate {
		return newCandidateCondition(matched, candidate), nil
	}
	if fromCompare {
		mask, err := normalizeConditionAgainstSource(scope, candidate, cond)
		if err != nil {
			return ConditionEntry{}, err
		}
		return newMaskCondition(mask, candidate), nil
	}
	return newMaskCondition(cond, candidate), nil
}

func (s *SFFrame) activeConditionScope() (conditionScopeState, bool) {
	if s == nil || s.conditionScope == nil || s.conditionScope.Len() == 0 {
		return conditionScopeState{}, false
	}
	return s.conditionScope.Peek(), true
}

func (s *SFFrame) pushCondition(cond []bool, candidate Values, fromCompare bool) error {
	scope, ok := s.activeConditionScope()
	if !ok {
		return utils.Wrap(CriticalError, "condition failed: missing condition scope")
	}
	entry, err := buildConditionEntry(scope, cond, candidate, fromCompare)
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
		s.conditionStack.Push(newCandidateCondition(!entry.Matched, NewEmptyValues()))
		return nil
	case ConditionModeMask:
		s.conditionStack.Push(newMaskCondition(invertMask(entry.Mask), NewEmptyValues()))
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

func buildFilterMask(scope conditionScopeState, cond Values) ([]bool, error) {
	mask := make([]bool, scope.anchorWidth)
	for _, operator := range cond {
		if utils.IsNil(operator) || operator.IsEmpty() {
			continue
		}
		bits := operator.GetAnchorBitVector()
		if bits == nil || bits.IsEmpty() {
			return nil, utils.Wrapf(
				CriticalError,
				"filter condition failed: missing anchor bits for %T(%s)",
				operator,
				operator.String(),
			)
		}
		matched := markMaskByBitVector(mask, bits, scope.anchorBase)
		if !matched {
			return nil, utils.Wrapf(
				CriticalError,
				"filter condition failed: anchor bits out of active scope (base=%d,width=%d) for %T(%s)",
				scope.anchorBase,
				scope.anchorWidth,
				operator,
				operator.String(),
			)
		}
	}
	return mask, nil
}

func (s *SFFrame) pushFilterCondition(cond Values) error {
	scope, ok := s.activeConditionScope()
	if !ok {
		return utils.Wrap(CriticalError, "condition failed: missing condition scope")
	}
	if scope.mode == ConditionModeCandidate {
		return s.pushCondition([]bool{hasNonEmptyValue(cond)}, cond, false)
	}
	mask, err := buildFilterMask(scope, cond)
	if err != nil {
		return err
	}
	return s.pushCondition(mask, cond, false)
}

func applyCondition(value Values, entry ConditionEntry) (Values, error) {
	switch entry.Mode {
	case ConditionModeCandidate:
		if entry.Matched && !entry.Candidate.IsEmpty() {
			return entry.Candidate, nil
		}
		return NewEmptyValues(), nil
	case ConditionModeMask:
		if len(entry.Mask) != len(value) {
			return nil, utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", len(value), len(entry.Mask))
		}
		filtered := make([]ValueOperator, 0, len(value))
		for idx, ok := range entry.Mask {
			if !ok {
				continue
			}
			filtered = append(filtered, value[idx])
		}
		return NewValues(filtered), nil
	default:
		return nil, utils.Wrapf(CriticalError, "condition failed: invalid mode %v", entry.Mode)
	}
}
