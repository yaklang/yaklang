package aicommon

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
	// TerminationReasonBlockedByGuard marks a bash preflight denial for ReAct.
	TerminationReasonBlockedByGuard = "blocked_by_bash_guard"

	maxBashRuleCommandBytes = 64 * 1024
	disableBashRuleEnv      = "YAK_DISABLE_AI_BASH_COMMAND_GUARD"
	bashRuleCacheTTL        = 5 * time.Minute
)

// BashRuleCheckOption selects which SyntaxFlow rule sources participate.
type BashRuleCheckOption func(*bashRuleCheckOptions)

type bashRuleCheckOptions struct {
	disableBuiltin bool
	includeCustom  bool
}

// WithDisableBuiltinBashRules skips embedded/DB built-in Bash SyntaxFlow rules.
func WithDisableBuiltinBashRules(disable bool) BashRuleCheckOption {
	return func(o *bashRuleCheckOptions) { o.disableBuiltin = disable }
}

// WithIncludeCustomBashRules includes user SF rules and GUI/runtime rows that
// were compiled into temporary SyntaxFlow rules.
func WithIncludeCustomBashRules(include bool) BashRuleCheckOption {
	return func(o *bashRuleCheckOptions) { o.includeCustom = include }
}

func resolveBashRuleCheckOptions(options []BashRuleCheckOption) bashRuleCheckOptions {
	var opts bashRuleCheckOptions
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}
	return opts
}

var (
	bashEmbeddedOnce  sync.Once
	bashEmbeddedRules []*schema.SyntaxFlowRule
	bashEmbeddedErr   error

	bashBuiltinCache bashSFRuleCache
	bashCustomCache  bashSFRuleCache
)

type bashSFRuleCache struct {
	mu       sync.Mutex
	rules    []*schema.SyntaxFlowRule
	loaded   bool
	loadedAt time.Time
}

func (c *bashSFRuleCache) get(loader func() ([]*schema.SyntaxFlowRule, error)) ([]*schema.SyntaxFlowRule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.loaded && now.Sub(c.loadedAt) < bashRuleCacheTTL {
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

func loadEmbeddedBashRules() {
	rules, err := sfbuildin.LoadEmbeddedRulesByLanguage(string(ssaconfig.BASH), schema.SFR_SEVERITY_HIGH)
	if err != nil {
		bashEmbeddedErr = fmt.Errorf("load embedded bash rules: %w", err)
		return
	}
	if len(rules) == 0 {
		bashEmbeddedErr = fmt.Errorf("no embedded bash high rules")
		return
	}
	bashEmbeddedRules = rules
}

// BashRuleVerdict is the only result returned to AI callers after SyntaxFlow.
// Action is one of: allow / warn / ask / deny.
type BashRuleVerdict struct {
	Available bool                    `json:"available"`
	Action    schema.SyntaxFlowAction `json:"action"`
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

func bashVerdict(action schema.SyntaxFlowAction) BashRuleVerdict {
	if action == "" {
		action = schema.SFR_ACTION_ALLOW
	}
	return BashRuleVerdict{Available: true, Action: action}
}

func bashVerdictUnavailable() BashRuleVerdict {
	return BashRuleVerdict{Available: false, Action: schema.SFR_ACTION_ALLOW}
}

func isBashRuleDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(disableBashRuleEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// CheckBashCommand runs high-severity built-in SyntaxFlow rules (preflight deny).
func CheckBashCommand(ctx context.Context, cmd string, options ...BashRuleCheckOption) BashRuleVerdict {
	if isBashRuleDisabled() {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	opts := resolveBashRuleCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}

	var rules []*schema.SyntaxFlowRule
	if !opts.disableBuiltin {
		bashEmbeddedOnce.Do(loadEmbeddedBashRules)
		if bashEmbeddedErr != nil {
			log.Warnf("bash rule check unavailable: %v", bashEmbeddedErr)
		} else {
			rules = append(rules, bashEmbeddedRules...)
		}
	}
	if opts.includeCustom {
		rules = append(rules, loadAllCustomBashSFRules()...)
	}
	if len(rules) == 0 {
		if !opts.disableBuiltin && bashEmbeddedErr != nil {
			return bashVerdictUnavailable()
		}
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	return runBashSyntaxFlow(ctx, cmd, rules)
}

// CheckBuiltinBashCommand runs all built-in Bash SyntaxFlow rules for review.
func CheckBuiltinBashCommand(ctx context.Context, cmd string, options ...BashRuleCheckOption) BashRuleVerdict {
	if isBashRuleDisabled() {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	opts := resolveBashRuleCheckOptions(options)
	if opts.disableBuiltin && !opts.includeCustom {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}

	var rules []*schema.SyntaxFlowRule
	if !opts.disableBuiltin {
		builtin, err := loadBuiltinBashSFRules()
		if err == nil && len(builtin) > 0 {
			rules = append(rules, builtin...)
		} else {
			bashEmbeddedOnce.Do(loadEmbeddedBashRules)
			if bashEmbeddedErr == nil {
				rules = append(rules, bashEmbeddedRules...)
			} else if len(rules) == 0 {
				log.Warnf("bash built-in rule check unavailable: %v", err)
				if opts.includeCustom {
					rules = append(rules, loadAllCustomBashSFRules()...)
					if len(rules) == 0 {
						return bashVerdictUnavailable()
					}
					return runBashSyntaxFlow(ctx, cmd, rules)
				}
				return bashVerdictUnavailable()
			}
		}
	}
	if opts.includeCustom {
		rules = append(rules, loadAllCustomBashSFRules()...)
	}
	if len(rules) == 0 {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	return runBashSyntaxFlow(ctx, cmd, rules)
}

func runBashSyntaxFlow(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) BashRuleVerdict {
	cmd = strings.ReplaceAll(strings.ReplaceAll(cmd, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(cmd) == "" {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	if len(cmd) > maxBashRuleCommandBytes {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
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
		return bashVerdict(schema.SFR_ACTION_ALLOW)
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		return bashVerdict(schema.SFR_ACTION_ALLOW)
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
	return bashVerdict(best)
}

func loadBuiltinBashSFRules() ([]*schema.SyntaxFlowRule, error) {
	return bashBuiltinCache.get(func() ([]*schema.SyntaxFlowRule, error) {
		all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
		if err != nil {
			return nil, err
		}
		rules := make([]*schema.SyntaxFlowRule, 0, len(all))
		for _, rule := range all {
			if rule == nil || !rule.IsBuildInRule || strings.TrimSpace(rule.Content) == "" {
				continue
			}
			rules = append(rules, rule)
		}
		return rules, nil
	})
}

func loadCustomDBBashSFRules() ([]*schema.SyntaxFlowRule, error) {
	return bashCustomCache.get(func() ([]*schema.SyntaxFlowRule, error) {
		all, err := sfdb.GetRuleByLanguage(string(ssaconfig.BASH))
		if err != nil {
			return nil, err
		}
		rules := make([]*schema.SyntaxFlowRule, 0, len(all))
		for _, rule := range all {
			if rule == nil || rule.IsBuildInRule || strings.TrimSpace(rule.Content) == "" {
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
		return rules, nil
	})
}

func loadAllCustomBashSFRules() []*schema.SyntaxFlowRule {
	var rules []*schema.SyntaxFlowRule
	if dbRules, err := loadCustomDBBashSFRules(); err != nil {
		log.Warnf("load custom bash SF rules failed: %v", err)
	} else {
		rules = append(rules, dbRules...)
	}
	rules = append(rules, loadCompiledCustomRegexSyntaxFlowRules()...)
	rules = append(rules, loadRuntimeBashPermissionSyntaxFlowRules()...)
	return rules
}
