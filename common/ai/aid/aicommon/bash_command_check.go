package aicommon

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	// TerminationReasonBlockedByGuard is the semantic result marker checked by
	// the ReAct layer after a preflight denial.
	TerminationReasonBlockedByGuard = "blocked_by_bash_guard"

	maxCommandBytes       = 64 * 1024
	maxFindingValueBytes  = 512
	disableBashGuardEnv   = "YAK_DISABLE_AI_BASH_COMMAND_GUARD"
	bashGuardRulesTTL     = 5 * time.Minute
)

// BashCommandCheckOption changes which rule sources participate in one check.
type BashCommandCheckOption func(*bashCommandCheckOptions)

type bashCommandCheckOptions struct {
	disableBuiltin bool
	includeCustom  bool
}

// WithDisableBuiltinBashRules skips embedded/DB built-in Bash SyntaxFlow rules.
func WithDisableBuiltinBashRules(disable bool) BashCommandCheckOption {
	return func(o *bashCommandCheckOptions) { o.disableBuiltin = disable }
}

// WithIncludeCustomBashRules includes user-authored Bash SyntaxFlow rules and
// GUI regex rows compiled into temporary SyntaxFlow rules.
func WithIncludeCustomBashRules(include bool) BashCommandCheckOption {
	return func(o *bashCommandCheckOptions) { o.includeCustom = include }
}

func resolveBashCommandCheckOptions(options []BashCommandCheckOption) bashCommandCheckOptions {
	var opts bashCommandCheckOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	return opts
}

var (
	bashGuardOnce  sync.Once
	bashGuardRules []*schema.SyntaxFlowRule
	bashGuardErr   error

	bashBuiltinRulesCache bashRuleCache
	bashCustomRulesCache  bashRuleCache
)

type bashRuleCache struct {
	mu       sync.Mutex
	rules    []*schema.SyntaxFlowRule
	loaded   bool
	loadedAt time.Time
}

func (c *bashRuleCache) get(loader func() ([]*schema.SyntaxFlowRule, error)) ([]*schema.SyntaxFlowRule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if c.loaded && now.Sub(c.loadedAt) < bashGuardRulesTTL {
		return c.rules, nil
	}
	rules, err := loader()
	if err != nil {
		return nil, err
	}
	c.rules = rules
	c.loaded = true
	c.loadedAt = now
	return rules, nil
}

func loadEmbeddedBashGuardRules() {
	rules, err := sfbuildin.LoadEmbeddedRulesByLanguage(string(ssaconfig.BASH), schema.SFR_SEVERITY_HIGH)
	if err != nil {
		bashGuardErr = fmt.Errorf("load embedded bash guard rules: %w", err)
		return
	}
	if len(rules) == 0 {
		bashGuardErr = fmt.Errorf("no embedded bash guard high rules")
		return
	}
	bashGuardRules = rules
}

// BashCommandFinding is one SyntaxFlow hit reduced to plain data for tool results.
type BashCommandFinding struct {
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

// BashCommandCheckResult is the outcome of checking one shell command through
// SyntaxFlow. Available is false when the checker itself is not usable.
type BashCommandCheckResult struct {
	Available bool                      `json:"available"`
	Blocked   bool                      `json:"blocked"`
	Action    schema.SyntaxFlowAction   `json:"action"`
	Severity  schema.SyntaxFlowSeverity `json:"severity"`
	Findings  []BashCommandFinding      `json:"findings,omitempty"`
	Reason    string                    `json:"reason,omitempty"`
}

// TopFinding returns the finding that determined the action.
func (r BashCommandCheckResult) TopFinding() *BashCommandFinding {
	if len(r.Findings) == 0 {
		return nil
	}
	return &r.Findings[0]
}

// NeedsConfirmation reports whether a human must approve the command.
func (r BashCommandCheckResult) NeedsConfirmation() bool {
	return r.Action == schema.SFR_ACTION_ASK
}

// IsWarning reports whether the command may proceed but should surface a warning.
func (r BashCommandCheckResult) IsWarning() bool {
	return r.Action == schema.SFR_ACTION_WARN
}

// Message is a compact human/LLM-readable summary of the hit rules.
func (r BashCommandCheckResult) Message() string {
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
// command is rejected before execution.
func (r BashCommandCheckResult) BlockedResult(shell string) map[string]any {
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
		"termination_reason":  TerminationReasonBlockedByGuard,
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

func finalizeBashCommandResult(findings []BashCommandFinding) BashCommandCheckResult {
	result := BashCommandCheckResult{Available: true, Action: schema.SFR_ACTION_ALLOW, Severity: schema.SFR_SEVERITY_INFO}
	if len(findings) == 0 {
		return result
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Action.Rank() != findings[j].Action.Rank() {
			return findings[i].Action.Rank() > findings[j].Action.Rank()
		}
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity.Rank() > findings[j].Severity.Rank()
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

func allowedBashCommandResult(reason string) BashCommandCheckResult {
	return BashCommandCheckResult{
		Available: true,
		Action:    schema.SFR_ACTION_ALLOW,
		Severity:  schema.SFR_SEVERITY_INFO,
		Reason:    reason,
	}
}

func unavailableBashCommandResult(reason string) BashCommandCheckResult {
	result := allowedBashCommandResult(reason)
	result.Available = false
	return result
}

// CheckBashCommand parses one shell command and runs high-severity Bash
// SyntaxFlow rules. This is the preflight path wired into the bash tool
// callback; it cannot ask a human, so only deny findings block execution.
func CheckBashCommand(ctx context.Context, cmd string, options ...BashCommandCheckOption) BashCommandCheckResult {
	if isBashGuardDisabled() {
		return allowedBashCommandResult("disabled")
	}
	opts := resolveBashCommandCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return allowedBashCommandResult("disabled_by_config")
	}

	var rules []*schema.SyntaxFlowRule
	builtinErr := error(nil)
	if !opts.disableBuiltin {
		bashGuardOnce.Do(loadEmbeddedBashGuardRules)
		if bashGuardErr != nil {
			builtinErr = bashGuardErr
			log.Warnf("bash command guard unavailable: %v", bashGuardErr)
		} else {
			rules = append(rules, bashGuardRules...)
		}
	}
	if opts.includeCustom {
		rules = append(rules, loadCustomBashSyntaxFlowRules()...)
	}
	if len(rules) == 0 {
		if builtinErr != nil {
			return unavailableBashCommandResult("unavailable: " + builtinErr.Error())
		}
		return allowedBashCommandResult("no_rules_by_config")
	}
	return checkBashCommandWithRules(ctx, cmd, rules)
}

// CheckBuiltinBashCommand evaluates every built-in Bash SyntaxFlow rule from
// the profile DB, plus user-authored rules when explicitly enabled. It is the
// severity-aware gate used by the AI review flow.
func CheckBuiltinBashCommand(ctx context.Context, cmd string, options ...BashCommandCheckOption) BashCommandCheckResult {
	if isBashGuardDisabled() {
		return allowedBashCommandResult("disabled")
	}
	opts := resolveBashCommandCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return allowedBashCommandResult("disabled_by_config")
	}

	var rules []*schema.SyntaxFlowRule
	builtinErr := error(nil)
	fallbackUsed := false
	if !opts.disableBuiltin {
		builtin, err := loadBuiltinBashRules()
		if err == nil && len(builtin) > 0 {
			rules = append(rules, builtin...)
		} else {
			builtinErr = err
			bashGuardOnce.Do(loadEmbeddedBashGuardRules)
			if bashGuardErr == nil && len(bashGuardRules) > 0 {
				rules = append(rules, bashGuardRules...)
				fallbackUsed = true
			} else if err != nil {
				builtinErr = err
			} else {
				builtinErr = fmt.Errorf("no embedded bash guard high rules")
			}
		}
	}
	if opts.includeCustom {
		rules = append(rules, loadCustomBashSyntaxFlowRules()...)
	}

	if len(rules) == 0 {
		if builtinErr != nil {
			log.Warnf("bash built-in rule check unavailable: %v", builtinErr)
			return unavailableBashCommandResult("builtin_unavailable: " + builtinErr.Error())
		}
		return allowedBashCommandResult("no_rules_by_config")
	}
	result := checkBashCommandWithRules(ctx, cmd, rules)
	if !result.Available {
		return result
	}
	if fallbackUsed {
		if builtinErr != nil {
			result.Reason = "builtin_unavailable_using_embedded_high: " + builtinErr.Error()
		} else {
			result.Reason = "no_builtin_bash_rules_using_embedded_high"
		}
	}
	return result
}

func checkBashCommandWithRules(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) BashCommandCheckResult {
	cmd = normalizeBashCommand(cmd)
	if strings.TrimSpace(cmd) == "" {
		return allowedBashCommandResult("empty")
	}
	if len(cmd) > maxCommandBytes {
		log.Warnf("bash command guard skipped oversized command: %d bytes", len(cmd))
		return allowedBashCommandResult("too_large")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(rules) == 0 {
		return allowedBashCommandResult("no_rules")
	}

	prog, err := ssaapi.Parse(cmd,
		ssaapi.WithLanguage(ssaconfig.BASH),
		ssaapi.WithMemory(),
		ssaapi.WithContext(ctx),
	)
	if err != nil {
		log.Warnf("bash command guard skipped unparsable command: %v", err)
		return allowedBashCommandResult("parse_failed: " + err.Error())
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		log.Warnf("bash command guard skipped language mismatch: %s", prog.GetLanguage())
		return allowedBashCommandResult("language_mismatch: " + string(prog.GetLanguage()))
	}

	var findings []BashCommandFinding
	seen := make(map[string]struct{})
	for _, rule := range rules {
		if rule == nil || strings.TrimSpace(rule.Content) == "" {
			continue
		}
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
				finding := buildBashCommandFinding(rule, info, alertName, value)
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
	return finalizeBashCommandResult(findings)
}

func loadBuiltinBashRules() ([]*schema.SyntaxFlowRule, error) {
	return bashBuiltinRulesCache.get(func() ([]*schema.SyntaxFlowRule, error) {
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
		return rules, nil
	})
}

func loadCustomDBBashRules() ([]*schema.SyntaxFlowRule, error) {
	return bashCustomRulesCache.get(func() ([]*schema.SyntaxFlowRule, error) {
		all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
		if err != nil {
			return nil, fmt.Errorf("load custom bash rules: %w", err)
		}
		return filterCustomBashRules(all), nil
	})
}

func filterCustomBashRules(all []*schema.SyntaxFlowRule) []*schema.SyntaxFlowRule {
	rules := make([]*schema.SyntaxFlowRule, 0, len(all))
	for _, rule := range all {
		if rule == nil || rule.IsBuildInRule {
			continue
		}
		if strings.TrimSpace(rule.Content) == "" {
			continue
		}
		if rule.Type != "" && rule.Type != schema.SFR_RULE_TYPE_SF {
			continue
		}
		if rule.Mode != "" && rule.Mode != schema.SFR_MODE_SSA {
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

// loadCustomBashSyntaxFlowRules merges user SF rows from the profile DB with
// GUI regex rows compiled into temporary SyntaxFlow rules.
func loadCustomBashSyntaxFlowRules() []*schema.SyntaxFlowRule {
	var rules []*schema.SyntaxFlowRule
	dbRules, err := loadCustomDBBashRules()
	if err != nil {
		log.Warnf("load custom bash rules failed: %v", err)
	} else {
		rules = append(rules, dbRules...)
	}
	rules = append(rules, loadCompiledCustomRegexSyntaxFlowRules()...)
	return rules
}

func buildBashCommandFinding(rule *schema.SyntaxFlowRule, info *schema.SyntaxFlowDescInfo, alertName string, value *ssaapi.Value) BashCommandFinding {
	severity := schema.ValidSeverityType(rule.Severity)
	finding := BashCommandFinding{
		RuleID:   rule.RuleId,
		Level:    string(severity),
		Severity: severity,
		Action:   schema.SyntaxFlowActionForSeverity(severity),
		Title:    rule.Title,
		TitleZh:  rule.TitleZh,
		Message:  rule.Description,
		Risk:     rule.RiskType,
		Value:    truncateBashFindingValue(value.String(), maxFindingValueBytes),
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

func normalizeBashCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
	return strings.ReplaceAll(cmd, "\r", "\n")
}

func isBashGuardDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(disableBashGuardEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func truncateBashFindingValue(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
