package preprocess

import (
	"strconv"
	"strings"
	"unicode"
)

// ppExprEnv supplies macro values for #if expression evaluation.
type ppExprEnv struct {
	local  *MacroEnvironment
	global *MacroEnvironment
	defs   map[string]string
}

func newPPExprEnv(local, global *MacroEnvironment, defs map[string]string) *ppExprEnv {
	return &ppExprEnv{local: local, global: global, defs: defs}
}

func (e *ppExprEnv) lookupObject(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if v, ok := e.defs[name]; ok {
		return v, true
	}
	if e.local != nil {
		if v, ok := e.local.tables.Object[name]; ok {
			return v, true
		}
	}
	if e.global != nil {
		flat := e.global.Flatten()
		if v, ok := flat.Object[name]; ok {
			return v, true
		}
	}
	return "", false
}

func (e *ppExprEnv) isDefined(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := e.defs[name]; ok {
		return true
	}
	if e.local != nil && e.local.IsDefined(name) {
		return true
	}
	if e.global != nil && e.global.IsDefined(name) {
		return true
	}
	return false
}

type ifTokKind int

const (
	ifTokEOF ifTokKind = iota
	ifTokNumber
	ifTokIdent
	ifTokDefined
	ifTokOp
)

type ifTok struct {
	kind ifTokKind
	text string
}

type ifParser struct {
	toks []ifTok
	pos  int
}

func preprocessIfTokens(tokens []ifTok, env *ppExprEnv) []ifTok {
	tokens = replaceDefinedTokens(tokens, env)
	return expandIfExprIdentifiers(tokens, env)
}

func replaceDefinedTokens(tokens []ifTok, env *ppExprEnv) []ifTok {
	var out []ifTok
	for i := 0; i < len(tokens); i++ {
		if tokens[i].kind != ifTokDefined {
			out = append(out, tokens[i])
			continue
		}
		i++
		var name string
		if i < len(tokens) && tokens[i].kind == ifTokOp && tokens[i].text == "(" {
			i++
			if i < len(tokens) && tokens[i].kind == ifTokIdent {
				name = tokens[i].text
				i++
			}
			if i < len(tokens) && tokens[i].kind == ifTokOp && tokens[i].text == ")" {
				i++
			}
		} else if i < len(tokens) && tokens[i].kind == ifTokIdent {
			name = tokens[i].text
			i++
		}
		i--
		if env.isDefined(name) {
			out = append(out, ifTok{kind: ifTokNumber, text: "1"})
		} else {
			out = append(out, ifTok{kind: ifTokNumber, text: "0"})
		}
	}
	out = append(out, ifTok{kind: ifTokEOF})
	return out
}

// EvalPreprocessorCondition evaluates a #if/#elif expression.
func EvalPreprocessorCondition(expr string, local, global *MacroEnvironment, defs map[string]string) bool {
	expr = normalizeLineEndings(expr)
	env := newPPExprEnv(local, global, defs)
	tokens := preprocessIfTokens(tokenizeIfExpr(expr), env)
	v, ok := evalIfExprTokens(tokens)
	if !ok {
		return false
	}
	return v != 0
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func normalizeLogicalLine(s string) string {
	return strings.TrimRight(normalizeLineEndings(s), " \t")
}

func tokenizeIfExpr(expr string) []ifTok {
	expr = strings.TrimSpace(expr)
	var out []ifTok
	i := 0
	for i < len(expr) {
		if unicode.IsSpace(rune(expr[i])) {
			i++
			continue
		}
		if i+1 < len(expr) {
			two := expr[i : i+2]
			switch two {
			case "&&", "||", "<<", ">>", "<=", ">=", "==", "!=":
				out = append(out, ifTok{kind: ifTokOp, text: two})
				i += 2
				continue
			}
		}
		switch expr[i] {
		case '+', '-', '*', '/', '%', '&', '|', '^', '~', '!', '<', '>', '(', ')':
			out = append(out, ifTok{kind: ifTokOp, text: string(expr[i])})
			i++
			continue
		}
		if expr[i] == '0' && i+1 < len(expr) && (expr[i+1] == 'x' || expr[i+1] == 'X') {
			j := i + 2
			for j < len(expr) && isHexDigit(expr[j]) {
				j++
			}
			out = append(out, ifTok{kind: ifTokNumber, text: expr[i:j]})
			i = j
			continue
		}
		if isDigit(expr[i]) {
			j := i
			for j < len(expr) && isDigit(expr[j]) {
				j++
			}
			out = append(out, ifTok{kind: ifTokNumber, text: expr[i:j]})
			i = j
			continue
		}
		if expr[i] == '_' || unicode.IsLetter(rune(expr[i])) {
			j := i
			for j < len(expr) {
				c := expr[j]
				if c == '_' || isDigit(c) || unicode.IsLetter(rune(c)) {
					j++
					continue
				}
				break
			}
			name := expr[i:j]
			if name == "defined" {
				out = append(out, ifTok{kind: ifTokDefined, text: name})
			} else {
				out = append(out, ifTok{kind: ifTokIdent, text: name})
			}
			i = j
			continue
		}
		i++
	}
	out = append(out, ifTok{kind: ifTokEOF})
	return out
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func expandIfExprIdentifiers(tokens []ifTok, env *ppExprEnv) []ifTok {
	var out []ifTok
	for _, t := range tokens {
		if t.kind == ifTokEOF {
			break
		}
		if t.kind != ifTokIdent {
			out = append(out, t)
			continue
		}
		if body, ok := env.lookupObject(t.text); ok {
			sub := tokenizeIfExpr(body)
			if len(sub) > 0 && sub[len(sub)-1].kind == ifTokEOF {
				sub = sub[:len(sub)-1]
			}
			out = append(out, sub...)
			continue
		}
		if env.isDefined(t.text) {
			out = append(out, ifTok{kind: ifTokNumber, text: "1"})
		} else {
			out = append(out, ifTok{kind: ifTokNumber, text: "0"})
		}
	}
	out = append(out, ifTok{kind: ifTokEOF})
	return out
}

func evalIfExprTokens(tokens []ifTok) (int64, bool) {
	p := &ifParser{toks: tokens}
	v, ok := p.parseExpr(0)
	if !ok {
		return 0, false
	}
	if p.peek().kind != ifTokEOF {
		return 0, false
	}
	return v, true
}

func (p *ifParser) peek() ifTok {
	if p.pos >= len(p.toks) {
		return ifTok{kind: ifTokEOF}
	}
	return p.toks[p.pos]
}

func (p *ifParser) next() ifTok {
	t := p.peek()
	if t.kind != ifTokEOF {
		p.pos++
	}
	return t
}

func (p *ifParser) parseExpr(minBP int) (int64, bool) {
	left, ok := p.parseUnary()
	if !ok {
		return 0, false
	}
	for {
		op := p.peek()
		if op.kind != ifTokOp {
			break
		}
		bp := ifInfixBP(op.text)
		if bp < minBP {
			break
		}
		p.next()
		right, ok := p.parseExpr(bp + 1)
		if !ok {
			return 0, false
		}
		left, ok = applyIfBinOp(op.text, left, right)
		if !ok {
			return 0, false
		}
	}
	return left, true
}

func (p *ifParser) parseUnary() (int64, bool) {
	switch p.peek().kind {
	case ifTokDefined:
		return 0, false
	case ifTokNumber:
		t := p.next()
		v, err := strconv.ParseInt(t.text, 0, 64)
		return v, err == nil
	case ifTokOp:
		op := p.next().text
		switch op {
		case "+":
			return p.parseUnary()
		case "-":
			v, ok := p.parseUnary()
			return -v, ok
		case "!":
			v, ok := p.parseUnary()
			return boolToInt(v == 0), ok
		case "~":
			v, ok := p.parseUnary()
			return ^v, ok
		case "(":
			v, ok := p.parseExpr(0)
			if !ok {
				return 0, false
			}
			if p.next().text != ")" {
				return 0, false
			}
			return v, true
		}
	}
	return 0, false
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func ifInfixBP(op string) int {
	switch op {
	case "||":
		return 1
	case "&&":
		return 2
	case "|":
		return 3
	case "^":
		return 4
	case "&":
		return 5
	case "==", "!=":
		return 6
	case "<", ">", "<=", ">=":
		return 7
	case "<<", ">>":
		return 8
	case "+", "-":
		return 9
	case "*", "/", "%":
		return 10
	default:
		return 0
	}
}

func applyIfBinOp(op string, a, b int64) (int64, bool) {
	switch op {
	case "||":
		return boolToInt(a != 0 || b != 0), true
	case "&&":
		return boolToInt(a != 0 && b != 0), true
	case "|":
		return a | b, true
	case "^":
		return a ^ b, true
	case "&":
		return a & b, true
	case "==":
		return boolToInt(a == b), true
	case "!=":
		return boolToInt(a != b), true
	case "<":
		return boolToInt(a < b), true
	case ">":
		return boolToInt(a > b), true
	case "<=":
		return boolToInt(a <= b), true
	case ">=":
		return boolToInt(a >= b), true
	case "<<":
		return a << uint(b), true
	case ">>":
		return a >> uint(b), true
	case "+":
		return a + b, true
	case "-":
		return a - b, true
	case "*":
		return a * b, true
	case "/":
		if b == 0 {
			return 0, false
		}
		return a / b, true
	case "%":
		if b == 0 {
			return 0, false
		}
		return a % b, true
	default:
		return 0, false
	}
}
