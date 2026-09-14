package tests

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// G4 inventory of parser-rule alternatives extracted from a parser grammar.
// Unreachable rules (never referenced from the start rule) are recorded, not treated as uncovered.

type g4Alt struct {
	Index         int
	Text          string
	RequiredTok   []string
	RequiredRules []string
	OptionalTok   []string
	OptionalRules []string
	AllTok        []string
	AllRules      []string
}

type g4Rule struct {
	Name      string
	Alts      []g4Alt
	Reachable bool
}

type g4Inventory struct {
	Start       string
	Rules       map[string]*g4Rule
	Reachable   []string
	Unreachable []string
}

var g4RuleHeader = regexp.MustCompile(`(?m)(?:^|\n)\s*(?:public\s+|private\s+|protected\s+|fragment\s+)*([A-Za-z_][A-Za-z0-9_]*)\s*:`)

func parseParserG4(src, start string) *g4Inventory {
	cleaned := stripG4Comments(src)
	cleaned = regexp.MustCompile(`parser\s+grammar\s+\w+\s*;`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`options\s*\{[^}]*\}\s*;`).ReplaceAllString(cleaned, "")

	raw := map[string][]string{}
	matches := g4RuleHeader.FindAllStringSubmatchIndex(cleaned, -1)
	for i, m := range matches {
		name := cleaned[m[2]:m[3]]
		bodyStart := m[1]
		bodyEnd := len(cleaned)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		raw[name] = splitG4Alts(extractG4Body(cleaned[bodyStart:bodyEnd]))
	}
	names := map[string]struct{}{}
	for n := range raw {
		names[n] = struct{}{}
	}

	inv := &g4Inventory{Start: start, Rules: map[string]*g4Rule{}}
	refs := map[string][]string{}
	for name, alts := range raw {
		rule := &g4Rule{Name: name}
		for i, a := range alts {
			parsed := parseG4Alt(a, names, name)
			parsed.Index = i + 1
			rule.Alts = append(rule.Alts, parsed)
			for _, r := range parsed.AllRules {
				if _, ok := names[r]; ok {
					refs[name] = append(refs[name], r)
				}
			}
		}
		inv.Rules[name] = rule
	}

	seen := map[string]bool{}
	var walk func(string)
	walk = func(cur string) {
		if seen[cur] {
			return
		}
		if _, ok := inv.Rules[cur]; !ok {
			return
		}
		seen[cur] = true
		inv.Rules[cur].Reachable = true
		for _, r := range refs[cur] {
			walk(r)
		}
	}
	walk(start)

	for name, rule := range inv.Rules {
		if rule.Reachable {
			inv.Reachable = append(inv.Reachable, name)
		} else {
			inv.Unreachable = append(inv.Unreachable, name)
		}
	}
	sort.Strings(inv.Reachable)
	sort.Strings(inv.Unreachable)
	return inv
}

func (inv *g4Inventory) reachableAltCount() int {
	n := 0
	for _, name := range inv.Reachable {
		n += len(inv.Rules[name].Alts)
	}
	return n
}

func stripG4Comments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if src[i] == '/' && i+1 < len(src) {
			if src[i+1] == '/' {
				i += 2
				for i < len(src) && src[i] != '\n' && src[i] != '\r' {
					i++
				}
				continue
			}
			if src[i+1] == '*' {
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					i++
				}
				if i+1 < len(src) {
					i += 2
				}
				continue
			}
		}
		if src[i] == '\'' || src[i] == '"' {
			q := src[i]
			b.WriteByte(q)
			i++
			for i < len(src) {
				b.WriteByte(src[i])
				if src[i] == '\\' && i+1 < len(src) {
					i++
					b.WriteByte(src[i])
					i++
					continue
				}
				if src[i] == q {
					i++
					break
				}
				i++
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func skipG4Action(s string, i int) int {
	depth := 0
	n := len(s)
	for i < n {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		} else if s[i] == '\'' || s[i] == '"' {
			q := s[i]
			i++
			for i < n && s[i] != q {
				if s[i] == '\\' {
					i++
				}
				i++
			}
		}
		i++
	}
	return n
}

func extractG4Body(chunk string) string {
	var b strings.Builder
	depth := 0
	i := 0
	for i < len(chunk) {
		ch := chunk[i]
		if ch == '{' {
			j := skipG4Action(chunk, i)
			b.WriteString(chunk[i:j])
			i = j
			continue
		}
		if ch == '\'' || ch == '"' {
			q := ch
			b.WriteByte(ch)
			i++
			for i < len(chunk) {
				b.WriteByte(chunk[i])
				if chunk[i] == q {
					i++
					break
				}
				if chunk[i] == '\\' && i+1 < len(chunk) {
					i++
					b.WriteByte(chunk[i])
				}
				i++
			}
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		} else if ch == ';' && depth == 0 {
			break
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func splitG4Alts(body string) []string {
	var alts []string
	var buf strings.Builder
	depth := 0
	i := 0
	for i < len(body) {
		ch := body[i]
		if ch == '{' {
			j := skipG4Action(body, i)
			buf.WriteString(body[i:j])
			i = j
			continue
		}
		if ch == '\'' || ch == '"' {
			q := ch
			buf.WriteByte(ch)
			i++
			for i < len(body) {
				buf.WriteByte(body[i])
				if body[i] == '\\' && i+1 < len(body) {
					i++
					buf.WriteByte(body[i])
					i++
					continue
				}
				if body[i] == q {
					i++
					break
				}
				i++
			}
			continue
		}
		if ch == '(' {
			depth++
			buf.WriteByte(ch)
			i++
			continue
		}
		if ch == ')' {
			depth--
			buf.WriteByte(ch)
			i++
			continue
		}
		if ch == '|' && depth == 0 {
			alts = append(alts, strings.TrimSpace(buf.String()))
			buf.Reset()
			i++
			continue
		}
		buf.WriteByte(ch)
		i++
	}
	alts = append(alts, strings.TrimSpace(buf.String()))
	out := alts[:0]
	for _, a := range alts {
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func parseG4Alt(alt string, names map[string]struct{}, parent string) g4Alt {
	var out g4Alt
	out.Text = strings.Join(strings.Fields(alt), " ")
	i := 0
	n := len(alt)
	for i < n {
		for i < n && unicode.IsSpace(rune(alt[i])) {
			i++
		}
		if i >= n {
			break
		}
		if alt[i] == '{' {
			j := skipG4Action(alt, i)
			rest := strings.TrimLeft(alt[j:], " \t\n\r")
			if strings.HasPrefix(rest, "?") {
				i = len(alt) - len(rest) + 1
			} else {
				i = j
			}
			continue
		}
		if alt[i] == '\'' {
			j := i + 1
			for j < n {
				if alt[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				if alt[j] == '\'' {
					j++
					break
				}
				j++
			}
			tok := unescapeG4Token(alt[i+1 : j-1])
			var suf string
			i, suf = takeG4Suffix(alt, j)
			out.AllTok = append(out.AllTok, tok)
			if suf == "?" || suf == "*" {
				out.OptionalTok = append(out.OptionalTok, tok)
			} else {
				out.RequiredTok = append(out.RequiredTok, tok)
			}
			continue
		}
		if alt[i] == '(' {
			depth := 1
			i++
			start := i
			for i < n && depth > 0 {
				if alt[i] == '{' {
					i = skipG4Action(alt, i)
					continue
				}
				if alt[i] == '\'' || alt[i] == '"' {
					q := alt[i]
					i++
					for i < n && alt[i] != q {
						if alt[i] == '\\' {
							i++
						}
						i++
					}
					if i < n {
						i++
					}
					continue
				}
				if alt[i] == '(' {
					depth++
				} else if alt[i] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
				i++
			}
			inner := alt[start:i]
			if i < n && alt[i] == ')' {
				i++
			}
			var suf string
			i, suf = takeG4Suffix(alt, i)
			sub := parseG4Alt(inner, names, parent)
			out.AllTok = append(out.AllTok, sub.AllTok...)
			out.AllRules = append(out.AllRules, sub.AllRules...)
			if suf == "?" || suf == "*" {
				out.OptionalTok = append(out.OptionalTok, sub.RequiredTok...)
				out.OptionalTok = append(out.OptionalTok, sub.OptionalTok...)
				out.OptionalRules = append(out.OptionalRules, sub.RequiredRules...)
				out.OptionalRules = append(out.OptionalRules, sub.OptionalRules...)
			} else {
				out.RequiredTok = append(out.RequiredTok, sub.RequiredTok...)
				out.RequiredRules = append(out.RequiredRules, sub.RequiredRules...)
				out.OptionalTok = append(out.OptionalTok, sub.OptionalTok...)
				out.OptionalRules = append(out.OptionalRules, sub.OptionalRules...)
			}
			continue
		}
		if ident, next, ok := readG4Ident(alt, i); ok {
			var suf string
			i, suf = takeG4Suffix(alt, next)
			if _, isRule := names[ident]; isRule {
				out.AllRules = append(out.AllRules, ident)
				if ident == parent || suf == "?" || suf == "*" {
					out.OptionalRules = append(out.OptionalRules, ident)
				} else {
					out.RequiredRules = append(out.RequiredRules, ident)
				}
			} else {
				// lexer token name (Integer_Literal, DEFAULT, ...)
				out.AllTok = append(out.AllTok, ident)
				if suf == "?" || suf == "*" {
					out.OptionalTok = append(out.OptionalTok, ident)
				} else {
					out.RequiredTok = append(out.RequiredTok, ident)
				}
			}
			continue
		}
		_, w := utf8.DecodeRuneInString(alt[i:])
		if w <= 0 {
			w = 1
		}
		i += w
	}
	req := out.RequiredRules[:0]
	for _, r := range out.RequiredRules {
		if r != parent {
			req = append(req, r)
		}
	}
	out.RequiredRules = req
	return out
}

func takeG4Suffix(s string, i int) (int, string) {
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i++
	}
	suf := ""
	for i < len(s) && (s[i] == '?' || s[i] == '*' || s[i] == '+') {
		suf += string(s[i])
		i++
	}
	return i, suf
}

func readG4Ident(s string, i int) (string, int, bool) {
	if i >= len(s) {
		return "", i, false
	}
	r, w := utf8.DecodeRuneInString(s[i:])
	if !unicode.IsLetter(r) && r != '_' {
		return "", i, false
	}
	j := i + w
	for j < len(s) {
		r, w = utf8.DecodeRuneInString(s[j:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			j += w
			continue
		}
		break
	}
	return s[i:j], j, true
}

func unescapeG4Token(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\'', '"', '\\':
				b.WriteByte(s[i])
			default:
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func altKey(rule string, alt int) string {
	return fmt.Sprintf("%s#%d", rule, alt)
}
