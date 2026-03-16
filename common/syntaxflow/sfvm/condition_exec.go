package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
)

type ConditionEntry interface {
	Mode() ConditionMode
	Bang() ConditionEntry
	Merge(other ConditionEntry, andMode bool) (ConditionEntry, error)
	Apply(value Values) (Values, error)
}

type maskConditionEntry struct {
	mask []bool
}

func (e maskConditionEntry) Mode() ConditionMode { return ConditionModeMask }
func (e maskConditionEntry) Bang() ConditionEntry {
	return maskConditionEntry{mask: invertMask(e.mask)}
}
func (e maskConditionEntry) Merge(other ConditionEntry, andMode bool) (ConditionEntry, error) {
	if other == nil {
		return nil, utils.Wrap(CriticalError, "condition failed: missing rhs condition")
	}
	o, ok := other.(maskConditionEntry)
	if !ok {
		return nil, utils.Wrapf(CriticalError, "condition failed: mode mismatch (%v vs %v)", e.Mode(), other.Mode())
	}
	if len(e.mask) != len(o.mask) {
		return nil, utils.Wrapf(CriticalError, "condition failed: conds1(%v) vs conds2(%v)", len(e.mask), len(o.mask))
	}
	res := make([]bool, len(e.mask))
	for idx := 0; idx < len(e.mask); idx++ {
		if andMode {
			res[idx] = e.mask[idx] && o.mask[idx]
		} else {
			res[idx] = e.mask[idx] || o.mask[idx]
		}
	}
	return newMaskCondition(res), nil
}
func (e maskConditionEntry) Apply(value Values) (Values, error) {
	if value.IsEmpty() {
		return NewEmptyValues(), nil
	}
	if len(e.mask) != len(value) {
		return nil, utils.Wrapf(CriticalError, "condition failed: stack top(%v) vs conds(%v)", len(value), len(e.mask))
	}
	filtered := make([]ValueOperator, 0, len(value))
	for idx, ok := range e.mask {
		if !ok {
			continue
		}
		filtered = append(filtered, value[idx])
	}
	return NewValues(filtered), nil
}

type candidateConditionEntry struct {
	matched   bool
	candidate Values
}

func (e candidateConditionEntry) Mode() ConditionMode { return ConditionModeCandidate }
func (e candidateConditionEntry) Bang() ConditionEntry {
	// Candidate-mode "!" stays conservative and only flips truthiness.
	return newCandidateCondition(!e.matched, NewEmptyValues())
}
func (e candidateConditionEntry) Merge(other ConditionEntry, andMode bool) (ConditionEntry, error) {
	if other == nil {
		return nil, utils.Wrap(CriticalError, "condition failed: missing rhs condition")
	}
	o, ok := other.(candidateConditionEntry)
	if !ok {
		return nil, utils.Wrapf(CriticalError, "condition failed: mode mismatch (%v vs %v)", e.Mode(), other.Mode())
	}

	mergedCandidate := mergeValuesByID(o.candidate, e.candidate, andMode)
	matched := e.matched && o.matched
	if !andMode {
		matched = e.matched || o.matched
	}
	return newCandidateCondition(matched, mergedCandidate), nil
}
func (e candidateConditionEntry) Apply(value Values) (Values, error) {
	if e.matched && !e.candidate.IsEmpty() {
		return e.candidate, nil
	}
	return NewEmptyValues(), nil
}

func newMaskCondition(mask []bool) ConditionEntry {
	return maskConditionEntry{mask: mask}
}

func newCandidateCondition(matched bool, candidate Values) ConditionEntry {
	return candidateConditionEntry{
		matched:   matched,
		candidate: candidate,
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

func normalizeConditionAgainstSource(scope anchorScopeState, result Values, cond []bool) ([]bool, error) {
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
	//
	// In mask-mode, a compare/native-call often produces a derived value list that has been
	// flattened/expanded, so cond length is not aligned to source slots. We normalize it by
	// projecting each derived value's anchor bits back to the scope source range:
	//   mask[i] = true iff (anchorBase+i) in derived.bits
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

func buildConditionEntry(scope anchorScopeState, cond []bool, candidate Values, fromCompare bool) (ConditionEntry, error) {
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
			return nil, err
		}
		return newMaskCondition(mask), nil
	}
	return newMaskCondition(cond), nil
}

func (s *SFFrame) pushCondition(cond []bool, candidate Values, fromCompare bool) error {
	scope, ok := s.activeAnchorScope()
	if !ok {
		return utils.Wrap(CriticalError, "condition failed: missing anchor scope")
	}
	entry, err := buildConditionEntry(scope, cond, candidate, fromCompare)
	if err != nil {
		return err
	}
	if entry == nil {
		return utils.Wrap(CriticalError, "condition failed: empty condition entry")
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
	if entry == nil {
		return utils.Wrap(CriticalError, "condition failed: empty condition stack")
	}
	next := entry.Bang()
	if next == nil {
		return utils.Wrap(CriticalError, "condition failed: empty condition entry")
	}
	s.conditionStack.Push(next)
	return nil
}

func (s *SFFrame) applyLogicBinaryCondition(andMode bool) error {
	left := s.popCondition()
	right := s.popCondition()
	if left == nil || right == nil {
		return utils.Wrap(CriticalError, "condition failed: empty condition stack")
	}
	merged, err := left.Merge(right, andMode)
	if err != nil {
		return err
	}
	if merged == nil {
		return utils.Wrap(CriticalError, "condition failed: empty condition entry")
	}
	s.conditionStack.Push(merged)
	return nil
}

func buildFilterMask(scope anchorScopeState, cond Values) ([]bool, error) {
	// Filter conditions are derived values that must carry anchor bits so we can map
	// them back to the scope source mask:
	//   mask[i] = true iff (anchorBase+i) in condValue.bits
	//
	// Missing/out-of-scope anchor bits in mask-mode is a bug (native call/op forgot to propagate).
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
	scope, ok := s.activeAnchorScope()
	if !ok {
		return utils.Wrap(CriticalError, "condition failed: missing anchor scope")
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
