package commandcheck

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// Finding is one guard-rule hit, reduced to plain data so it can be embedded
// in a tool result without leaking SSA values.
type Finding struct {
	RuleID   string                    `json:"rule_id"`
	Level    string                    `json:"level"`
	Severity schema.SyntaxFlowSeverity `json:"severity"`
	Action   schema.SyntaxFlowAction   `json:"action"`
	Title    string                    `json:"title"`
	TitleZh  string                    `json:"title_zh"`
	Message  string                    `json:"message"`
	Risk     string                    `json:"risk"`
	Value    string                    `json:"value"`
	Start    int                       `json:"start"`
	End      int                       `json:"end"`
}

// Result is the outcome of checking one shell command. Available is false when
// the checker itself is not usable (rule compile failure); callers must treat
// that as "not blocked" and decide their own fail-open policy.
//
// Action is the highest-priority action across findings:
//
//	high/critical -> deny
//	middle        -> ask
//	low           -> warn
//	info          -> allow
type Result struct {
	Available bool                      `json:"available"`
	Blocked   bool                      `json:"blocked"`
	Action    schema.SyntaxFlowAction   `json:"action"`
	Severity  schema.SyntaxFlowSeverity `json:"severity"`
	Findings  []Finding                 `json:"findings,omitempty"`
	Reason    string                    `json:"reason,omitempty"`
}

// TopFinding returns the finding that determined the action.
func (r Result) TopFinding() *Finding {
	if len(r.Findings) == 0 {
		return nil
	}
	return &r.Findings[0]
}

// NeedsConfirmation reports whether a human must approve the command.
func (r Result) NeedsConfirmation() bool {
	return r.Action == schema.SFR_ACTION_ASK
}

// IsWarning reports whether the command may proceed but should surface a
// warning.
func (r Result) IsWarning() bool {
	return r.Action == schema.SFR_ACTION_WARN
}

// Message is a compact human/LLM-readable summary of the hit rules.
func (r Result) Message() string {
	if len(r.Findings) == 0 {
		return "command allowed"
	}
	prefix := "command allowed"
	switch r.Action {
	case schema.SFR_ACTION_DENY:
		prefix = "command blocked by bash guard"
	case schema.SFR_ACTION_ASK:
		prefix = "command requires confirmation by bash guard"
	case schema.SFR_ACTION_WARN:
		prefix = "command allowed with bash guard warning"
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
		return prefix
	}
	return prefix + ": " + strings.Join(parts, "; ")
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
			"severity": string(finding.Severity),
			"action":   string(finding.Action),
			"title":    finding.Title,
			"title_zh": finding.TitleZh,
			"message":  finding.Message,
			"risk":     finding.Risk,
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
		"process_error":       r.Message() + ". The command was not executed. Do not retry this command through another shell, tool, encoding, path, script wrapper, MCP, or sub-agent. If the next step is the same operation or a bypass, stop it and tell the user; otherwise continue with unrelated work.",
		"security_denied":     true,
		"check": map[string]any{
			"verdict":  "block",
			"action":   string(r.Action),
			"severity": string(r.Severity),
			"findings": findings,
		},
	}
}

func finalizeResult(findings []Finding) Result {
	result := Result{Available: true, Action: schema.SFR_ACTION_ALLOW, Severity: schema.SFR_SEVERITY_INFO}
	if len(findings) == 0 {
		return result
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Action.Rank() != findings[j].Action.Rank() {
			return findings[i].Action.Rank() > findings[j].Action.Rank()
		}
		if findings[i].Severity != findings[j].Severity {
			return severityRank(findings[i].Severity) > severityRank(findings[j].Severity)
		}
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Start < findings[j].Start
	})
	result.Findings = findings
	result.Action = findings[0].Action
	result.Severity = findings[0].Severity
	result.Blocked = result.Action == schema.SFR_ACTION_DENY
	return result
}

func severityRank(severity schema.SyntaxFlowSeverity) int {
	switch schema.ValidSeverityType(severity) {
	case schema.SFR_SEVERITY_CRITICAL:
		return 4
	case schema.SFR_SEVERITY_HIGH:
		return 3
	case schema.SFR_SEVERITY_WARNING:
		return 2
	case schema.SFR_SEVERITY_LOW:
		return 1
	default:
		return 0
	}
}
