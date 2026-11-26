package aicommon

import (
	"context"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	// TerminationReasonBlockedByGuard marks a bash preflight denial for ReAct.
	TerminationReasonBlockedByGuard = "blocked_by_bash_guard"

	maxBashRuleCommandBytes = 64 * 1024
)

// BashRuleVerdict is the enforcement result returned to AI callers.
type BashRuleVerdict struct {
	Action schema.SyntaxFlowAction `json:"action"`
}

// Blocked reports whether execution must stop.
func (v BashRuleVerdict) Blocked() bool {
	return v.Action == schema.SFR_ACTION_DENY
}

// BlockedResult is the minimal payload returned when a bash tool call is denied.
func (v BashRuleVerdict) BlockedResult(shell string) map[string]any {
	if shell == "" {
		shell = "bash"
	}
	return map[string]any{
		"shell":               shell,
		"executed":            false,
		"process_started":     false,
		"process_completed":   false,
		"timed_out":           false,
		"exit_code":           nil,
		"exit_code_available": false,
		"termination_reason":  TerminationReasonBlockedByGuard,
		"process_error":       "command denied by rule permission policy. The command was not executed. Do not retry this command through another shell, tool, encoding, path, script wrapper, MCP, or sub-agent.",
		"security_denied":     true,
		"verdict":             string(schema.SFR_ACTION_DENY),
	}
}

var (
	bashBuiltinOnce  sync.Once
	bashBuiltinRules []*schema.SyntaxFlowRule
)

// loadBashSFRules loads built-in Bash rules from the synced database and falls
// back to the embedded snapshot when the database is not ready.
func loadBashSFRules() []*schema.SyntaxFlowRule {
	bashBuiltinOnce.Do(func() {
		all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
		if err == nil {
			for _, rule := range all {
				if rule != nil && rule.IsBuildInRule && strings.TrimSpace(rule.Content) != "" {
					bashBuiltinRules = append(bashBuiltinRules, rule)
				}
			}
		}
		if len(bashBuiltinRules) > 0 {
			return
		}
		embedded, err := sfbuildin.LoadEmbeddedRulesByLanguage(string(ssaconfig.BASH))
		if err != nil {
			log.Warnf("load embedded bash rules failed: %v", err)
			return
		}
		bashBuiltinRules = embedded
	})
	return bashBuiltinRules
}

// CheckBashCommand runs built-in SyntaxFlow rules and user-authored GUI rules.
// It returns the strongest action found.
func CheckBashCommand(ctx context.Context, cmd string) BashRuleVerdict {
	cmd = strings.ReplaceAll(strings.ReplaceAll(cmd, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(cmd) == "" || len(cmd) > maxBashRuleCommandBytes {
		return BashRuleVerdict{Action: schema.SFR_ACTION_ALLOW}
	}

	best := checkBashGuardRegexRules(cmd)
	if best != schema.SFR_ACTION_DENY {
		if action := runBashSyntaxFlow(ctx, cmd, loadBashSFRules()); action.Rank() > best.Rank() {
			best = action
		}
	}
	return BashRuleVerdict{Action: best}
}

func runBashSyntaxFlow(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) schema.SyntaxFlowAction {
	if len(rules) == 0 {
		return schema.SFR_ACTION_ALLOW
	}
	if ctx == nil {
		ctx = context.Background()
	}

	prog, err := ssaapi.Parse(cmd,
		ssaapi.WithLanguage(ssaconfig.BASH),
		ssaapi.WithMemory(),
		ssaapi.WithContext(ctx),
	)
	if err != nil {
		log.Warnf("bash rule check skipped unparsable command: %v", err)
		return schema.SFR_ACTION_ALLOW
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		return schema.SFR_ACTION_ALLOW
	}

	best := schema.SFR_ACTION_ALLOW
	for _, rule := range rules {
		if rule == nil || strings.TrimSpace(rule.Content) == "" {
			continue
		}
		ruleCopy := *rule
		res, err := prog.SyntaxFlowRule(&ruleCopy)
		if err != nil || res == nil {
			continue
		}
		action := schema.SyntaxFlowActionForSeverity(schema.ValidSeverityType(rule.Severity))
		res.GetAlertValues().ForEach(func(alertName string, values ssaapi.Values) bool {
			if values == nil || values.Len() == 0 {
				return true
			}
			info, _ := res.GetAlertInfo(alertName)
			hitAction := action
			if info != nil && info.Severity != "" {
				hitAction = schema.SyntaxFlowActionForSeverity(schema.ValidSeverityType(info.Severity))
			}
			if hitAction.Rank() > best.Rank() {
				best = hitAction
			}
			return true
		})
		if best == schema.SFR_ACTION_DENY {
			break
		}
	}
	return best
}
