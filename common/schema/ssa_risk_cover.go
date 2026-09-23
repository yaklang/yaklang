package schema

import "strings"

// Scan mode precision. A later stage is more precise and may not finish.
// source is the floor, struct is the compile-unit check, ssa is full dataflow.
const (
	scanModeRankSource = 1
	scanModeRankStruct = 2
	scanModeRankSSA    = 3
)

// ScanModeRank returns the precision rank of a rule scan mode.
// Empty mode is ssa, matching ValidRuleMode.
func ScanModeRank(mode string) int {
	switch ValidRuleMode(mode) {
	case SFR_MODE_SOURCE:
		return scanModeRankSource
	case SFR_MODE_STRUCT:
		return scanModeRankStruct
	default:
		return scanModeRankSSA
	}
}

// CoverAction is what to do with an already kept risk when a new one arrives.
type CoverAction int

const (
	// CoverKeepBoth means the two risks are different findings.
	CoverKeepBoth CoverAction = iota
	// CoverReplaceOld drops the earlier risk and keeps the new one.
	CoverReplaceOld
	// CoverDropNew keeps the earlier risk and ignores the new one.
	CoverDropNew
)

// SameRiskFeature reports whether two risks are the same error at the same
// place inside one program.
//
// RiskFeatureHash matches a function and SSA value without the rule name, so
// struct and ssa rules that alert the same instruction share it. The file and
// the reported position must also match: two sibling statements that share a
// function and an instruction text hash alike but are different findings.
//
// A source-text hit carries no SSA value and therefore a different hash; it
// matches an SSA hit on the same file line and risk type, which is the same
// place in the code.
func SameRiskFeature(a, b *SSARisk) bool {
	if a == nil || b == nil {
		return false
	}
	if a.ProgramName != b.ProgramName {
		return false
	}
	if hash := strings.TrimSpace(a.RiskFeatureHash); hash != "" && hash == strings.TrimSpace(b.RiskFeatureHash) {
		if a.CodeSourceUrl != "" && b.CodeSourceUrl != "" && a.CodeSourceUrl != b.CodeSourceUrl {
			return false
		}
		if !sameRiskPosition(a, b) {
			return false
		}
		return true
	}
	return sameRiskLocation(a, b)
}

// sameRiskPosition compares the reported code range. It is the same JSON in
// every stage of one scan, so equality means both rows point at one spot.
func sameRiskPosition(a, b *SSARisk) bool {
	rangeA := strings.TrimSpace(a.CodeRange)
	rangeB := strings.TrimSpace(b.CodeRange)
	if rangeA != "" && rangeB != "" {
		return rangeA == rangeB
	}
	return a.Line > 0 && a.Line == b.Line
}

// sameRiskLocation compares the place a source-text hit lands on. Such a hit
// has no SSA value, so it cannot be tied to a code range the way an SSA row
// can; two rules matching the same file line with the same risk type are the
// same error there.
func sameRiskLocation(a, b *SSARisk) bool {
	return a.Line > 0 && a.Line == b.Line &&
		a.RiskType != "" && a.RiskType == b.RiskType &&
		a.CodeSourceUrl != "" && a.CodeSourceUrl == b.CodeSourceUrl
}

// CoverActionFor decides whether the incoming risk replaces an existing one.
// The same error at the same place keeps the later, more precise row.
// A higher mode always wins. The same mode keeps the later row, so a finished
// deep stage replaces the copy it already recorded during the struct stage.
//
// Callers that can see two different rules of one mode must not let them cover
// each other; they are separate findings. See SarifReport.prepareCover.
func CoverActionFor(existing, incoming *SSARisk) CoverAction {
	if !SameRiskFeature(existing, incoming) {
		return CoverKeepBoth
	}
	existingRank := ScanModeRank(existing.ScanMode)
	incomingRank := ScanModeRank(incoming.ScanMode)
	if incomingRank < existingRank {
		return CoverDropNew
	}
	return CoverReplaceOld
}

// CoverSSARisks applies cover in scan order. Risks that never get a later
// match stay, which is the floor when a deeper stage does not finish.
func CoverSSARisks(risks []*SSARisk) []*SSARisk {
	kept := make([]*SSARisk, 0, len(risks))
	for _, incoming := range risks {
		if incoming == nil {
			continue
		}
		next := make([]*SSARisk, 0, len(kept)+1)
		dropNew := false
		for _, old := range kept {
			switch CoverActionFor(old, incoming) {
			case CoverDropNew:
				dropNew = true
				next = append(next, old)
			case CoverReplaceOld:
			default:
				next = append(next, old)
			}
		}
		if !dropNew {
			next = append(next, incoming)
		}
		kept = next
	}
	return kept
}
