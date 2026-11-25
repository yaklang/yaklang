package aicommon

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// RulePermissionLists is the allow/ask/deny triple for shell 规则权限策略.
// Only Bash(...) entries are used; other tool names are ignored.
type RulePermissionLists struct {
	Allow []string `json:"allow,omitempty"`
	Ask   []string `json:"ask,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// RulePermissionRule is one parsed Bash(...) permission entry.
type RulePermissionRule struct {
	Action    schema.SyntaxFlowAction `json:"action"`
	Tool      string                  `json:"tool"`
	Specifier string                  `json:"specifier,omitempty"`
	Raw       string                  `json:"raw"`
	RuleID    string                  `json:"rule_id"`
}

// ParseRulePermissions parses one 规则权限策略 document and keeps Bash entries only.
func ParseRulePermissions(raw string) ([]*RulePermissionRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var envelope struct {
		Permissions *RulePermissionLists `json:"permissions"`
		Allow       []string             `json:"allow"`
		Ask         []string             `json:"ask"`
		Deny        []string             `json:"deny"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("invalid rule permission JSON: %w", err)
	}

	lists := RulePermissionLists{
		Allow: envelope.Allow,
		Ask:   envelope.Ask,
		Deny:  envelope.Deny,
	}
	if envelope.Permissions != nil {
		lists = *envelope.Permissions
	}

	groups := []struct {
		action schema.SyntaxFlowAction
		values []string
	}{
		{schema.SFR_ACTION_DENY, lists.Deny},
		{schema.SFR_ACTION_ASK, lists.Ask},
		{schema.SFR_ACTION_ALLOW, lists.Allow},
	}

	var rules []*RulePermissionRule
	for _, group := range groups {
		for _, value := range group.values {
			rule, err := parseRulePermissionEntry(value, group.action)
			if err != nil {
				return nil, err
			}
			if rule == nil || !isBashPermissionTool(rule.Tool) {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func parseRulePermissionEntry(raw string, action schema.SyntaxFlowAction) (*RulePermissionRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	toolName := raw
	specifier := ""
	if open := strings.Index(raw, "("); open >= 0 {
		if !strings.HasSuffix(raw, ")") {
			return nil, fmt.Errorf("invalid permission rule %q: missing closing ')'", raw)
		}
		toolName = strings.TrimSpace(raw[:open])
		specifier = strings.TrimSpace(raw[open+1 : len(raw)-1])
	} else if strings.Contains(raw, ")") {
		return nil, fmt.Errorf("invalid permission rule %q: ')' without '('", raw)
	}
	if toolName == "" {
		return nil, fmt.Errorf("invalid permission rule %q: empty tool name", raw)
	}
	if strings.ContainsAny(toolName, " \t\r\n") {
		return nil, fmt.Errorf("invalid permission rule %q: tool name must not contain whitespace", raw)
	}

	return &RulePermissionRule{
		Action:    action,
		Tool:      toolName,
		Specifier: specifier,
		Raw:       raw,
		RuleID:    rulePermissionID(action, raw),
	}, nil
}

func rulePermissionID(action schema.SyntaxFlowAction, raw string) string {
	sum := sha1.Sum([]byte(string(action) + "\x00" + raw))
	return "rule-permission-" + hex.EncodeToString(sum[:6])
}

func isBashPermissionTool(tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "bash", "shell", "sh":
		return true
	default:
		return false
	}
}

// CompileBashPermissionSyntaxFlowRules turns Bash(...) entries into temporary
// SyntaxFlow rules for the SSA bash checker.
func CompileBashPermissionSyntaxFlowRules(rules []*RulePermissionRule) []*schema.SyntaxFlowRule {
	out := make([]*schema.SyntaxFlowRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !isBashPermissionTool(rule.Tool) {
			continue
		}
		// Allow entries do not alert; absence of deny/ask/warn is allow.
		if rule.Action == schema.SFR_ACTION_ALLOW {
			continue
		}
		sfRule, err := bashPermissionEntryToSyntaxFlow(rule)
		if err != nil {
			log.Warnf("skip bash permission rule %q: %v", rule.Raw, err)
			continue
		}
		if sfRule != nil {
			out = append(out, sfRule)
		}
	}
	return out
}

func bashPermissionEntryToSyntaxFlow(rule *RulePermissionRule) (*schema.SyntaxFlowRule, error) {
	query, err := bashPermissionSpecifierToSyntaxFlowQuery(rule.Specifier)
	if err != nil {
		return nil, err
	}
	severity := severityForPermissionAction(rule.Action)
	level := string(severity)
	title := rule.Raw
	ruleID := rule.RuleID
	if ruleID == "" {
		ruleID = rulePermissionID(rule.Action, rule.Raw)
	}
	content := fmt.Sprintf(`desc(
	mode: "ssa"
	language: "bash"
	title: %s
	level: %s
	rule_id: %s
)
%s
alert $hit for {
	level: %s,
	title: %s,
	message: %s,
}
`,
		strconv.Quote(title),
		strconv.Quote(level),
		strconv.Quote(ruleID),
		query,
		strconv.Quote(level),
		strconv.Quote(title),
		strconv.Quote("命中规则权限策略"),
	)
	return &schema.SyntaxFlowRule{
		RuleName: title,
		RuleId:   ruleID,
		Title:    title,
		Language: ssaconfig.BASH,
		Severity: severity,
		Type:     schema.SFR_RULE_TYPE_SF,
		Mode:     schema.SFR_MODE_SSA,
		Content:  content,
	}, nil
}

// bashPermissionSpecifierToSyntaxFlowQuery turns Bash(spec) into an SSA-oriented
// SyntaxFlow query: command name + required argument tokens.
func bashPermissionSpecifierToSyntaxFlowQuery(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "*" {
		return `*?{opcode: call} as $hit`, nil
	}
	if strings.HasSuffix(spec, ":*") {
		spec = strings.TrimSpace(strings.TrimSuffix(spec, ":*"))
	}
	parts := strings.Fields(spec)
	if len(parts) == 0 {
		return `*?{opcode: call} as $hit`, nil
	}

	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "*" || part == "?" {
			continue
		}
		tokens = append(tokens, part)
	}
	if len(tokens) == 0 {
		return `*?{opcode: call} as $hit`, nil
	}

	cmd := tokens[0]
	var b strings.Builder
	fmt.Fprintf(&b, "/%s/ as $cmd\n", escapeSyntaxFlowRegexBody("^"+regexp.QuoteMeta(cmd)+"$"))
	b.WriteString("$cmd(* as $argv) as $call\n")
	current := "$call"
	for i, token := range tokens[1:] {
		name := "$hit"
		if i+1 < len(tokens[1:]) {
			name = fmt.Sprintf("$arg%d", i)
		}
		fmt.Fprintf(&b, "%s?{have: %s} as %s\n", current, strconv.Quote(token), name)
		current = name
	}
	if current == "$call" {
		b.WriteString("$call as $hit\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func severityForPermissionAction(action schema.SyntaxFlowAction) schema.SyntaxFlowSeverity {
	switch action {
	case schema.SFR_ACTION_DENY:
		return schema.SFR_SEVERITY_HIGH
	case schema.SFR_ACTION_ASK:
		return schema.SFR_SEVERITY_WARNING
	case schema.SFR_ACTION_WARN:
		return schema.SFR_SEVERITY_LOW
	default:
		return schema.SFR_SEVERITY_INFO
	}
}
