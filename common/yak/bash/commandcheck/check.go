package commandcheck

import (
	"context"
	"fmt"
	"os"
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
	// maxCommandBytes bounds the SSA build. The AI's bash tool usually receives
	// short commands or small scripts; anything larger is allowed through
	// unchecked rather than stalling the tool call.
	maxCommandBytes = 64 * 1024

	disableEnv = "YAK_DISABLE_AI_BASH_COMMAND_GUARD"

	// rulesTTL keeps the profile DB lookup out of every tool call while still
	// picking up rule edits without a restart.
	rulesTTL = 5 * time.Minute
)

// CheckOption changes which rule sources participate in one command check.
// The zero value keeps the historical behavior: embedded/DB built-in rules are
// enabled and user-authored rules are excluded.
type CheckOption func(*checkOptions)

type checkOptions struct {
	disableBuiltin bool
	includeCustom  bool
}

// WithDisableBuiltinRules skips all built-in Bash rules, including the embedded
// high-severity fallback. It only makes sense together with
// WithIncludeCustomRules when the caller wants custom rules alone.
func WithDisableBuiltinRules(disable bool) CheckOption {
	return func(o *checkOptions) { o.disableBuiltin = disable }
}

// WithIncludeCustomRules includes user-authored rules from the profile DB:
// Bash SyntaxFlow rules plus the structured regex rows edited on the AI Bash
// guard page. Only SSA-mode SyntaxFlow rules are eligible.
func WithIncludeCustomRules(include bool) CheckOption {
	return func(o *checkOptions) { o.includeCustom = include }
}

func resolveCheckOptions(options []CheckOption) checkOptions {
	var opts checkOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	return opts
}

var (
	guardOnce  sync.Once
	guardRules []*schema.SyntaxFlowRule
	guardErr   error

	builtinRulesCache ruleCache
	customRulesCache  ruleCache
)

type ruleCache struct {
	mu       sync.Mutex
	rules    []*schema.SyntaxFlowRule
	loaded   bool
	loadedAt time.Time
}

func (c *ruleCache) get(loader func() ([]*schema.SyntaxFlowRule, error)) ([]*schema.SyntaxFlowRule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if c.loaded && now.Sub(c.loadedAt) < rulesTTL {
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

func loadGuardRules() {
	rules, err := sfbuildin.LoadEmbeddedRulesByLanguage(string(ssaconfig.BASH), schema.SFR_SEVERITY_HIGH)
	if err != nil {
		guardErr = fmt.Errorf("load embedded bash guard rules: %w", err)
		return
	}
	if len(rules) == 0 {
		guardErr = fmt.Errorf("no embedded bash guard high rules")
		return
	}
	guardRules = rules
}

// CheckCommand parses one shell command and runs high-severity Bash rules plus
// custom deny rows. This is the preflight path wired into the bash tool
// callback; it cannot ask a human, so only deny findings block execution.
func CheckCommand(ctx context.Context, cmd string, options ...CheckOption) Result {
	if isGuardDisabled() {
		return allowedResult("disabled")
	}
	opts := resolveCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return allowedResult("disabled_by_config")
	}

	var rules []*schema.SyntaxFlowRule
	var regexRules []compiledRegexRule
	builtinErr := error(nil)
	if !opts.disableBuiltin {
		guardOnce.Do(loadGuardRules)
		if guardErr != nil {
			builtinErr = guardErr
			log.Warnf("bash command guard unavailable: %v", guardErr)
		} else {
			rules = append(rules, guardRules...)
		}
	}
	if opts.includeCustom {
		custom, compiledRegex := loadCustomRuleSet()
		for _, rule := range custom {
			if schema.ValidSeverityType(rule.Severity).Rank() >= schema.SFR_SEVERITY_HIGH.Rank() {
				rules = append(rules, rule)
			}
		}
		regexRules = compiledRegex
	}
	if len(rules) == 0 && len(regexRules) == 0 {
		if builtinErr != nil {
			return unavailableResult("unavailable: " + builtinErr.Error())
		}
		return allowedResult("no_rules_by_config")
	}
	return checkWithRules(ctx, cmd, rules, regexRules)
}

// CheckBuiltinCommand evaluates every built-in Bash rule from the profile DB,
// plus user-authored rules when explicitly enabled. It is the severity-aware
// gate used by the AI review flow, where middle findings can be escalated to a
// human and low findings can warn.
func CheckBuiltinCommand(ctx context.Context, cmd string, options ...CheckOption) Result {
	if isGuardDisabled() {
		return allowedResult("disabled")
	}
	opts := resolveCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return allowedResult("disabled_by_config")
	}

	var rules []*schema.SyntaxFlowRule
	var regexRules []compiledRegexRule
	builtinErr := error(nil)
	fallbackUsed := false
	if !opts.disableBuiltin {
		builtin, err := loadBuiltinRules()
		if err == nil && len(builtin) > 0 {
			rules = append(rules, builtin...)
		} else {
			builtinErr = err
			// While the profile DB is unavailable (or has not synced yet), keep the
			// high-severity deny path working with the embedded rule set. Middle and
			// low rules are unavailable in that state.
			guardOnce.Do(loadGuardRules)
			if guardErr == nil && len(guardRules) > 0 {
				rules = append(rules, guardRules...)
				fallbackUsed = true
			} else if err != nil {
				builtinErr = err
			} else {
				builtinErr = fmt.Errorf("no embedded bash guard high rules")
			}
		}
	}
	if opts.includeCustom {
		custom, compiledRegex := loadCustomRuleSet()
		rules = append(rules, custom...)
		regexRules = compiledRegex
	}

	if len(rules) == 0 && len(regexRules) == 0 {
		if builtinErr != nil {
			log.Warnf("bash built-in rule check unavailable: %v", builtinErr)
			return unavailableResult("builtin_unavailable: " + builtinErr.Error())
		}
		return allowedResult("no_rules_by_config")
	}
	result := checkWithRules(ctx, cmd, rules, regexRules)
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

func checkWithRules(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule, regexRules []compiledRegexRule) Result {
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

	// Regex rows read the raw command text and stay available even when the
	// bash SSA frontend rejects the command.
	findings := matchRegexRules(cmd, regexRules)
	if len(rules) == 0 {
		return finalizeResult(findings)
	}

	prog, err := ssaapi.Parse(cmd,
		ssaapi.WithLanguage(ssaconfig.BASH),
		ssaapi.WithMemory(),
		ssaapi.WithContext(ctx),
	)
	if err != nil {
		log.Warnf("bash command guard skipped unparsable command: %v", err)
		if len(findings) > 0 {
			return finalizeResult(findings)
		}
		return allowedResult("parse_failed: " + err.Error())
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		log.Warnf("bash command guard skipped language mismatch: %s", prog.GetLanguage())
		if len(findings) > 0 {
			return finalizeResult(findings)
		}
		return allowedResult("language_mismatch: " + string(prog.GetLanguage()))
	}

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
func loadBuiltinRules() ([]*schema.SyntaxFlowRule, error) {
	return builtinRulesCache.get(func() ([]*schema.SyntaxFlowRule, error) {
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

// loadCustomRules returns user-authored Bash SyntaxFlow rules. The guard only
// executes SSA-mode SyntaxFlow rules; Yak rules and source/struct modes are
// intentionally excluded because checkWithRules compiles a bash SSA program.
func loadCustomRules() ([]*schema.SyntaxFlowRule, error) {
	return customRulesCache.get(func() ([]*schema.SyntaxFlowRule, error) {
		all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
		if err != nil {
			return nil, fmt.Errorf("load custom bash rules: %w", err)
		}
		return filterCustomRules(all), nil
	})
}

func filterCustomRules(all []*schema.SyntaxFlowRule) []*schema.SyntaxFlowRule {
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

// loadCustomRuleSet loads user rows once for both entry points. A failed load
// still returns the regex rows, so one broken SyntaxFlow rule cannot disable
// the structured rules edited on the guard page.
func loadCustomRuleSet() ([]*schema.SyntaxFlowRule, []compiledRegexRule) {
	rules, err := loadCustomRules()
	if err != nil {
		log.Warnf("load custom bash rules failed: %v", err)
		rules = nil
	}
	return rules, loadCompiledRegexRules()
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
		Value:    truncateValue(value.String(), maxFindingValueBytes),
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
