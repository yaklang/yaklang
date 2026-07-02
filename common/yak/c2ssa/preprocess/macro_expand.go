package preprocess

import (
	"fmt"
	"strings"
)

const maxMacroExpandDepth = 64

// ExpandFunctionMacros expands function-like and object-like macros defined in src.
// #include and conditional compilation directives are left unchanged.
// Successfully parsed #define lines are removed from the output.
func ExpandFunctionMacros(src string) (string, error) {
	return ExpandFunctionMacrosWithTables(src, NewMacroTables())
}

// ExpandFunctionMacrosWithTables expands macros using a project-wide base table plus local #define/#undef.
func ExpandFunctionMacrosWithTables(src string, base MacroTables) (string, error) {
	collected := collectFunctionMacros(src, exportToMacroTables(base))
	env := &macroEnv{
		tables:   collected.tables,
		maxDepth: maxMacroExpandDepth,
	}
	expanded := env.expandSource(collected.output)
	expanded = collapsePreprocessorContinuations(expanded)
	return expanded, nil
}

type macroEnv struct {
	tables   macroTables
	depth    int
	maxDepth int
}

func (e *macroEnv) expandSource(src string) string {
	return e.expandSourceWithState(src, &macroScanState{})
}

func (e *macroEnv) expandSourceWithState(src string, st *macroScanState) string {
	tokens := tokenizeMacroSourceWithState(src, st)
	expanded := e.expandTokens(tokens)
	return tokensToString(expanded)
}

func (e *macroEnv) expandTokens(tokens []macroToken) []macroToken {
	for {
		next, changed := e.expandOnce(tokens)
		tokens = next
		if !changed {
			return tokens
		}
	}
}

func (e *macroEnv) expandOnce(tokens []macroToken) ([]macroToken, bool) {
	var out []macroToken
	changed := false
	i := 0
	for i < len(tokens) {
		if tokens[i].kind == macroTokComment {
			out = append(out, tokens[i])
			i++
			continue
		}
		if tokens[i].kind == macroTokIdent {
			name := tokens[i].text
			j := skipWhitespaceTokens(tokens, i+1)
			if j < len(tokens) && tokens[j].text == "(" {
				if fm, ok := e.tables.function[name]; ok {
					args, end, err := parseMacroCallArgs(tokens, j+1)
					if err == nil {
						e.depth++
						if e.depth > e.maxDepth {
							e.depth--
							out = append(out, tokens[i])
							i++
							continue
						}
						expandedArgs := make([][]macroToken, len(args))
						for ai, arg := range args {
							expandedArgs[ai] = e.expandTokens(arg)
						}
						repl := e.substituteMacro(fm, expandedArgs)
						repl = e.expandTokens(repl)
						e.depth--
						out = append(out, repl...)
						i = end + 1
						changed = true
						continue
					}
				}
			} else if body, ok := e.tables.object[name]; ok {
				repl := tokenizeMacroSource(body)
				repl = e.expandTokens(repl)
				out = append(out, repl...)
				i++
				changed = true
				continue
			}
		}
		out = append(out, tokens[i])
		i++
	}
	return out, changed
}

func parseMacroCallArgs(tokens []macroToken, start int) ([][]macroToken, int, error) {
	var args [][]macroToken
	var cur []macroToken
	depth := 1
	i := start
	for i < len(tokens) && depth > 0 {
		t := tokens[i]
		switch {
		case t.kind == macroTokPunct && t.text == "(":
			depth++
			cur = append(cur, t)
		case t.kind == macroTokPunct && t.text == ")":
			depth--
			if depth == 0 {
				args = append(args, trimArgTokens(cur))
				return args, i, nil
			}
			cur = append(cur, t)
		case t.kind == macroTokPunct && t.text == "," && depth == 1:
			args = append(args, trimArgTokens(cur))
			cur = nil
		default:
			cur = append(cur, t)
		}
		i++
	}
	return nil, 0, fmt.Errorf("unterminated macro argument list")
}

func trimArgTokens(tokens []macroToken) []macroToken {
	start, end := 0, len(tokens)
	for start < end && tokens[start].kind == macroTokWhitespace {
		start++
	}
	for end > start && tokens[end-1].kind == macroTokWhitespace {
		end--
	}
	return tokens[start:end]
}

func (e *macroEnv) substituteMacro(fm functionMacro, args [][]macroToken) []macroToken {
	body := tokenizeMacroSource(fm.body)
	argMap := make(map[string][]macroToken, len(fm.params))
	for i, p := range fm.params {
		if i < len(args) {
			argMap[p] = args[i]
		} else {
			argMap[p] = nil
		}
	}
	if fm.variadic {
		var va []macroToken
		for i := len(fm.params); i < len(args); i++ {
			if i > len(fm.params) {
				va = append(va, macroToken{macroTokPunct, ","})
			}
			va = append(va, args[i]...)
		}
		argMap[vaArgsName] = va
	}
	return expandMacroBody(body, argMap)
}

func expandMacroBody(body []macroToken, argMap map[string][]macroToken) []macroToken {
	var out []macroToken
	i := 0
	for i < len(body) {
		// stringification: # param
		if body[i].kind == macroTokPunct && body[i].text == "#" {
			j := skipWhitespaceTokens(body, i+1)
			if j < len(body) && body[j].kind == macroTokIdent {
				if arg, ok := argMap[body[j].text]; ok {
					out = append(out, macroToken{macroTokString, stringifyMacroTokens(arg)})
					i = j + 1
					continue
				}
			}
		}
		// token pasting: lhs ## rhs (whitespace around ## is ignored, as in ISO C)
		hashPos := skipWhitespaceTokens(body, i+1)
		if hashPos < len(body) && body[hashPos].kind == macroTokPunct && body[hashPos].text == "##" {
			left := resolveBodyToken(body[i], argMap)
			k := skipWhitespaceTokens(body, hashPos+1)
			if k < len(body) {
				right := resolveBodyToken(body[k], argMap)
				merged := pasteMacroTokens(left, right)
				out = append(out, merged...)
				i = k + 1
				continue
			}
		}
		if body[i].kind == macroTokIdent {
			if arg, ok := argMap[body[i].text]; ok {
				out = append(out, arg...)
				i++
				continue
			}
		}
		out = append(out, body[i])
		i++
	}
	return out
}

func resolveBodyToken(t macroToken, argMap map[string][]macroToken) []macroToken {
	if t.kind == macroTokIdent {
		if arg, ok := argMap[t.text]; ok {
			return arg
		}
	}
	return []macroToken{t}
}

func pasteMacroTokens(left, right []macroToken) []macroToken {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	l := left[len(left)-1]
	r := right[0]
	mergedText := l.text + r.text
	kind := macroTokPunct
	if len(mergedText) > 0 {
		allIdent := true
		for _, ch := range mergedText {
			if !isMacroIdentPart(ch) {
				allIdent = false
				break
			}
		}
		if allIdent && isMacroIdentStart(rune(mergedText[0])) {
			kind = macroTokIdent
		}
	}
	combined := append(left[:len(left)-1], macroToken{kind, mergedText})
	combined = append(combined, right[1:]...)
	return combined
}

func stringifyMacroTokens(tokens []macroToken) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, t := range tokens {
		switch t.kind {
		case macroTokString:
			inner := t.text
			if len(inner) >= 2 && inner[0] == '"' && inner[len(inner)-1] == '"' {
				inner = inner[1 : len(inner)-1]
			}
			b.WriteString(inner)
		case macroTokChar:
			b.WriteString(t.text)
		default:
			b.WriteString(t.text)
		}
	}
	b.WriteByte('"')
	return b.String()
}
