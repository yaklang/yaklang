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
// struct and ssa rules that alert the same instruction share it. The file
// must also match, otherwise two copies of the same function stay separate.
// A source-text hit and an SSA hit on the same file line and risk type match
// even when their hashes differ, because the text hit is not an SSA value.
func SameRiskFeature(a, b *SSARisk) bool {
	if a == nil || b == nil {
		return false
	}
	if a.ProgramName != b.ProgramName {
		return false
	}
	if hash := strings.TrimSpace(a.RiskFeatureHash); hash != "" && hash == strings.TrimSpace(b.RiskFeatureHash) {
		if a.CodeSourceUrl == "" || b.CodeSourceUrl == "" || a.CodeSourceUrl == b.CodeSourceUrl {
			return true
		}
	}
	if a.Line > 0 && a.Line == b.Line &&
		a.RiskType != "" && a.RiskType == b.RiskType &&
		a.CodeSourceUrl != "" && a.CodeSourceUrl == b.CodeSourceUrl {
		return true
	}
	return false
}

// CoverActionFor decides whether the incoming risk replaces an existing one.
// The same error at the same place keeps the later, more precise row.
// A higher mode always wins. The same mode keeps the later row, so a finished
// deep stage replaces the copy it already recorded during the struct stage.
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
