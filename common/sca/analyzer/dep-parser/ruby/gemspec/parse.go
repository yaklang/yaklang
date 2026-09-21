package gemspec

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/model"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const specNewStr = "Gem::Specification.new"

var newVarRegexp = regexp.MustCompile(`\|(?P<var>.*)\|`)

type gemReq struct {
	types.Requirement
	generic bool
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) (libs []types.Library, deps []types.Dependency, err error) {
	ctx := types.ContextOf(r)
	var newVar, name, version, license, homepage string
	var rawLicenses []string
	var diags []model.Diagnostic
	var rawReqs []gemReq
	start, end, n := 0, 0, 0

	scanner := textdecode.NewRecordLines(ctx, r)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		n++
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, specNewStr) {
			newVar = findSubString(newVarRegexp, line, "var")
			if start == 0 {
				start = n
			}
			end = n
		}
		if newVar == "" {
			continue
		}
		end = n
		switch {
		case strings.HasPrefix(line, newVar+".name"):
			name, err = literalAssignment(line)
			if err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(line, newVar+".version"):
			version, err = literalAssignment(line)
			if err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(line, newVar+".homepage"):
			hp, herr := literalAssignment(line)
			if herr != nil {
				diags = append(diags, model.Diagnostic{Code: "unsupported_syntax", Stage: "parse", Reason: herr.Error(), Incomplete: true})
			} else {
				homepage = hp
			}
		case strings.HasPrefix(line, newVar+".licenses"):
			rawLicenses, err = licenseAssignment(line, true)
			if err != nil {
				diags = append(diags, diagnosticFor(err))
				rawLicenses = nil
				license = ""
				continue
			}
			license = strings.Join(rawLicenses, ", ")
		case strings.HasPrefix(line, newVar+".license"):
			rawLicenses, err = licenseAssignment(line, false)
			if err != nil {
				diags = append(diags, diagnosticFor(err))
				rawLicenses = nil
				license = ""
				continue
			}
			license = strings.Join(rawLicenses, ", ")
		default:
			req, ok, derr := parseAddDependency(newVar, line)
			if derr != nil {
				diags = append(diags, diagnosticFor(derr))
				continue
			}
			if ok {
				rawReqs = append(rawReqs, req)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("malformed_input: gemspec: %w", err)
	}
	if name == "" || version == "" {
		return nil, nil, fmt.Errorf("malformed_input: gemspec name or version")
	}
	if err := budget.From(ctx).Result(budget.SizeOfPackage(name, version, license)); err != nil {
		return nil, nil, err
	}
	id := name + "@" + version
	lib := types.Library{
		ID:          id,
		Name:        name,
		Version:     version,
		License:     license,
		RawLicenses: rawLicenses,
		Source:      homepage,
		Diagnostics: diags,
	}
	if start != 0 {
		if end < start {
			end = start
		}
		lib.Locations = []types.Location{{StartLine: start, EndLine: end}}
	}
	qs := uniqueGemReqs(rawReqs)
	var outDeps []types.Dependency
	if len(qs) > 0 {
		for _, q := range qs {
			if err := budget.From(ctx).Result(budget.SizeOfEdge() + budget.SizeOfString(q.Target) + budget.SizeOfString(q.Constraint)); err != nil {
				return nil, nil, err
			}
		}
		outDeps = []types.Dependency{{ID: id, Requirements: qs}}
	}
	return []types.Library{lib}, outDeps, ctx.Err()
}

func diagnosticFor(err error) model.Diagnostic {
	code := "malformed_input"
	msg := err.Error()
	if strings.HasPrefix(msg, "unsupported_syntax:") {
		code = "unsupported_syntax"
	}
	return model.Diagnostic{Code: code, Stage: "parse", Reason: msg, Incomplete: true}
}

func parseAddDependency(varName, line string) (gemReq, bool, error) {
	var out gemReq
	prefix := varName + "."
	if !strings.HasPrefix(line, prefix) {
		return out, false, nil
	}
	rest := line[len(prefix):]
	switch {
	case strings.HasPrefix(rest, "add_runtime_dependency"):
		rest = strings.TrimSpace(rest[len("add_runtime_dependency"):])
	case strings.HasPrefix(rest, "add_development_dependency"):
		out.Scope = "dev"
		rest = strings.TrimSpace(rest[len("add_development_dependency"):])
	case strings.HasPrefix(rest, "add_dependency"):
		out.generic = true
		rest = strings.TrimSpace(rest[len("add_dependency"):])
	default:
		return out, false, nil
	}
	if rest == "" {
		return out, false, fmt.Errorf("malformed_input: truncated gemspec dependency")
	}
	opened := false
	if strings.HasPrefix(rest, "(") {
		opened = true
		rest = strings.TrimSpace(rest[1:])
	}
	name, rest, err := takeRubyString(rest)
	if err != nil {
		return out, false, err
	}
	rest = skipRubyFreeze(rest)
	var parts []string
	for {
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, ",") {
			break
		}
		rest = strings.TrimSpace(rest[1:])
		if rest == "" {
			return out, false, fmt.Errorf("malformed_input: truncated gemspec dependency")
		}
		if rest[0] == '[' {
			more, next, err := takeRubyRequirementList(rest)
			if err != nil {
				return out, false, err
			}
			parts = append(parts, more...)
			rest = skipRubyFreeze(next)
			continue
		}
		val, next, err := takeRubyString(rest)
		if err != nil {
			return out, false, err
		}
		parts = append(parts, val)
		rest = skipRubyFreeze(next)
	}
	if err := finishRubyCall(rest, opened); err != nil {
		return out, false, err
	}
	out.Target = name
	out.Constraint = strings.Join(parts, ", ")
	if out.Constraint != "" {
		out.Condition = name + "@" + out.Constraint
	} else {
		out.Condition = name
	}
	return out, true, nil
}

func skipRubyFreeze(s string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ".freeze"))
}

func finishRubyCall(rest string, opened bool) error {
	rest = skipRubyFreeze(rest)
	if opened {
		if rest == "" {
			return fmt.Errorf("malformed_input: truncated gemspec dependency")
		}
		if !strings.HasPrefix(rest, ")") {
			return rubyCallTail(rest)
		}
		rest = skipRubyFreeze(strings.TrimSpace(rest[1:]))
	}
	if rest == "" || strings.HasPrefix(rest, "#") {
		return nil
	}
	return rubyCallTail(rest)
}

func rubyCallTail(rest string) error {
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, ",") {
		return fmt.Errorf("malformed_input: truncated gemspec dependency")
	}
	switch rest[0] {
	case '+', '*', '/', '%', '|', '&', '<', '>', '?', ':', '{', '#', '`', '\\':
		return fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
	}
	if rest[0] == '.' {
		return fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
	}
	if rest[0] == '_' || rest[0] == '@' || rest[0] == '$' ||
		(rest[0] >= 'A' && rest[0] <= 'Z') || (rest[0] >= 'a' && rest[0] <= 'z') {
		return fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
	}
	return fmt.Errorf("malformed_input: gemspec dependency suffix")
}

func takeRubyString(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("malformed_input: truncated gemspec string")
	}
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' {
				return "", s, fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
			}
			if quote == '"' && s[i] == '#' && i+1 < len(s) && s[i+1] == '{' {
				return "", s, fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
			}
			if s[i] == quote {
				return s[1:i], s[i+1:], nil
			}
		}
		return "", s, fmt.Errorf("malformed_input: truncated gemspec string")
	}
	if len(s) >= 2 && (strings.HasPrefix(s, "%q") || strings.HasPrefix(s, "%Q")) {
		interp := strings.HasPrefix(s, "%Q")
		body := s[2:]
		if body == "" {
			return "", s, fmt.Errorf("malformed_input: truncated gemspec string")
		}
		open := body[0]
		close := rubyCloser(open)
		depth := 1
		for i := 1; i < len(body); i++ {
			if interp && body[i] == '#' && i+1 < len(body) && body[i+1] == '{' {
				return "", s, fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
			}
			if open != close && body[i] == open {
				depth++
			} else if body[i] == close {
				depth--
				if depth == 0 {
					return body[1:i], body[i+1:], nil
				}
			}
		}
		return "", s, fmt.Errorf("malformed_input: truncated gemspec string")
	}
	return "", s, fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
}

func rubyCloser(b byte) byte {
	switch b {
	case '<':
		return '>'
	case '{':
		return '}'
	case '(':
		return ')'
	case '[':
		return ']'
	default:
		return b
	}
}

func takeRubyRequirementList(s string) ([]string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '[' {
		return nil, s, fmt.Errorf("malformed_input: truncated gemspec requirement")
	}
	var parts []string
	inner := strings.TrimSpace(s[1:])
	for {
		inner = strings.TrimSpace(inner)
		if inner == "" {
			return nil, s, fmt.Errorf("malformed_input: truncated gemspec requirement")
		}
		if inner[0] == ']' {
			return parts, inner[1:], nil
		}
		val, rest, err := takeRubyString(inner)
		if err != nil {
			return nil, s, err
		}
		parts = append(parts, val)
		inner = skipRubyFreeze(rest)
		if strings.HasPrefix(inner, "]") {
			return parts, inner[1:], nil
		}
		if !strings.HasPrefix(inner, ",") {
			return nil, s, fmt.Errorf("malformed_input: missing gemspec array separator")
		}
		inner = strings.TrimSpace(inner[1:])
	}
}

func uniqueGemReqs(in []gemReq) []types.Requirement {
	type key struct{ target, constraint string }
	groups := map[key][]gemReq{}
	var order []key
	for _, r := range in {
		if r.Target == "" {
			continue
		}
		k := key{r.Target, r.Constraint}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var out []types.Requirement
	for _, k := range order {
		group := groups[k]
		var specific, generic []gemReq
		for _, r := range group {
			if r.generic && r.Scope == "" {
				generic = append(generic, r)
			} else {
				specific = append(specific, r)
			}
		}
		keep := specific
		if len(keep) == 0 {
			keep = generic[:1]
		}
		seen := map[string]bool{}
		for _, r := range keep {
			if seen[r.Scope] {
				continue
			}
			seen[r.Scope] = true
			out = append(out, r.Requirement)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		if out[i].Constraint != out[j].Constraint {
			return out[i].Constraint < out[j].Constraint
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}

// Evaluate no Ruby. Even a quoted prefix followed by an expression is not a
// static value, and interpolation must not become a fictitious package version.
func literalAssignment(line string) (string, error) {
	_, s, ok := strings.Cut(line, "=")
	s = strings.TrimSpace(s)
	// A trailing comment outside the closed literal is not part of its value.
	if len(s) > 1 && (s[0] == '\'' || s[0] == '"') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			end++
			tail := strings.TrimSpace(s[end+1:])
			tail = strings.TrimSpace(strings.TrimPrefix(tail, ".freeze"))
			if strings.HasPrefix(tail, "#") {
				s = s[:end+1]
			}
		}
	}
	s = strings.TrimSuffix(s, ".freeze")
	if !ok || len(s) < 2 || (s[0] != '\'' && s[0] != '"') || s[len(s)-1] != s[0] || strings.ContainsAny(s[1:len(s)-1], "\\\"'`#") {
		return "", fmt.Errorf("unsupported_syntax: dynamic Ruby assignment")
	}
	return s[1 : len(s)-1], nil
}

func findSubString(re *regexp.Regexp, line, name string) string {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[re.SubexpIndex(name)]
}

// Retain array boundaries: a comma in a single custom license is literal text.
func licenseAssignment(line string, array bool) ([]string, error) {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return nil, fmt.Errorf("malformed_input: gemspec license assignment")
	}
	var out []string
	var tail string
	var err error
	if array {
		out, tail, err = takeRubyRequirementList(value)
	} else {
		var license string
		license, tail, err = takeRubyString(value)
		out = []string{license}
	}
	if err != nil {
		return nil, err
	}
	if err = finishRubyCall(tail, false); err != nil {
		return nil, err
	}
	return out, nil
}
