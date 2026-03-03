package sfvm

import (
	"reflect"

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

func collectPredecessorNodes(value any) []any {
	if value == nil {
		return nil
	}
	rawVal := reflect.ValueOf(value)
	if !rawVal.IsValid() {
		return nil
	}
	method := rawVal.MethodByName("GetPredecessors")
	if !method.IsValid() || method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		return nil
	}
	outs := method.Call(nil)
	if len(outs) != 1 {
		return nil
	}
	preds := outs[0]
	if !preds.IsValid() || preds.Kind() != reflect.Slice {
		return nil
	}
	nodes := make([]any, 0, preds.Len())
	for i := 0; i < preds.Len(); i++ {
		pred := preds.Index(i)
		if !pred.IsValid() {
			continue
		}
		if pred.Kind() == reflect.Ptr && pred.IsNil() {
			continue
		}
		if pred.Kind() == reflect.Ptr {
			pred = pred.Elem()
		}
		if !pred.IsValid() || pred.Kind() != reflect.Struct {
			continue
		}
		nodeField := pred.FieldByName("Node")
		if !nodeField.IsValid() {
			continue
		}
		if nodeField.Kind() == reflect.Ptr && nodeField.IsNil() {
			continue
		}
		nodes = append(nodes, nodeField.Interface())
	}
	return nodes
}

func collectReachableSourceIDs(value ValueOperator) []int64 {
	if utils.IsNil(value) {
		return nil
	}
	var ids []int64
	queue := []any{value}
	visited := make(map[int64]struct{})
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		idGetter, ok := current.(ssa.GetIdIF)
		if !ok {
			continue
		}
		id := idGetter.GetId()
		if _, existed := visited[id]; existed {
			continue
		}
		visited[id] = struct{}{}
		ids = append(ids, id)
		queue = append(queue, collectPredecessorNodes(current)...)
	}
	return ids
}

func buildSourceIDIndex(source []ValueOperator) map[int64][]int {
	index := make(map[int64][]int, len(source))
	for i, src := range source {
		if utils.IsNil(src) {
			continue
		}
		if idGetter, ok := src.(ssa.GetIdIF); ok {
			index[idGetter.GetId()] = append(index[idGetter.GetId()], i)
		}
	}
	return index
}

func normalizeConditionAgainstSource(source ValueOperator, result ValueOperator, cond []bool) ([]bool, error) {
	sourceVals := flattenValues(source)
	width := len(sourceVals)
	if width == 0 {
		return nil, nil
	}
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

	// General case: map result values back to source only by direct ID.
	// Compare operators should not expand via predecessor chain here.
	mask := make([]bool, width)
	sourceByID := buildSourceIDIndex(sourceVals)
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
			if idGetter, ok := operator.(ssa.GetIdIF); ok {
				if positions, existed := sourceByID[idGetter.GetId()]; existed {
					for _, pos := range positions {
						if pos >= 0 && pos < len(mask) {
							mask[pos] = true
						}
					}
					return nil
				}
			}
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
	return NewValues(out)
}

func normalizeConditionEntryAgainstSource(source ValueOperator, entry ConditionEntry) (ConditionEntry, error) {
	switch entry.Mode {
	case ConditionModeMask:
		mask, err := normalizeConditionAgainstSource(source, entry.Candidate, entry.Mask)
		if err != nil {
			return ConditionEntry{}, err
		}
		return newMaskCondition(mask, entry.Candidate), nil
	case ConditionModeCandidate:
		width := len(flattenValues(source))
		if width == 0 {
			return newMaskCondition(nil, entry.Candidate), nil
		}
		if !utils.IsNil(entry.Candidate) && !entry.Candidate.IsEmpty() {
			mask, err := normalizeConditionAgainstSource(source, entry.Candidate, nil)
			if err != nil {
				return ConditionEntry{}, err
			}
			if entry.Matched && len(mask) == 1 {
				mask[0] = true
			}
			return newMaskCondition(mask, entry.Candidate), nil
		}
		mask := make([]bool, width)
		if entry.Matched && width == 1 {
			mask[0] = true
		}
		return newMaskCondition(mask, entry.Candidate), nil
	default:
		return newMaskCondition(nil, nil), nil
	}
}

func coerceConditionEntryToMode(source ValueOperator, entry ConditionEntry, targetMode ConditionMode) (ConditionEntry, error) {
	if entry.Mode == targetMode {
		if targetMode == ConditionModeMask {
			return normalizeConditionEntryAgainstSource(source, entry)
		}
		return entry, nil
	}

	switch targetMode {
	case ConditionModeMask:
		return normalizeConditionEntryAgainstSource(source, entry)
	case ConditionModeCandidate:
		return newCandidateCondition(anyTrue(entry.Mask), entry.Candidate), nil
	default:
		return entry, nil
	}
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
	if fromCompare {
		entry, err := buildConditionEntry(source, cond, candidate, true)
		if err != nil {
			return err
		}
		s.conditionStack.Push(entry)
		return nil
	} else {
		entry, err := buildConditionEntry(source, cond, candidate, false)
		if err != nil {
			return err
		}
		s.conditionStack.Push(entry)
		return nil
	}
}

func (s *SFFrame) popCondition() ConditionEntry {
	return s.conditionStack.Pop()
}

func (s *SFFrame) popConditionForSourceMode(source ValueOperator, mode ConditionMode) (ConditionEntry, error) {
	entry := s.popCondition()
	return coerceConditionEntryToMode(source, entry, mode)
}

func invertMask(mask []bool) []bool {
	out := make([]bool, len(mask))
	for i := 0; i < len(mask); i++ {
		out[i] = !mask[i]
	}
	return out
}

func (s *SFFrame) applyLogicBangCondition(source ValueOperator) error {
	mode := conditionModeFromSource(source)
	normalized, err := s.popConditionForSourceMode(source, mode)
	if err != nil {
		return err
	}
	if mode == ConditionModeCandidate {
		// Candidate-mode "!" stays conservative and only flips truthiness.
		return s.pushCondition(source, []bool{!normalized.Matched}, nil, false)
	} else {
		return s.pushCondition(source, invertMask(normalized.Mask), nil, false)
	}
}

func (s *SFFrame) applyLogicBinaryCondition(source ValueOperator, andMode bool) error {
	mode := conditionModeFromSource(source)
	left, err := s.popConditionForSourceMode(source, mode)
	if err != nil {
		return err
	}
	right, err := s.popConditionForSourceMode(source, mode)
	if err != nil {
		return err
	}

	mergedCandidate := mergeValuesByID(right.Candidate, left.Candidate, andMode)
	if mode == ConditionModeCandidate {
		matched := left.Matched && right.Matched
		if !andMode {
			matched = left.Matched || right.Matched
		}
		return s.pushCondition(source, []bool{matched}, mergedCandidate, false)
	} else {
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
		return s.pushCondition(source, res, mergedCandidate, false)
	}
}

func buildFilterMask(source ValueOperator, cond ValueOperator) ([]bool, error) {
	srcValues := flattenValues(source)
	sourceByID := buildSourceIDIndex(srcValues)
	mask := make([]bool, len(srcValues))
	usedAnyCondition := false

	if err := cond.Recursive(func(operator ValueOperator) error {
		if utils.IsNil(operator) {
			return nil
		}
		if operator.IsEmpty() {
			return nil
		}
		usedAnyCondition = true

		// Fallback 1: direct ID mapping.
		if idGetter, ok := operator.(ssa.GetIdIF); ok {
			if positions, existed := sourceByID[idGetter.GetId()]; existed {
				for _, pos := range positions {
					if pos >= 0 && pos < len(mask) {
						mask[pos] = true
					}
				}
				return nil
			}
		}

		// Fallback 2: bounded predecessor traversal.
		// To avoid accidental match amplification, only accept unique source position.
		reachableIDs := collectReachableSourceIDs(operator)
		if len(reachableIDs) == 0 {
			return nil
		}
		posSet := make(map[int]struct{})
		for _, id := range reachableIDs {
			for _, pos := range sourceByID[id] {
				if pos >= 0 && pos < len(mask) {
					posSet[pos] = struct{}{}
				}
			}
		}
		if len(posSet) == 1 {
			for pos := range posSet {
				mask[pos] = true
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Program-like source (single entry without source IDs): any condition hit means true.
	if len(mask) == 1 && len(sourceByID) == 0 && usedAnyCondition {
		mask[0] = true
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
	mode := conditionModeFromSource(value)
	normalized, err := s.popConditionForSourceMode(value, mode)
	if err != nil {
		return nil, err
	}

	if mode == ConditionModeCandidate {
		if normalized.Matched && !utils.IsNil(normalized.Candidate) && !normalized.Candidate.IsEmpty() {
			return normalized.Candidate, nil
		} else {
			return NewEmptyValues(), nil
		}
	} else {
		return filterValueByMask(value, normalized.Mask)
	}
}
