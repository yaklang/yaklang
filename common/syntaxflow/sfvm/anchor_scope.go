package sfvm

import "github.com/yaklang/yaklang/common/utils"

type ConditionMode uint8

const (
	ConditionModeMask ConditionMode = iota
	ConditionModeCandidate
)

// conditionModeFromSource decides how an anchor scope represents truth:
//
//   - Mask mode: common case, source is a list -> represent condition as []bool aligned to source slots.
//   - Candidate mode: special singleton sources (e.g. Program/Overlay) where a "mask per slot" is not
//     meaningful; represent condition as a single truth value plus optional candidate values.
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

func (s *SFFrame) activeAnchorScope() (anchorScopeState, bool) {
	if s == nil || s.anchorScope == nil || s.anchorScope.Len() == 0 {
		return anchorScopeState{}, false
	}
	return s.anchorScope.Peek(), true
}
