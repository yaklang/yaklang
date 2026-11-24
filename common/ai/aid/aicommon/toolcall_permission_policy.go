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
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/bash/bash2ssa"
)

// Claude Code permission rules are a good common shape for user-authored tool
// policy: allow/ask/deny lists whose entries are Tool(specifier). This file
// compiles those entries into in-memory temporary rules. Nothing is persisted
// to the syntax-flow database; the compiled gate lives with the AI config.

const maxTemporaryPolicyMatches = 8

// ClaudeCodePermissionConfig mirrors the permissions block in Claude Code
// settings.json. Unknown siblings such as defaultMode are intentionally ignored.
type ClaudeCodePermissionConfig struct {
	Permissions ClaudeCodePermissionLists `json:"permissions"`
}

// ClaudeCodePermissionLists is the allow/ask/deny triple. The same shape is
// also accepted at the top level to make single-purpose configs convenient.
type ClaudeCodePermissionLists struct {
	Allow []string `json:"allow,omitempty"`
	Ask   []string `json:"ask,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ToolCallPolicyRule is one compiled, in-memory permission rule. Raw preserves
// the source rule for review cards and logs; RuleID is stable across parses.
type ToolCallPolicyRule struct {
	Action    schema.SyntaxFlowAction   `json:"action"`
	Severity  schema.SyntaxFlowSeverity `json:"severity"`
	Tool      string                    `json:"tool"`
	Specifier string                    `json:"specifier,omitempty"`
	Raw       string                    `json:"raw"`
	RuleID    string                    `json:"rule_id"`
}

// ParseClaudeCodePermissions parses one Claude Code permissions document. A
// missing permissions envelope is accepted only when allow/ask/deny are at the
// top level, which keeps tests and small embedded configs readable.
func ParseClaudeCodePermissions(raw string) ([]*ToolCallPolicyRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var envelope struct {
		Permissions *ClaudeCodePermissionLists `json:"permissions"`
		Allow       []string                   `json:"allow"`
		Ask         []string                   `json:"ask"`
		Deny        []string                   `json:"deny"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, fmt.Errorf("invalid Claude Code permissions JSON: %w", err)
	}

	lists := ClaudeCodePermissionLists{
		Allow: envelope.Allow,
		Ask:   envelope.Ask,
		Deny:  envelope.Deny,
	}
	if envelope.Permissions != nil {
		lists = *envelope.Permissions
	}

	groups := []struct {
		action   schema.SyntaxFlowAction
		severity schema.SyntaxFlowSeverity
		values   []string
	}{
		{schema.SFR_ACTION_DENY, schema.SFR_SEVERITY_HIGH, lists.Deny},
		{schema.SFR_ACTION_ASK, schema.SFR_SEVERITY_WARNING, lists.Ask},
		{schema.SFR_ACTION_ALLOW, schema.SFR_SEVERITY_LOW, lists.Allow},
	}

	var rules []*ToolCallPolicyRule
	for _, group := range groups {
		for _, value := range group.values {
			rule, err := parseClaudePermissionRule(value, group.action, group.severity)
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

// SpliceClaudeCodePermissionConfigs concatenates several permissions documents
// into one temporary rule set. Deny still wins at evaluation time regardless of
// which document declared it.
func SpliceClaudeCodePermissionConfigs(configs ...string) ([]*ToolCallPolicyRule, error) {
	var rules []*ToolCallPolicyRule
	for i, raw := range configs {
		parsed, err := ParseClaudeCodePermissions(raw)
		if err != nil {
			return nil, fmt.Errorf("permissions config %d: %w", i+1, err)
		}
		rules = append(rules, parsed...)
	}
	return rules, nil
}

// CompileClaudeCodePermissions builds an in-memory gate from one or more
// permissions documents. Empty input returns (nil, nil), which callers can
// safely pass to ComposeToolCallRuleGates.
func CompileClaudeCodePermissions(configs ...string) (ToolCallRuleGate, error) {
	rules, err := SpliceClaudeCodePermissionConfigs(configs...)
	if err != nil {
		return nil, err
	}
	return NewToolCallPolicyGate(rules), nil
}

// NewToolCallPolicyGate evaluates rules with Claude Code precedence:
// deny, then ask, then allow. Within one action the first declared match wins.
func NewToolCallPolicyGate(rules []*ToolCallPolicyRule) ToolCallRuleGate {
	snapshot := make([]ToolCallPolicyRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		copied := *rule
		if copied.RuleID == "" {
			copied.RuleID = claudePermissionRuleID(copied.Action, copied.Raw)
		}
		snapshot = append(snapshot, copied)
	}
	if len(snapshot) == 0 {
		return nil
	}

	return func(_ context.Context, toolName string, params aitool.InvokeParams) (decision *ToolCallRuleDecision) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Warnf("temporary tool permission gate panicked for tool[%v]: %v", toolName, recovered)
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

// ComposeToolCallRuleGates merges several gates with deny > ask > warn > allow.
// A user allow rule can never erase a deny/ask finding from the built-in gate.
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
				continue
			}
			if decision.Action == best.Action {
				best.Matches = appendUniqueRuleMatches(best.Matches, decision.Matches...)
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

func appendUniqueRuleMatches(dst []ToolCallRuleMatch, src ...ToolCallRuleMatch) []ToolCallRuleMatch {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, match := range dst {
		seen[match.RuleID+"\x00"+match.RuleName] = struct{}{}
	}
	for _, match := range src {
		key := match.RuleID + "\x00" + match.RuleName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dst = append(dst, match)
		if len(dst) >= maxTemporaryPolicyMatches {
			break
		}
	}
	return dst
}

func parseClaudePermissionRule(raw string, action schema.SyntaxFlowAction, severity schema.SyntaxFlowSeverity) (*ToolCallPolicyRule, error) {
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

	return &ToolCallPolicyRule{
		Action:    action,
		Severity:  severity,
		Tool:      toolName,
		Specifier: specifier,
		Raw:       raw,
		RuleID:    claudePermissionRuleID(action, raw),
	}, nil
}

func claudePermissionRuleID(action schema.SyntaxFlowAction, raw string) string {
	sum := sha1.Sum([]byte(string(action) + "\x00" + raw))
	return "claude-code-" + hex.EncodeToString(sum[:6])
}

func (r *ToolCallPolicyRule) decision(evidence string) *ToolCallRuleDecision {
	message := fmt.Sprintf("tool permission rule matched: %s", r.Raw)
	if evidence != "" {
		message += " [" + evidence + "]"
	}
	return &ToolCallRuleDecision{
		Action:   r.Action,
		Severity: r.Severity,
		RuleName: r.Raw,
		RuleID:   r.RuleID,
		Reason:   message,
		Matches: []ToolCallRuleMatch{{
			RuleName: r.Raw,
			RuleID:   r.RuleID,
			Severity: string(r.Severity),
			Message:  message,
			Risk:     "tool-permission",
		}},
	}
}

// Matches reports whether the rule applies to one tool call. The evidence is
// the concrete command/path/url that matched, for review cards and logs.
func (r *ToolCallPolicyRule) Matches(toolName string, params aitool.InvokeParams) (bool, string) {
	if r == nil {
		return false, ""
	}
	toolName = strings.TrimSpace(toolName)

	switch strings.ToLower(strings.TrimSpace(r.Tool)) {
	case "bash":
		// Claude Code's Bash rule is about the command, not the transport. Apply
		// it to every shell tool so cmd/powershell cannot sidestep a Bash rule.
		if !isShellToolName(toolName) {
			return false, ""
		}
		command := utils.InterfaceToString(params["command"])
		if strings.TrimSpace(command) == "" {
			command = utils.InterfaceToString(params["cmd"])
		}
		if strings.TrimSpace(command) == "" {
			return false, ""
		}
		return r.matchesBash(command)
	case "read":
		// Claude Code blocks edits/writes on paths denied by Read(...) as well.
		if !isReadToolName(toolName) && !isWriteToolName(toolName) {
			return false, ""
		}
		return r.matchesPath(params)
	case "edit", "write", "multiedit", "notebookedit":
		if !isWriteToolName(toolName) {
			return false, ""
		}
		return r.matchesPath(params)
	case "webfetch", "websearch":
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

func (r *ToolCallPolicyRule) matchesBash(command string) (bool, string) {
	spec := strings.TrimSpace(r.Specifier)
	commands, err := bash2ssa.ExtractShellCommands(command)
	if err != nil || len(commands) == 0 {
		// Parsing is advisory here; a malformed command still gets the raw
		// pattern check instead of silently bypassing a deny rule.
		commands = []bash2ssa.ShellCommand{{Text: command, EffectiveText: command}}
	}

	matched := 0
	total := 0
	evidence := ""
	for _, extracted := range commands {
		text := strings.TrimSpace(extracted.Text)
		if text == "" {
			continue
		}
		total++
		effective := strings.TrimSpace(extracted.EffectiveText)

		if r.Action == schema.SFR_ACTION_ALLOW {
			// Claude Code requires every simple command, including static
			// sh -c/eval payloads, to satisfy an allow rule.
			if !matchBashPattern(spec, text) {
				return false, ""
			}
			matched++
			if evidence == "" {
				evidence = text
			}
			continue
		}

		if matchBashPattern(spec, text) || (effective != "" && effective != text && matchBashPattern(spec, effective)) {
			if effective != "" && effective != text {
				evidence = effective
			} else {
				evidence = text
			}
			return true, evidence
		}
	}

	if r.Action == schema.SFR_ACTION_ALLOW {
		return total > 0 && matched == total, evidence
	}
	return false, ""
}

func (r *ToolCallPolicyRule) matchesPath(params aitool.InvokeParams) (bool, string) {
	for _, candidate := range collectPathValues(params) {
		if matchPathPattern(r.Specifier, candidate) {
			return true, candidate
		}
	}
	return false, ""
}

func (r *ToolCallPolicyRule) matchesDomain(params aitool.InvokeParams) (bool, string) {
	for _, candidate := range collectURLValues(params) {
		if matchDomainPattern(r.Specifier, candidate) {
			return true, candidate
		}
	}
	return false, ""
}

func matchBashPattern(spec, command string) bool {
	spec = strings.TrimSpace(spec)
	command = strings.TrimSpace(command)
	if spec == "" || spec == "*" {
		return true
	}
	if strings.HasSuffix(spec, ":*") {
		prefix := strings.TrimSpace(strings.TrimSuffix(spec, ":*"))
		if prefix == "" {
			return true
		}
		return command == prefix ||
			strings.HasPrefix(command, prefix+" ") ||
			strings.HasPrefix(command, prefix+"\t") ||
			strings.HasPrefix(command, prefix+"\n") ||
			strings.HasPrefix(command, prefix+":")
	}
	return globMatch(spec, command, false)
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

func isShellToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash", "shell", "sh", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
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
