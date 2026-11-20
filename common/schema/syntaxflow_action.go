package schema

// SyntaxFlowAction is the enforcement action a rule asks its host to take.
// Severity describes impact; action describes what the host does when the rule
// fires. Keeping the mapping in one place lets scans, editors and AI tool
// guards agree on what "high" or "middle" means.
type SyntaxFlowAction string

const (
	// SFR_ACTION_ALLOW records the finding without interrupting the caller.
	SFR_ACTION_ALLOW SyntaxFlowAction = "allow"
	// SFR_ACTION_WARN lets the caller continue but surfaces a warning.
	SFR_ACTION_WARN SyntaxFlowAction = "warn"
	// SFR_ACTION_ASK requires a human decision before the caller continues.
	SFR_ACTION_ASK SyntaxFlowAction = "ask"
	// SFR_ACTION_DENY blocks the caller.
	SFR_ACTION_DENY SyntaxFlowAction = "deny"
)

// Rank orders actions by strictness. A caller that collects several findings
// keeps the one with the highest rank.
func (a SyntaxFlowAction) Rank() int {
	switch a {
	case SFR_ACTION_DENY:
		return 3
	case SFR_ACTION_ASK:
		return 2
	case SFR_ACTION_WARN:
		return 1
	default:
		return 0
	}
}

// IsBlocking reports whether the action prevents the operation outright.
func (a SyntaxFlowAction) IsBlocking() bool {
	return a == SFR_ACTION_DENY
}

// String implements fmt.Stringer for logs and JSON fallbacks.
func (a SyntaxFlowAction) String() string {
	return string(a)
}

// SyntaxFlowActionForSeverity maps a severity to the default enforcement
// action:
//
//	high / critical -> deny
//	middle          -> ask
//	low             -> warn
//	info / unknown  -> allow
//
// Unknown severities normalize to info so a malformed rule can never block a
// caller by accident.
func SyntaxFlowActionForSeverity(severity SyntaxFlowSeverity) SyntaxFlowAction {
	switch ValidSeverityType(severity) {
	case SFR_SEVERITY_CRITICAL, SFR_SEVERITY_HIGH:
		return SFR_ACTION_DENY
	case SFR_SEVERITY_WARNING:
		return SFR_ACTION_ASK
	case SFR_SEVERITY_LOW:
		return SFR_ACTION_WARN
	default:
		return SFR_ACTION_ALLOW
	}
}

// Action returns the rule's effective enforcement action.
func (s *SyntaxFlowRule) Action() SyntaxFlowAction {
	if s == nil {
		return SFR_ACTION_ALLOW
	}
	return SyntaxFlowActionForSeverity(s.Severity)
}
