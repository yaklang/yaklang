package schema

// Rank orders severities so callers that merge findings can keep the most
// severe one. Unknown values rank below info and must never outrank a known
// finding.
func (s SyntaxFlowSeverity) Rank() int {
	switch ValidSeverityType(s) {
	case SFR_SEVERITY_CRITICAL:
		return 5
	case SFR_SEVERITY_HIGH:
		return 4
	case SFR_SEVERITY_WARNING:
		return 3
	case SFR_SEVERITY_LOW:
		return 2
	default:
		return 1
	}
}
