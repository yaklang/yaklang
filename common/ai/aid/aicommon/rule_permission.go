package aicommon

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// RulePermissionLists is the allow/ask/deny triple for 规则权限策略.
type RulePermissionLists struct {
	Allow []string `json:"allow,omitempty"`
	Ask   []string `json:"ask,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// RulePermissionRule is one compiled in-memory permission rule for non-bash
// tools. Bash entries are compiled into SyntaxFlow separately.
type RulePermissionRule struct {
	Action    schema.SyntaxFlowAction `json:"action"`
	Tool      string                  `json:"tool"`
	Specifier string                  `json:"specifier,omitempty"`
	Raw       string                  `json:"raw"`
	RuleID    string                  `json:"rule_id"`
}

// ParseRulePermissions parses one 规则权限策略 document.
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
			if rule != nil {
				rules = append(rules, rule)
			}
		}
	}
	return rules, nil
}

// SpliceRulePermissionConfigs concatenates several permission documents.
func SpliceRulePermissionConfigs(configs ...string) ([]*RulePermissionRule, error) {
	var rules []*RulePermissionRule
	for i, raw := range configs {
		parsed, err := ParseRulePermissions(raw)
		if err != nil {
			return nil, fmt.Errorf("permissions config %d: %w", i+1, err)
		}
		rules = append(rules, parsed...)
	}
	return rules, nil
}

// CompileRulePermissions builds an in-memory gate for non-bash tools.
// Bash entries are ignored here; they become SyntaxFlow rules instead.
func CompileRulePermissions(configs ...string) (ToolCallRuleGate, error) {
	rules, err := SpliceRulePermissionConfigs(configs...)
	if err != nil {
		return nil, err
	}
	return NewRulePermissionGate(rules), nil
}

// NewRulePermissionGate evaluates non-bash rules with deny > ask > allow.
func NewRulePermissionGate(rules []*RulePermissionRule) ToolCallRuleGate {
	snapshot := make([]RulePermissionRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || isBashPermissionTool(rule.Tool) {
			continue
		}
		copied := *rule
		if copied.RuleID == "" {
			copied.RuleID = rulePermissionID(copied.Action, copied.Raw)
		}
		snapshot = append(snapshot, copied)
	}
	if len(snapshot) == 0 {
		return nil
	}

	return func(_ context.Context, toolName string, params aitool.InvokeParams) (decision *ToolCallRuleDecision) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Warnf("rule permission gate panicked for tool[%v]: %v", toolName, recovered)
				decision = nil
			}
		}()

		for _, action := range []schema.SyntaxFlowAction{
			schema.SFR_ACTION_DENY,
			schema.SFR_ACTION_ASK,
			schema.SFR_ACTION_ALLOW,
		} {
			for i := range snapshot {
				rule := &snapshot[i]
				if rule.Action != action {
					continue
				}
				if matched, evidence := rule.Matches(toolName, params); matched {
					return rule.decision(evidence)
				}
			}
		}
		return nil
	}
}

// ComposeToolCallRuleGates merges gates with deny > ask > warn > allow.
func ComposeToolCallRuleGates(gates ...ToolCallRuleGate) ToolCallRuleGate {
	filtered := make([]ToolCallRuleGate, 0, len(gates))
	for _, gate := range gates {
		if gate != nil {
			filtered = append(filtered, gate)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}

	return func(ctx context.Context, toolName string, params aitool.InvokeParams) *ToolCallRuleDecision {
		var best *ToolCallRuleDecision
		for _, gate := range filtered {
			decision := callToolCallRuleGateSafely(ctx, gate, toolName, params)
			if decision == nil {
				continue
			}
			if best == nil || decision.Action.Rank() > best.Action.Rank() {
				best = decision
			}
		}
		return best
	}
}

func callToolCallRuleGateSafely(ctx context.Context, gate ToolCallRuleGate, toolName string, params aitool.InvokeParams) (decision *ToolCallRuleDecision) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("tool permission gate panicked for tool[%v]: %v", toolName, recovered)
			decision = nil
		}
	}()
	return gate(ctx, toolName, params)
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

func (r *RulePermissionRule) decision(evidence string) *ToolCallRuleDecision {
	reason := fmt.Sprintf("rule permission matched: %s", r.Raw)
	if evidence != "" {
		reason += " [" + evidence + "]"
	}
	return &ToolCallRuleDecision{
		Action:   r.Action,
		RuleName: r.Raw,
		RuleID:   r.RuleID,
		Reason:   reason,
	}
}

// Matches reports whether the rule applies to one non-bash tool call.
func (r *RulePermissionRule) Matches(toolName string, params aitool.InvokeParams) (bool, string) {
	if r == nil || isBashPermissionTool(r.Tool) {
		return false, ""
	}
	toolName = strings.TrimSpace(toolName)

	switch strings.ToLower(strings.TrimSpace(r.Tool)) {
	case "read":
		if !isReadToolName(toolName) && !isWriteToolName(toolName) {
			return false, ""
		}
		if strings.TrimSpace(r.Specifier) == "" {
			return true, toolName
		}
		return r.matchesPath(params)
	case "edit", "write", "multiedit", "notebookedit":
		if !isWriteToolName(toolName) {
			return false, ""
		}
		if strings.TrimSpace(r.Specifier) == "" {
			return true, toolName
		}
		return r.matchesPath(params)
	case "webfetch", "websearch":
		if strings.TrimSpace(r.Specifier) == "" {
			if values := collectURLValues(params); len(values) > 0 {
				return true, values[0]
			}
			return false, ""
		}
		return r.matchesDomain(params)
	default:
		if !matchToolName(r.Tool, toolName) {
			return false, ""
		}
		if strings.TrimSpace(r.Specifier) == "" {
			return true, toolName
		}
		return matchGenericSpecifier(r.Specifier, params)
	}
}

func (r *RulePermissionRule) matchesPath(params aitool.InvokeParams) (bool, string) {
	for _, candidate := range collectPathValues(params) {
		if matchPathPattern(r.Specifier, candidate) {
			return true, candidate
		}
	}
	return false, ""
}

func (r *RulePermissionRule) matchesDomain(params aitool.InvokeParams) (bool, string) {
	for _, candidate := range collectURLValues(params) {
		if matchDomainPattern(r.Specifier, candidate) {
			return true, candidate
		}
	}
	return false, ""
}

// CompileBashPermissionSyntaxFlowRules turns Bash(...) permission entries into
// temporary SyntaxFlow rules evaluated by the SSA bash checker.
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
// SyntaxFlow query. Specifiers match command name + required argument tokens
// (wildcards ignored), because SF regex runs against SSA values, not raw source.
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

func matchToolName(ruleTool, toolName string) bool {
	ruleTool = strings.TrimSpace(ruleTool)
	toolName = strings.TrimSpace(toolName)
	if ruleTool == "" || toolName == "" {
		return false
	}
	if strings.EqualFold(ruleTool, toolName) {
		return true
	}
	if strings.Contains(ruleTool, "*") {
		return globMatch(ruleTool, toolName, true)
	}
	return false
}

func matchGenericSpecifier(spec string, params aitool.InvokeParams) (bool, string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return true, ""
	}
	for _, candidate := range collectStringValues(params, 0) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == spec || globMatch(spec, candidate, false) {
			return true, candidate
		}
	}
	return false, ""
}

func globMatch(pattern, value string, caseInsensitive bool) bool {
	var b strings.Builder
	if caseInsensitive {
		b.WriteString("(?i)")
	}
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return pattern == value
	}
	return re.MatchString(value)
}

func isReadToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "read_file", "read_file_lines", "get_file_content", "get_file_content_any", "query_file_meta",
		"grep", "grep_files", "search_files":
		return true
	}
	return strings.HasPrefix(name, "read_")
}

func isWriteToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "write_file", "write_file_lines", "edit_file", "modify_code", "apply_code_patch", "create_file", "delete_file":
		return true
	}
	for _, prefix := range []string{"write_", "edit_", "modify_", "patch_", "create_", "delete_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

var pathParamKeys = []string{
	"file", "path", "filepath", "file_path", "filename",
	"target", "destination", "source", "src", "dst", "dir", "directory",
}

var urlParamKeys = []string{"url", "uri", "endpoint", "target", "host", "domain"}

func collectPathValues(params aitool.InvokeParams) []string {
	var values []string
	for _, key := range pathParamKeys {
		values = append(values, collectStringValues(params[key], 0)...)
	}
	for _, key := range []string{"files", "paths", "items"} {
		values = append(values, collectNestedPathValues(params[key], 0)...)
	}
	return dedupeStrings(values)
}

func collectURLValues(params aitool.InvokeParams) []string {
	var values []string
	for _, key := range urlParamKeys {
		values = append(values, collectStringValues(params[key], 0)...)
	}
	return dedupeStrings(values)
}

func collectNestedPathValues(value any, depth int) []string {
	if depth > 3 {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		var values []string
		for _, key := range pathParamKeys {
			values = append(values, collectStringValues(v[key], 0)...)
		}
		for _, child := range v {
			values = append(values, collectNestedPathValues(child, depth+1)...)
		}
		return values
	case aitool.InvokeParams:
		var values []string
		for _, key := range pathParamKeys {
			values = append(values, collectStringValues(v[key], 0)...)
		}
		for _, child := range v {
			values = append(values, collectNestedPathValues(child, depth+1)...)
		}
		return values
	case []any:
		var values []string
		for _, item := range v {
			values = append(values, collectNestedPathValues(item, depth+1)...)
		}
		return values
	case []string:
		return v
	case string:
		return []string{v}
	default:
		return nil
	}
}

func collectStringValues(value any, depth int) []string {
	if depth > 2 {
		return nil
	}
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		var values []string
		for _, item := range v {
			values = append(values, collectStringValues(item, depth+1)...)
		}
		return values
	case map[string]any:
		var values []string
		for _, child := range v {
			values = append(values, collectStringValues(child, depth+1)...)
		}
		return values
	case aitool.InvokeParams:
		var values []string
		for _, child := range v {
			values = append(values, collectStringValues(child, depth+1)...)
		}
		return values
	default:
		return nil
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func matchPathPattern(pattern, candidate string) bool {
	pattern = strings.TrimSpace(strings.Trim(pattern, `"'`))
	candidate = strings.TrimSpace(strings.Trim(candidate, `"'`))
	if pattern == "" || candidate == "" {
		return false
	}

	pattern = strings.ReplaceAll(pattern, "\\", "/")
	candidate = strings.ReplaceAll(candidate, "\\", "/")
	if strings.HasPrefix(pattern, "//") {
		pattern = strings.TrimPrefix(pattern, "/")
	}
	if strings.HasPrefix(pattern, "~") {
		pattern = strings.TrimPrefix(pattern, "~")
		pattern = strings.TrimPrefix(pattern, "/")
	} else {
		pattern = strings.TrimPrefix(pattern, "./")
	}
	if pattern == "" {
		return false
	}

	glob := pathGlobToRegex(pattern)
	flags := ""
	if runtime.GOOS == "windows" {
		flags = "(?i)"
	}
	if ok, _ := regexp.MatchString(flags+"^"+glob+"$", candidate); ok {
		return true
	}
	if !strings.HasPrefix(pattern, "/") {
		if ok, _ := regexp.MatchString(flags+"^(?:.*/)?"+glob+"$", candidate); ok {
			return true
		}
		return false
	}
	if ok, _ := regexp.MatchString(flags+"^(?:.*/)?"+strings.TrimPrefix(glob, "/")+"$", candidate); ok {
		return true
	}
	return false
}

func pathGlobToRegex(pattern string) string {
	var b strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}
	return b.String()
}

func matchDomainPattern(spec, raw string) bool {
	spec = strings.TrimSpace(spec)
	spec = strings.TrimSpace(strings.TrimPrefix(spec, "domain:"))
	host := extractURLHost(raw)
	if spec == "" || host == "" {
		return false
	}

	spec = strings.ToLower(strings.TrimSuffix(spec, "."))
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if strings.HasPrefix(spec, "*.") {
		suffix := strings.TrimPrefix(spec, "*")
		return strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".")
	}
	if strings.HasPrefix(spec, ".") {
		base := strings.TrimPrefix(spec, ".")
		return host == base || strings.HasSuffix(host, spec)
	}
	if strings.Contains(spec, "*") {
		return globMatch(spec, host, true)
	}
	return host == spec
}

func extractURLHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if parsed, err := url.Parse("//" + strings.TrimPrefix(raw, "//")); err == nil {
		return parsed.Hostname()
	}
	return ""
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
