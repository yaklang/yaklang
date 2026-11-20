package commandcheck

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed rules/*.sf
var guardRuleFS embed.FS

const (
	// maxCommandBytes bounds the SSA build. The AI's bash tool usually receives
	// short commands or small scripts; anything larger is allowed through
	// unchecked rather than stalling the tool call.
	maxCommandBytes = 64 * 1024

	disableEnv = "YAK_DISABLE_AI_BASH_COMMAND_GUARD"

	// builtinRulesTTL keeps the profile DB lookup out of every tool call while
	// still picking up rule edits without a restart.
	builtinRulesTTL = 5 * time.Minute
)

var (
	guardOnce  sync.Once
	guardRules []*schema.SyntaxFlowRule
	guardErr   error

	builtinRulesMu       sync.Mutex
	builtinRules         []*schema.SyntaxFlowRule
	builtinRulesLoaded   bool
	builtinRulesLoadedAt time.Time
)

func loadGuardRules() {
	entries, err := fs.ReadDir(guardRuleFS, "rules")
	if err != nil {
		guardErr = fmt.Errorf("read embedded bash guard rules: %w", err)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			continue
		}
		raw, err := guardRuleFS.ReadFile("rules/" + entry.Name())
		if err != nil {
			guardErr = fmt.Errorf("read bash guard rule %s: %w", entry.Name(), err)
			return
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			guardErr = fmt.Errorf("compile bash guard rule %s: %w", entry.Name(), err)
			return
		}
		guardRules = append(guardRules, rule)
	}
	if len(guardRules) == 0 {
		guardErr = fmt.Errorf("no embedded bash guard rules")
	}
}

// CheckCommand parses one shell command and runs the embedded high-severity
// bash rules against it. This is the preflight path wired into the bash tool
// callback; it cannot ask a human, so only deny findings block execution.
func CheckCommand(ctx context.Context, cmd string) Result {
	guardOnce.Do(loadGuardRules)

	if isGuardDisabled() {
		return allowedResult("disabled")
	}
	if guardErr != nil {
		log.Warnf("bash command guard unavailable: %v", guardErr)
		return unavailableResult("unavailable: " + guardErr.Error())
	}
	return checkWithRules(ctx, cmd, guardRules)
}

// CheckBuiltinCommand evaluates every built-in Bash rule from the profile DB.
// It is the severity-aware gate used by the AI review flow, where middle
// findings can be escalated to a human and low findings can warn.
func CheckBuiltinCommand(ctx context.Context, cmd string) Result {
	if isGuardDisabled() {
		return allowedResult("disabled")
	}

	rules, err := loadBuiltinRules()
	if err == nil && len(rules) > 0 {
		return checkWithRules(ctx, cmd, rules)
	}

	// While the profile DB is unavailable (or has not synced yet), keep the
	// high-severity deny path working with the embedded rule set. Middle and
	// low rules are unavailable in that state.
	guardOnce.Do(loadGuardRules)
	if guardErr == nil && len(guardRules) > 0 {
		result := checkWithRules(ctx, cmd, guardRules)
		if !result.Available {
			return result
		}
		if err != nil {
			result.Reason = "builtin_unavailable_using_embedded_high: " + err.Error()
		} else {
			result.Reason = "no_builtin_bash_rules_using_embedded_high"
		}
		return result
	}
	if err != nil {
		log.Warnf("bash built-in rule check unavailable: %v", err)
		return unavailableResult("builtin_unavailable: " + err.Error())
	}
	return unavailableResult("no_builtin_bash_rules")
}

// CheckCommandWithRules is the deterministic evaluator used by tests and by
// callers that already hold a rule set (for example future user-configured
// temporary rules).
func CheckCommandWithRules(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) Result {
	if isGuardDisabled() {
		return allowedResult("disabled")
	}
	if len(rules) == 0 {
		return unavailableResult("no_rules")
	}
	return checkWithRules(ctx, cmd, rules)
}

func checkWithRules(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) Result {
	cmd = normalizeCommand(cmd)
	if strings.TrimSpace(cmd) == "" {
		return allowedResult("empty")
	}
	if len(cmd) > maxCommandBytes {
		log.Warnf("bash command guard skipped oversized command: %d bytes", len(cmd))
		return allowedResult("too_large")
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
		log.Warnf("bash command guard skipped unparsable command: %v", err)
		return allowedResult("parse_failed: " + err.Error())
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		log.Warnf("bash command guard skipped language mismatch: %s", prog.GetLanguage())
		return allowedResult("language_mismatch: " + string(prog.GetLanguage()))
	}

	var findings []Finding
	seen := make(map[string]struct{})
	for _, rule := range rules {
		if rule == nil || strings.TrimSpace(rule.Content) == "" {
			continue
		}
		// The rule loader may compile opcodes into the rule it receives. Work
		// on a copy so concurrent tool calls never race on shared DB records.
		ruleCopy := *rule
		res, err := prog.SyntaxFlowRule(&ruleCopy)
		if err != nil {
			log.Warnf("bash command guard rule %s failed: %v", rule.RuleName, err)
			continue
		}
		if res == nil {
			continue
		}
		res.GetAlertValues().ForEach(func(alertName string, values ssaapi.Values) bool {
			info, _ := res.GetAlertInfo(alertName)
			for _, value := range values {
				finding := buildFinding(rule, info, alertName, value)
				key := fmt.Sprintf("%s|%s|%s|%d|%d", finding.RuleID, alertName, finding.Value, finding.Start, finding.End)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				findings = append(findings, finding)
			}
			return true
		})
	}
	return finalizeResult(findings)
}

// loadBuiltinRules returns the built-in Bash rules from the profile database.
// User rules are intentionally excluded until the guard gets an explicit
// user-rule configuration path.
func loadBuiltinRules() ([]*schema.SyntaxFlowRule, error) {
	builtinRulesMu.Lock()
	defer builtinRulesMu.Unlock()

	now := time.Now()
	if builtinRulesLoaded && now.Sub(builtinRulesLoadedAt) < builtinRulesTTL {
		return builtinRules, nil
	}

	all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
	if err != nil {
		return nil, fmt.Errorf("load built-in bash rules: %w", err)
	}
	rules := make([]*schema.SyntaxFlowRule, 0, len(all))
	for _, rule := range all {
		if rule == nil || !rule.IsBuildInRule {
			continue
		}
		if strings.TrimSpace(rule.Content) == "" {
			continue
		}
		rules = append(rules, rule)
	}

	builtinRules = rules
	builtinRulesLoaded = true
	builtinRulesLoadedAt = now
	return rules, nil
}

func buildFinding(rule *schema.SyntaxFlowRule, info *schema.SyntaxFlowDescInfo, alertName string, value *ssaapi.Value) Finding {
	severity := schema.ValidSeverityType(rule.Severity)
	finding := Finding{
		RuleID:   rule.RuleId,
		Level:    string(severity),
		Severity: severity,
		Action:   schema.SyntaxFlowActionForSeverity(severity),
		Title:    rule.Title,
		TitleZh:  rule.TitleZh,
		Message:  rule.Description,
		Risk:     rule.RiskType,
		Value:    truncateValue(value.String(), 512),
	}
	if info != nil {
		if info.RuleId != "" {
			finding.RuleID = info.RuleId
		}
		if info.Title != "" {
			finding.Title = info.Title
		}
		if info.TitleZh != "" {
			finding.TitleZh = info.TitleZh
		}
		if info.RiskType != "" {
			finding.Risk = info.RiskType
		}
		if info.Severity != "" {
			severity = schema.ValidSeverityType(info.Severity)
			finding.Level = string(severity)
			finding.Severity = severity
			finding.Action = schema.SyntaxFlowActionForSeverity(severity)
		}
		switch {
		case info.Msg != "":
			finding.Message = info.Msg
		case info.Description != "":
			finding.Message = info.Description
		}
	}
	if finding.RuleID == "" {
		finding.RuleID = alertName
	}
	if r := value.GetRange(); r != nil {
		finding.Start = r.GetStartOffset()
		finding.End = r.GetEndOffset()
	}
	return finding
}

func allowedResult(reason string) Result {
	return Result{
		Available: true,
		Action:    schema.SFR_ACTION_ALLOW,
		Severity:  schema.SFR_SEVERITY_INFO,
		Reason:    reason,
	}
}

func unavailableResult(reason string) Result {
	result := allowedResult(reason)
	result.Available = false
	return result
}

func normalizeCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
	return strings.ReplaceAll(cmd, "\r", "\n")
}

func isGuardDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(disableEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func truncateValue(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "") + "..."
}
