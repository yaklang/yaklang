package sfaudit

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// Hit is a structured source-mode match produced by a rule.
type Hit struct {
	RuleID   string // codeaudit finding ID the rule is registered under
	Title    string // title from the rule's desc/alert block
	Severity string // normalized: critical / high / medium / low
	File     string // the map key the hit came from
	Line     int    // 1-based line of the match
	Snippet  string // trimmed single code line (max 240 chars)
}

// Engine executes embedded .sf rules over an in-memory file set.
type Engine struct {
	name  string
	files map[string]string
}

// NewEngine builds an engine over a path→content map. The name is used as the
// SyntaxFlow program name and must be non-empty (it prefixes hit URLs).
func NewEngine(name string, files map[string]string) *Engine {
	if name == "" {
		name = "sfaudit"
	}
	if files == nil {
		files = map[string]string{}
	}
	return &Engine{name: name, files: files}
}

// Files exposes the engine's file set.
func (e *Engine) Files() map[string]string {
	return e.files
}

// Run executes the rules identified by ruleIDs over the engine's files and
// returns all hits. Rule failures are collected into the returned error and
// do not abort the remaining rules.
func (e *Engine) Run(ctx context.Context, ruleIDs ...string) ([]Hit, error) {
	target := ssaapi.NewSourceQueryTarget(e.name, e.files)
	var (
		hits []Hit
		errs error
	)
	for _, id := range ruleIDs {
		rule, err := loadRule(id)
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			continue
		}
		res, err := target.SyntaxFlowRule(rule, ssaapi.QueryWithContext(ctx))
		if err != nil {
			errs = utils.JoinErrors(errs, err)
			continue
		}
		// Materialize risks in memory (no DB write), then stream them.
		_ = res.CreateRisk()
		for risk := range res.YieldRisk() {
			if risk == nil {
				continue
			}
			hits = append(hits, e.toHit(id, risk))
		}
	}
	return hits, errs
}

// toHit converts an SSARisk into a Hit, resolving the file path back to the
// original map key and deriving a single-line snippet from the file content.
func (e *Engine) toHit(id string, risk *schema.SSARisk) Hit {
	file := e.resolveFile(riskSourcePath(risk))
	snippet := lineSnippet(e.files[file], int(risk.Line))
	if snippet == "" {
		snippet = firstLine(risk.CodeFragment)
	}
	return Hit{
		RuleID:   id,
		Title:    risk.Title,
		Severity: NormalizeSeverity(risk.Severity),
		File:     file,
		Line:     int(risk.Line),
		Snippet:  snippet,
	}
}

// resolveFile maps a hit URL ("/<programName>/<key>") back to the map key.
// Map keys may use OS-specific separators, so matching normalizes to slashes
// and falls back to suffix matching.
func (e *Engine) resolveFile(raw string) string {
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimPrefix(raw, "/"+e.name)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return raw
	}
	norm := strings.ReplaceAll(trimmed, "\\", "/")
	for key := range e.files {
		if strings.ReplaceAll(key, "\\", "/") == norm {
			return key
		}
	}
	for key := range e.files {
		kn := strings.ReplaceAll(key, "\\", "/")
		if strings.HasSuffix(norm, kn) || strings.HasSuffix(kn, norm) {
			return key
		}
	}
	return trimmed
}

// riskSourcePath extracts a usable path from an SSARisk, preferring the raw
// code source URL and falling back to the CodeRange JSON.
func riskSourcePath(risk *schema.SSARisk) string {
	if risk.CodeSourceUrl != "" {
		return risk.CodeSourceUrl
	}
	if risk.CodeRange != "" {
		var cr struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(risk.CodeRange), &cr); err == nil {
			return cr.URL
		}
	}
	return ""
}

// NormalizeSeverity maps SyntaxFlow severities onto the codeaudit scale
// (critical/high/medium/low).
func NormalizeSeverity(sev schema.SyntaxFlowSeverity) string {
	switch schema.ValidSeverityType(sev) {
	case schema.SFR_SEVERITY_INFO:
		return "low"
	case schema.SFR_SEVERITY_WARNING:
		return "medium"
	default:
		return strings.ToLower(string(schema.ValidSeverityType(sev)))
	}
}

// lineSnippet returns the trimmed 1-based line from content, capped at 240
// chars (mirrors the previous codeaudit snippet behavior).
func lineSnippet(content string, line int) string {
	if content == "" || line < 1 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if line > len(lines) {
		return ""
	}
	s := strings.TrimSpace(lines[line-1])
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}

// firstLine extracts the first non-empty line of a multi-line fragment.
func firstLine(fragment string) string {
	for _, l := range strings.Split(fragment, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			if len(s) > 240 {
				return s[:240]
			}
			return s
		}
	}
	return ""
}

// ruleCache caches parsed rules by ID; rule structs are read-only metadata.
var ruleCache sync.Map // string -> *schema.SyntaxFlowRule

// loadRule parses (once) and caches the embedded rule for a codeaudit ID.
func loadRule(id string) (*schema.SyntaxFlowRule, error) {
	if v, ok := ruleCache.Load(id); ok {
		return v.(*schema.SyntaxFlowRule), nil
	}
	content, err := RuleContent(id)
	if err != nil {
		return nil, err
	}
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil {
		return nil, utils.Errorf("sfaudit: compile rule %q: %v", id, err)
	}
	rule := frame.GetRule()
	if rule == nil {
		rule = &schema.SyntaxFlowRule{}
	}
	if rule.Content == "" {
		rule.Content = content
	}
	rule.RuleName = id
	ruleCache.Store(id, rule)
	return rule, nil
}
