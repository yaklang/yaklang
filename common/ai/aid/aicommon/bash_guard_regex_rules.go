package aicommon

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/syntax"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// BashGuardRegexRulesStorageKey is the profile-DB key shared with the GUI
	// settings page. Rows are matched against the command text directly.
	BashGuardRegexRulesStorageKey = "ai-bash-guard-regex-rules"

	bashGuardMatchExact  = "exact"
	bashGuardMatchPrefix = "prefix"
	bashGuardMatchRegex  = "regex"
)

// BashGuardRegexRule is one user-authored row from the AI Bash guard page.
type BashGuardRegexRule struct {
	Name        string `json:"name,omitempty"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
	Match       string `json:"match,omitempty"`
	Description string `json:"description,omitempty"`
	Disabled    *bool  `json:"disabled,omitempty"`
}

// ParseBashGuardRegexRules decodes the stored GUI payload. Rows without an
// explicit disabled flag stay off, matching the frontend contract.
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

func bashGuardRegexAction(raw string) schema.SyntaxFlowAction {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ask":
		return schema.SFR_ACTION_ASK
	case "warn":
		return schema.SFR_ACTION_WARN
	default:
		return schema.SFR_ACTION_DENY
	}
}

func bashGuardRegexMatchMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case bashGuardMatchExact:
		return bashGuardMatchExact
	case bashGuardMatchPrefix:
		return bashGuardMatchPrefix
	default:
		return bashGuardMatchRegex
	}
}

type compiledBashGuardRegexRule struct {
	action    schema.SyntaxFlowAction
	matchMode string
	pattern   string
	re        *regexp.Regexp
}

func compileBashGuardRegexRules(rules []BashGuardRegexRule) []compiledBashGuardRegexRule {
	compiled := make([]compiledBashGuardRegexRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Disabled != nil && *rule.Disabled {
			continue
		}
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		entry := compiledBashGuardRegexRule{
			action:    bashGuardRegexAction(rule.Action),
			matchMode: bashGuardRegexMatchMode(rule.Match),
			pattern:   pattern,
		}
		if entry.matchMode == bashGuardMatchRegex {
			re, err := regexp.Compile(pattern)
			if err != nil {
				log.Warnf("ai bash guard skipped invalid regex rule %q: %v", rule.Name, err)
				continue
			}
			entry.re = re
		}
		compiled = append(compiled, entry)
	}
	return compiled
}

// bashCommandTexts returns the raw command plus each parsed shell call. Custom
// rules are written for command lines, while bash SSA values are individual
// words, so matching must not rely on SSA values.
func bashCommandTexts(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	texts := []string{command}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return texts
	}
	printer := syntax.NewPrinter()
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		var buf strings.Builder
		if err := printer.Print(&buf, call); err == nil {
			if text := strings.TrimSpace(buf.String()); text != "" {
				texts = append(texts, text)
			}
		}
		return true
	})
	return texts
}

func bashGuardPrefixMatch(text, pattern string) bool {
	if !strings.HasPrefix(text, pattern) {
		return false
	}
	if len(text) == len(pattern) {
		return true
	}
	switch text[len(pattern)] {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func (r compiledBashGuardRegexRule) matches(text string) bool {
	switch r.matchMode {
	case bashGuardMatchExact:
		return text == r.pattern
	case bashGuardMatchPrefix:
		return bashGuardPrefixMatch(text, r.pattern)
	default:
		return r.re != nil && r.re.MatchString(text)
	}
}

func checkBashGuardRegexRules(command string) schema.SyntaxFlowAction {
	rules := loadBashGuardRegexRules()
	if len(rules) == 0 {
		return schema.SFR_ACTION_ALLOW
	}
	texts := bashCommandTexts(command)
	best := schema.SFR_ACTION_ALLOW
	for _, rule := range rules {
		if rule.action.Rank() <= best.Rank() {
			continue
		}
		for _, text := range texts {
			if rule.matches(text) {
				best = rule.action
				break
			}
		}
		if best == schema.SFR_ACTION_DENY {
			break
		}
	}
	return best
}

var bashGuardRegexRulesCache struct {
	mu     sync.Mutex
	raw    string
	rules  []compiledBashGuardRegexRule
	loaded bool
}

func loadBashGuardRegexRules() []compiledBashGuardRegexRule {
	bashGuardRegexRulesCache.mu.Lock()
	defer bashGuardRegexRulesCache.mu.Unlock()

	raw := yakit.GetKey(consts.GetGormProfileDatabase(), BashGuardRegexRulesStorageKey)
	if bashGuardRegexRulesCache.loaded && raw == bashGuardRegexRulesCache.raw {
		return bashGuardRegexRulesCache.rules
	}
	rules, err := ParseBashGuardRegexRules(raw)
	if err != nil {
		log.Warnf("load ai bash guard regex rules failed: %v", err)
		rules = nil
	}
	bashGuardRegexRulesCache.raw = raw
	bashGuardRegexRulesCache.rules = compileBashGuardRegexRules(rules)
	bashGuardRegexRulesCache.loaded = true
	return bashGuardRegexRulesCache.rules
}

// InvalidateBashGuardRegexRulesCache drops the cached GUI rows so a saved rule
// set takes effect on the next command check.
func InvalidateBashGuardRegexRulesCache() {
	bashGuardRegexRulesCache.mu.Lock()
	defer bashGuardRegexRulesCache.mu.Unlock()
	bashGuardRegexRulesCache.loaded = false
	bashGuardRegexRulesCache.raw = ""
	bashGuardRegexRulesCache.rules = nil
}
