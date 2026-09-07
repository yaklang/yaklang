package sfbuildin

// BuiltinRiskCheckResult aggregates the risk-type governance check over a rule
// directory: how many rules/alerts were inspected and every violation found.
type BuiltinRiskCheckResult struct {
	RuleCount      int
	AlertCount     int
	LibraryCount   int
	CanonicalTypes []string
	Violations     []string
}
