package aicommon

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// BashGuardRegexRulesStorageKey is the profile-DB key shared with the GUI
	// settings page. Rows are compiled into temporary SyntaxFlow rules at check
	// time rather than matched with a separate regex engine.
	BashGuardRegexRulesStorageKey = "ai-bash-guard-regex-rules"

	bashCustomRegexRuleActionDeny = string(schema.SFR_ACTION_DENY)
	bashCustomRegexRuleActionAsk  = string(schema.SFR_ACTION_ASK)
	bashCustomRegexRuleActionWarn = string(schema.SFR_ACTION_WARN)

	bashCustomRegexMatchExact  = "exact"
	bashCustomRegexMatchPrefix = "prefix"
	bashCustomRegexMatchRegex  = "regex"

	bashCustomRegexRulesTTL = 30 * time.Second
)

// BashGuardRegexRule is one user-authored row from the AI Bash guard page.
// Patterns are compiled into SyntaxFlow regex queries so they share the same
// SSA evaluation path as built-in rules.
type BashGuardRegexRule struct {
	Name        string `json:"name,omitempty"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
	Match       string `json:"match,omitempty"`
	Description string `json:"description,omitempty"`
	Disabled    *bool  `json:"disabled,omitempty"`
}

// ParseBashGuardRegexRules decodes the stored GUI payload.
func ParseBashGuardRegexRules(raw string) ([]BashGuardRegexRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var rules []BashGuardRegexRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("parse ai bash guard regex rules: %w", err)
	}
	for i := range rules {
		if rules[i].Disabled == nil {
			disabled := true
			rules[i].Disabled = &disabled
		}
	}
	return rules, nil
}

func bashGuardRegexRuleSeverity(action string) (schema.SyntaxFlowSeverity, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", bashCustomRegexRuleActionDeny, "block":
		return schema.SFR_SEVERITY_HIGH, true
	case bashCustomRegexRuleActionAsk, "confirm":
		return schema.SFR_SEVERITY_WARNING, true
	case bashCustomRegexRuleActionWarn, "warning":
		return schema.SFR_SEVERITY_LOW, true
	default:
		return schema.SFR_SEVERITY_HIGH, false
	}
}

func normalizeBashGuardRegexMatch(raw string, action schema.SyntaxFlowAction, pattern string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case bashCustomRegexMatchExact, "command":
		return bashCustomRegexMatchExact, true
	case bashCustomRegexMatchPrefix, "command_prefix":
		return bashCustomRegexMatchPrefix, true
	case bashCustomRegexMatchRegex, "pattern":
		return bashCustomRegexMatchRegex, true
	case "":
		return inferBashGuardRegexMatch(action, pattern), true
	default:
		return inferBashGuardRegexMatch(action, pattern), false
	}
}

func inferBashGuardRegexMatch(action schema.SyntaxFlowAction, pattern string) string {
	if action == schema.SFR_ACTION_DENY && isPlainBashCommandLiteral(pattern) {
		return bashCustomRegexMatchExact
	}
	return bashCustomRegexMatchRegex
}

func isPlainBashCommandLiteral(pattern string) bool {
	if pattern == "" {
		return false
	}
	for _, r := range pattern {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("_-./:@%+=,", r):
		default:
			return false
		}
	}
	return true
}

func bashGuardRegexRuleID(pattern, action, matchMode string) string {
	sum := sha1.Sum([]byte(action + "\n" + matchMode + "\n" + pattern))
	return fmt.Sprintf("ai-bash-regex-%x", sum[:6])
}

func defaultBashGuardRegexMessage(action schema.SyntaxFlowAction) string {
	switch action {
	case schema.SFR_ACTION_DENY:
		return "命中自定义 Bash 拦截规则，命令已被阻止执行"
	case schema.SFR_ACTION_ASK:
		return "命中自定义 Bash 询问规则，执行前需要用户确认"
	case schema.SFR_ACTION_WARN:
		return "命中自定义 Bash 警告规则"
	default:
		return "命中自定义 Bash 安全规则"
	}
}

// bashGuardRegexToSyntaxFlowPattern turns a GUI row into a SyntaxFlow regex body.
func bashGuardRegexToSyntaxFlowPattern(matchMode, pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", fmt.Errorf("empty pattern")
	}
	switch matchMode {
	case bashCustomRegexMatchExact:
		return "^" + regexp.QuoteMeta(pattern) + "$", nil
	case bashCustomRegexMatchPrefix:
		return "^" + regexp.QuoteMeta(pattern) + `(\s|$)`, nil
	default:
		if _, err := regexp.Compile(pattern); err != nil {
			return "", err
		}
		return pattern, nil
	}
}

func escapeSyntaxFlowRegexBody(pattern string) string {
	return strings.ReplaceAll(pattern, `/`, `\/`)
}

// CompileBashGuardRegexRuleToSyntaxFlow builds one temporary SyntaxFlow rule
// from a GUI regex row.
func CompileBashGuardRegexRuleToSyntaxFlow(rule BashGuardRegexRule) (*schema.SyntaxFlowRule, error) {
	if rule.Disabled != nil && *rule.Disabled {
		return nil, nil
	}
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" {
		return nil, nil
	}
	severity, ok := bashGuardRegexRuleSeverity(rule.Action)
	if !ok {
		log.Warnf("ai bash guard rule %q has unknown action %q, treating it as deny", rule.Name, rule.Action)
	}
	action := schema.SyntaxFlowActionForSeverity(severity)
	matchMode, validMatch := normalizeBashGuardRegexMatch(rule.Match, action, pattern)
	if !validMatch {
		log.Warnf("ai bash guard rule %q has unknown match %q, falling back to %s", rule.Name, rule.Match, matchMode)
	}
	sfPattern, err := bashGuardRegexToSyntaxFlowPattern(matchMode, pattern)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(rule.Name)
	if title == "" {
		title = pattern
	}
	message := strings.TrimSpace(rule.Description)
	if message == "" {
		message = defaultBashGuardRegexMessage(action)
	}
	ruleID := bashGuardRegexRuleID(pattern, string(action), matchMode)
	level := string(severity)
	content := fmt.Sprintf(`desc(
	mode: "ssa"
	language: "bash"
	title: %s
	level: %s
	rule_id: %s
)
/%s/ as $hit
alert $hit for {
	level: %s,
	title: %s,
	message: %s,
	risk: "bash-guard",
}
`,
		strconv.Quote(title),
		strconv.Quote(level),
		strconv.Quote(ruleID),
		escapeSyntaxFlowRegexBody(sfPattern),
		strconv.Quote(level),
		strconv.Quote(title),
		strconv.Quote(message),
	)

	return &schema.SyntaxFlowRule{
		RuleName:    title,
		RuleId:      ruleID,
		Title:       title,
		Description: message,
		Language:    ssaconfig.BASH,
		Severity:    severity,
		Type:        schema.SFR_RULE_TYPE_SF,
		Mode:        schema.SFR_MODE_SSA,
		RiskType:    "bash-guard",
		Content:     content,
	}, nil
}

func compileBashGuardRegexRulesToSyntaxFlow(rules []BashGuardRegexRule) []*schema.SyntaxFlowRule {
	compiled := make([]*schema.SyntaxFlowRule, 0, len(rules))
	for _, rule := range rules {
		sfRule, err := CompileBashGuardRegexRuleToSyntaxFlow(rule)
		if err != nil {
			log.Warnf("ai bash guard skipped invalid regex rule %q: %v", rule.Name, err)
			continue
		}
		if sfRule == nil {
			continue
		}
		compiled = append(compiled, sfRule)
	}
	return compiled
}

var bashCustomRegexRulesCache struct {
	mu       sync.Mutex
	raw      string
	rules    []*schema.SyntaxFlowRule
	loaded   bool
	loadedAt time.Time
}

func loadCompiledCustomRegexSyntaxFlowRules() []*schema.SyntaxFlowRule {
	bashCustomRegexRulesCache.mu.Lock()
	defer bashCustomRegexRulesCache.mu.Unlock()

	now := time.Now()
	if bashCustomRegexRulesCache.loaded && now.Sub(bashCustomRegexRulesCache.loadedAt) < bashCustomRegexRulesTTL {
		return bashCustomRegexRulesCache.rules
	}
	raw := yakit.GetKey(consts.GetGormProfileDatabase(), BashGuardRegexRulesStorageKey)
	if bashCustomRegexRulesCache.loaded && raw == bashCustomRegexRulesCache.raw {
		bashCustomRegexRulesCache.loadedAt = now
		return bashCustomRegexRulesCache.rules
	}
	rules, err := ParseBashGuardRegexRules(raw)
	if err != nil {
		log.Warnf("load ai bash guard regex rules failed: %v", err)
		bashCustomRegexRulesCache.raw = raw
		bashCustomRegexRulesCache.loadedAt = now
		return bashCustomRegexRulesCache.rules
	}
	bashCustomRegexRulesCache.raw = raw
	bashCustomRegexRulesCache.rules = compileBashGuardRegexRulesToSyntaxFlow(rules)
	bashCustomRegexRulesCache.loaded = true
	bashCustomRegexRulesCache.loadedAt = now
	return bashCustomRegexRulesCache.rules
}

// InvalidateBashGuardRegexRulesCache drops the cached GUI rows so a saved rule
// set takes effect on the next command check.
func InvalidateBashGuardRegexRulesCache() {
	bashCustomRegexRulesCache.mu.Lock()
	defer bashCustomRegexRulesCache.mu.Unlock()
	bashCustomRegexRulesCache.loaded = false
	bashCustomRegexRulesCache.raw = ""
	bashCustomRegexRulesCache.rules = nil
}
