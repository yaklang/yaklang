package commandcheck

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/bash/bash2ssa"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// RegexRulesStorageKey is the profile-DB key shared with the GUI settings
	// page. The value is a JSON array of RegexRule rows.
	RegexRulesStorageKey = "ai-bash-guard-regex-rules"

	// RegexRuleActionDeny / Ask / Warn mirror schema.SyntaxFlowAction so regex
	// hits and SyntaxFlow hits share one severity -> action contract.
	RegexRuleActionDeny = string(schema.SFR_ACTION_DENY)
	RegexRuleActionAsk  = string(schema.SFR_ACTION_ASK)
	RegexRuleActionWarn = string(schema.SFR_ACTION_WARN)

	// regexRulesTTL bounds how stale a stored rule set may be during a long AI
	// session. Saving through SetKey also invalidates the cache in process.
	regexRulesTTL = 30 * time.Second
)

// RegexRule is one user-authored row from the AI Bash guard page. Patterns are
// evaluated against the command text (raw sub-commands included) because
// SyntaxFlow SSA conditions match value representations, not raw source, and
// therefore cannot express arbitrary command-line regular expressions.
type RegexRule struct {
	Name        string `json:"name,omitempty"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// ParseRegexRules decodes the stored payload. An empty payload means the
// feature is unused rather than a broken configuration.
func ParseRegexRules(raw string) ([]RegexRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var rules []RegexRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("parse ai bash guard regex rules: %w", err)
	}
	return rules, nil
}

// RegexRuleSeverity maps a row action to the severity that drives both the
// finding level and the enforcement action. Unknown actions fail closed to
// deny: a typo in a security rule must not silently weaken it.
func RegexRuleSeverity(action string) (schema.SyntaxFlowSeverity, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", RegexRuleActionDeny, "block":
		return schema.SFR_SEVERITY_HIGH, true
	case RegexRuleActionAsk, "confirm":
		return schema.SFR_SEVERITY_WARNING, true
	case RegexRuleActionWarn, "warning":
		return schema.SFR_SEVERITY_LOW, true
	default:
		return schema.SFR_SEVERITY_HIGH, false
	}
}

// compiledRegexRule is one row ready to run against command text.
type compiledRegexRule struct {
	ruleID   string
	title    string
	message  string
	pattern  *regexp.Regexp
	severity schema.SyntaxFlowSeverity
	action   schema.SyntaxFlowAction
}

func compileRegexRules(rules []RegexRule) []compiledRegexRule {
	compiled := make([]compiledRegexRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Disabled {
			continue
		}
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Warnf("ai bash guard skipped invalid regex rule %q: %v", rule.Name, err)
			continue
		}
		severity, ok := RegexRuleSeverity(rule.Action)
		if !ok {
			log.Warnf("ai bash guard regex rule %q has unknown action %q, treating it as deny", rule.Name, rule.Action)
		}
		title := strings.TrimSpace(rule.Name)
		if title == "" {
			title = pattern
		}
		message := strings.TrimSpace(rule.Description)
		if message == "" {
			message = defaultRegexRuleMessage(schema.SyntaxFlowActionForSeverity(severity))
		}
		compiled = append(compiled, compiledRegexRule{
			ruleID:   regexRuleID(pattern, string(schema.SyntaxFlowActionForSeverity(severity))),
			title:    title,
			message:  message,
			pattern:  re,
			severity: severity,
			action:   schema.SyntaxFlowActionForSeverity(severity),
		})
	}
	return compiled
}

// regexRuleID is stable across renames so findings and UI rows stay correlated.
func regexRuleID(pattern, action string) string {
	sum := sha1.Sum([]byte(action + "\n" + pattern))
	return fmt.Sprintf("ai-bash-regex-%x", sum[:6])
}

func defaultRegexRuleMessage(action schema.SyntaxFlowAction) string {
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

type regexMatchTarget struct {
	text   string
	offset int
}

// regexMatchTargets returns the full command plus every statically extractable
// sub-command. Matching sub-commands keeps patterns like `curl\s+http://x`
// working inside pipelines and `&&` chains without forcing users to write
// whole-command patterns.
func regexMatchTargets(cmd string) []regexMatchTarget {
	targets := []regexMatchTarget{{text: cmd}}
	commands, err := bash2ssa.ExtractShellCommands(cmd)
	if err != nil {
		return targets
	}
	seen := map[string]struct{}{cmd: {}}
	for _, command := range commands {
		text := command.Text
		if strings.TrimSpace(text) == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		offset := strings.Index(cmd, text)
		if offset < 0 {
			offset = 0
		}
		targets = append(targets, regexMatchTarget{text: text, offset: offset})
	}
	return targets
}

func matchRegexRules(cmd string, rules []compiledRegexRule) []Finding {
	if len(rules) == 0 || strings.TrimSpace(cmd) == "" {
		return nil
	}
	targets := regexMatchTargets(cmd)
	var findings []Finding
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		for _, target := range targets {
			for _, loc := range rule.pattern.FindAllStringIndex(target.text, -1) {
				if loc[0] == loc[1] {
					continue
				}
				matched := target.text[loc[0]:loc[1]]
				key := rule.ruleID + "|" + matched
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				findings = append(findings, Finding{
					RuleID:   rule.ruleID,
					Level:    string(rule.severity),
					Severity: rule.severity,
					Action:   rule.action,
					Title:    rule.title,
					Message:  rule.message,
					Risk:     "bash-guard",
					Value:    truncateValue(matched, 512),
					Start:    target.offset + loc[0],
					End:      target.offset + loc[1],
				})
			}
		}
	}
	return findings
}

var regexRulesCache struct {
	mu       sync.Mutex
	raw      string
	rules    []compiledRegexRule
	loaded   bool
	loadedAt time.Time
}

// loadCompiledRegexRules reads the stored rows through the profile DB. Broken
// payloads keep the previous rule set so a bad save cannot silently remove the
// user's protection mid-session.
func loadCompiledRegexRules() []compiledRegexRule {
	regexRulesCache.mu.Lock()
	defer regexRulesCache.mu.Unlock()

	now := time.Now()
	if regexRulesCache.loaded && now.Sub(regexRulesCache.loadedAt) < regexRulesTTL {
		return regexRulesCache.rules
	}
	raw := yakit.GetKey(consts.GetGormProfileDatabase(), RegexRulesStorageKey)
	if regexRulesCache.loaded && raw == regexRulesCache.raw {
		regexRulesCache.loadedAt = now
		return regexRulesCache.rules
	}
	rules, err := ParseRegexRules(raw)
	if err != nil {
		log.Warnf("load ai bash guard regex rules failed: %v", err)
		return regexRulesCache.rules
	}
	regexRulesCache.raw = raw
	regexRulesCache.rules = compileRegexRules(rules)
	regexRulesCache.loaded = true
	regexRulesCache.loadedAt = now
	return regexRulesCache.rules
}

// InvalidateRegexRulesCache drops the cached rows so a saved rule set takes
// effect on the next command check instead of after regexRulesTTL.
func InvalidateRegexRulesCache() {
	regexRulesCache.mu.Lock()
	defer regexRulesCache.mu.Unlock()
	regexRulesCache.loaded = false
	regexRulesCache.raw = ""
	regexRulesCache.rules = nil
}
