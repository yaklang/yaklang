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

	// RegexRuleMatchExact / Prefix / Regex select how a row's pattern is
	// compared with each extracted command. Exact and prefix are literal
	// comparisons; regex keeps the legacy Go RE2 behavior.
	RegexRuleMatchExact  = "exact"
	RegexRuleMatchPrefix = "prefix"
	RegexRuleMatchRegex  = "regex"

	// regexRulesTTL bounds how stale a stored rule set may be during a long AI
	// session. Saving through SetKey also invalidates the cache in process.
	regexRulesTTL = 30 * time.Second
)

// RegexRule is one user-authored row from the AI Bash guard page. Patterns are
// evaluated against the command text (raw sub-commands included) because
// SyntaxFlow SSA conditions match value representations, not raw source, and
// therefore cannot express arbitrary command-line constraints.
//
// Match is optional for backward compatibility. An empty Match keeps the
// legacy behavior: a plain literal deny rule compares the whole command, while
// every other row is treated as a regex. New rows should set Match explicitly.
//
// Disabled is a pointer so ParseRegexRules can distinguish an omitted field
// from an explicit false. Stored rows default to disabled: a user must turn a
// row on before it participates in command checking.
type RegexRule struct {
	Name        string `json:"name,omitempty"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
	Match       string `json:"match,omitempty"`
	Description string `json:"description,omitempty"`
	Disabled    *bool  `json:"disabled,omitempty"`
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
	for i := range rules {
		if rules[i].Disabled == nil {
			rules[i].Disabled = boolPointer(true)
		}
	}
	return rules, nil
}

func boolPointer(value bool) *bool {
	return &value
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
	ruleID    string
	title     string
	message   string
	matchMode string
	literal   string
	pattern   *regexp.Regexp
	severity  schema.SyntaxFlowSeverity
	action    schema.SyntaxFlowAction
}

func compileRegexRules(rules []RegexRule) []compiledRegexRule {
	compiled := make([]compiledRegexRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Disabled != nil && *rule.Disabled {
			continue
		}
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		severity, ok := RegexRuleSeverity(rule.Action)
		if !ok {
			log.Warnf("ai bash guard rule %q has unknown action %q, treating it as deny", rule.Name, rule.Action)
		}
		action := schema.SyntaxFlowActionForSeverity(severity)
		matchMode, validMatch := normalizeRegexRuleMatch(rule.Match, action, pattern)
		if !validMatch {
			log.Warnf("ai bash guard rule %q has unknown match %q, falling back to %s", rule.Name, rule.Match, matchMode)
		}

		var re *regexp.Regexp
		if matchMode == RegexRuleMatchRegex {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				log.Warnf("ai bash guard skipped invalid regex rule %q: %v", rule.Name, err)
				continue
			}
		}

		title := strings.TrimSpace(rule.Name)
		if title == "" {
			title = pattern
		}
		message := strings.TrimSpace(rule.Description)
		if message == "" {
			message = defaultRegexRuleMessage(action)
		}
		compiled = append(compiled, compiledRegexRule{
			ruleID:    regexRuleID(pattern, string(action), matchMode),
			title:     title,
			message:   message,
			matchMode: matchMode,
			literal:   pattern,
			pattern:   re,
			severity:  severity,
			action:    action,
		})
	}
	return compiled
}

// normalizeRegexRuleMatch resolves the optional match field. When the caller
// omitted it, a plain literal deny row keeps the historical deny rule precise:
// `ls` matches the command `ls`, not every command containing the letters. Ask
// and warn rows stay regex-based because they are contextual prompts.
func normalizeRegexRuleMatch(raw string, action schema.SyntaxFlowAction, pattern string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case RegexRuleMatchExact, "command":
		return RegexRuleMatchExact, true
	case RegexRuleMatchPrefix, "command_prefix":
		return RegexRuleMatchPrefix, true
	case RegexRuleMatchRegex, "pattern":
		return RegexRuleMatchRegex, true
	case "":
		if action == schema.SFR_ACTION_DENY && isPlainCommandLiteral(pattern) {
			return RegexRuleMatchExact, true
		}
		return RegexRuleMatchRegex, true
	default:
		if action == schema.SFR_ACTION_DENY && isPlainCommandLiteral(pattern) {
			return RegexRuleMatchExact, false
		}
		return RegexRuleMatchRegex, false
	}
}

// isPlainCommandLiteral reports whether a pattern contains only command-name
// characters. Such a row is unambiguous enough to treat as an exact command;
// anything with spaces or regex metacharacters keeps the regex path.
func isPlainCommandLiteral(pattern string) bool {
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

// regexRuleID is stable across renames so findings and UI rows stay correlated.
func regexRuleID(pattern, action, matchMode string) string {
	sum := sha1.Sum([]byte(action + "\n" + matchMode + "\n" + pattern))
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
	appendFinding := func(rule compiledRegexRule, target regexMatchTarget, start, end int, matched string) {
		key := rule.ruleID + "|" + matched
		if _, ok := seen[key]; ok {
			return
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
			Start:    target.offset + start,
			End:      target.offset + end,
		})
	}

	for _, rule := range rules {
		for _, target := range targets {
			switch rule.matchMode {
			case RegexRuleMatchExact, RegexRuleMatchPrefix:
				text := strings.TrimSpace(target.text)
				if text == "" {
					continue
				}
				start := strings.Index(target.text, text)
				if start < 0 {
					start = 0
				}
				matched := false
				if rule.matchMode == RegexRuleMatchExact {
					matched = text == rule.literal
				} else {
					matched = matchCommandPrefix(rule.literal, text)
				}
				if matched {
					appendFinding(rule, target, start, start+len(text), text)
				}
			default:
				if rule.pattern == nil {
					continue
				}
				for _, loc := range rule.pattern.FindAllStringIndex(target.text, -1) {
					if loc[0] == loc[1] {
						continue
					}
					appendFinding(rule, target, loc[0], loc[1], target.text[loc[0]:loc[1]])
				}
			}
		}
	}
	return findings
}

// matchCommandPrefix matches the command itself or the command followed by an
// argument separator, so `ls` does not accidentally match `lsof`.
func matchCommandPrefix(prefix, command string) bool {
	prefix = strings.TrimSpace(prefix)
	command = strings.TrimSpace(command)
	if prefix == "" || command == "" {
		return false
	}
	if command == prefix {
		return true
	}
	for _, sep := range []string{" ", "\t", "\n", ":"} {
		if strings.HasPrefix(command, prefix+sep) {
			return true
		}
	}
	return false
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
