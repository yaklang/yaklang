package commandcheck

import "strings"

// Finding is one guard-rule hit, reduced to plain data so it can be embedded
// in a tool result without leaking SSA values.
type Finding struct {
	RuleID  string
	Level   string
	Title   string
	TitleZh string
	Message string
	Value   string
	Start   int
	End     int
}

// Result is the outcome of checking one shell command. Available is false when
// the checker itself is not usable (rule compile failure); callers must treat
// that as "not blocked" and decide their own fail-open policy.
type Result struct {
	Available bool
	Blocked   bool
	Findings  []Finding
	Reason    string
}

// Message is a compact human/LLM-readable summary of the hit rules.
func (r Result) Message() string {
	if len(r.Findings) == 0 {
		return "command allowed"
	}
	parts := make([]string, 0, len(r.Findings))
	seen := make(map[string]struct{}, len(r.Findings))
	for _, finding := range r.Findings {
		title := finding.TitleZh
		if title == "" {
			title = finding.Title
		}
		if title == "" {
			title = finding.RuleID
		}
		if title == "" {
			continue
		}
		if _, ok := seen[title]; ok {
			continue
		}
		seen[title] = struct{}{}
		parts = append(parts, title)
	}
	if len(parts) == 0 {
		return "command blocked by bash guard"
	}
	return "command blocked by bash guard: " + strings.Join(parts, "; ")
}

// BlockedResult builds the semantic map returned to the AI tool caller when a
// command is rejected. Keeping it in the same shape as bash.yak's RESULT lets
// the existing execution-status and timeline code report a failed invocation.
func (r Result) BlockedResult(shell string) map[string]any {
	if shell == "" {
		shell = "bash"
	}
	findings := make([]map[string]any, 0, len(r.Findings))
	for _, finding := range r.Findings {
		findings = append(findings, map[string]any{
			"rule_id":  finding.RuleID,
			"level":    finding.Level,
			"title":    finding.Title,
			"title_zh": finding.TitleZh,
			"message":  finding.Message,
			"value":    finding.Value,
			"range": map[string]any{
				"start": finding.Start,
				"end":   finding.End,
			},
		})
	}
	return map[string]any{
		"shell":               shell,
		"executed":            false,
		"process_started":     false,
		"process_completed":   false,
		"timed_out":           false,
		"exit_code":           nil,
		"exit_code_available": false,
		"termination_reason":  "blocked_by_bash_guard",
		"process_error":       r.Message() + ". The command was not executed.",
		"check": map[string]any{
			"verdict":  "block",
			"findings": findings,
		},
	}
}
