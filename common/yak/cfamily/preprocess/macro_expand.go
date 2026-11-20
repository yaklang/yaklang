package preprocess

import (
	"fmt"
	"strings"
)

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
		tables: collected.tables,
	}
	expanded := env.expandSource(collected.output)
	expanded = collapsePreprocessorContinuations(expanded)
	return expanded, nil
}

type macroEnv struct {
	tables             macroTables
	objectBodyTokens   map[string][]macroToken // session cache: tokenize only
	functionBodyTokens map[string][]macroToken
}

func newMacroEnvFromTables(tables MacroTables) *macroEnv {
	return &macroEnv{
		tables: exportToMacroTables(tables),
	}
}

// applyFunctionMacro installs a function-like macro in place. Any cached body
// tokens for the name are dropped, because a redefinition changes the body.
func (e *macroEnv) applyFunctionMacro(name string, fm functionMacro) {
	if e == nil {
		return
	}
	if e.tables.function == nil {
		e.tables.function = make(map[string]functionMacro)
	}
	e.tables.function[name] = fm
	delete(e.tables.object, name)
	e.forgetMacroTokens(name)
}

// applyObjectMacro installs an object-like macro in place.
func (e *macroEnv) applyObjectMacro(name, body string) {
	if e == nil {
		return
	}
	if e.tables.object == nil {
		e.tables.object = make(map[string]string)
	}
	e.tables.object[name] = body
	delete(e.tables.function, name)
	e.forgetMacroTokens(name)
}

// undefineMacro removes both forms of name in place.
func (e *macroEnv) undefineMacro(name string) {
	if e == nil {
		return
	}
	delete(e.tables.function, name)
	delete(e.tables.object, name)
	e.forgetMacroTokens(name)
}

func (e *macroEnv) forgetMacroTokens(name string) {
	if e.objectBodyTokens != nil {
		delete(e.objectBodyTokens, name)
	}
	if e.functionBodyTokens != nil {
		delete(e.functionBodyTokens, name)
	}
}

// cloneHideSet returns a shallow copy of h (or nil if h is empty).
// Expansion of a given macro only adds names to its own branch's copy, so a
// sibling expansion (e.g. the other side of a mutual recursion) is unaffected.
func cloneHideSet(h map[string]struct{}) map[string]struct{} {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(h)+1)
	for k := range h {
		out[k] = struct{}{}
	}
	return out
}

// withMacro returns a hide-set that additionally hides name.
// A fresh map is allocated so the caller's set is not mutated.
func withMacro(h map[string]struct{}, name string) map[string]struct{} {
	if _, ok := h[name]; ok {
		return h
	}
	out := make(map[string]struct{}, len(h)+1)
	for k := range h {
		out[k] = struct{}{}
	}
	out[name] = struct{}{}
	return out
}

func isHidden(h map[string]struct{}, name string) bool {
	_, ok := h[name]
	return ok
}

func (e *macroEnv) getObjectBodyTokens(name, body string) []macroToken {
	if e.objectBodyTokens == nil {
		e.objectBodyTokens = make(map[string][]macroToken)
	}
	if toks, ok := e.objectBodyTokens[name]; ok {
		return toks
	}
	toks := sanitizeMacroBodyTokens(tokenizeMacroSource(body))
	e.objectBodyTokens[name] = toks
	return toks
}

func (e *macroEnv) getFunctionBodyTokens(name string, fm functionMacro) []macroToken {
	if e.functionBodyTokens == nil {
		e.functionBodyTokens = make(map[string][]macroToken)
	}
	if toks, ok := e.functionBodyTokens[name]; ok {
		return toks
	}
	toks := sanitizeMacroBodyTokens(tokenizeMacroSource(fm.body))
	e.functionBodyTokens[name] = toks
	return toks
}

// lineMayNeedExpand cheaply checks whether line contains an identifier that is a known macro.
// False positives are safe (full expand still correct); false negatives must be avoided.
func (e *macroEnv) lineMayNeedExpand(line string) bool {
	if len(e.tables.function) == 0 && len(e.tables.object) == 0 {
		return false
	}
	for i := 0; i < len(line); {
		c := line[i]
		if isMacroIdentStart(rune(c)) {
			start := i
			i++
			for i < len(line) {
				r := rune(line[i])
				if !isMacroIdentPart(r) {
					break
				}
				i++
			}
			name := line[start:i]
			if _, ok := e.tables.object[name]; ok {
				return true
			}
			if _, ok := e.tables.function[name]; ok {
				return true
			}
			continue
		}
		i++
	}
	return false
}

func (e *macroEnv) expandSource(src string) string {
	return e.expandSourceWithState(src, &macroScanState{})
}

func (e *macroEnv) expandSourceWithState(src string, st *macroScanState) string {
	tokens := tokenizeMacroSourceWithState(src, st)
	expanded := e.expandTokens(tokens, nil)
	return tokensToString(expanded)
}

// expandTokens scans tokens once, recursively expanding each macro's replacement
// body. hidden is the set of macro names currently being expanded on this branch
// (ISO C "hide-set" / "painted blue"): a name in hidden is not expanded again, which
// terminates self-referential and mutually-recursive macros. A single recursive scan
// is sufficient because each macro's replacement is fully expanded before being
// emitted; re-scanning emitted tokens with the top-level (empty) hide-set would
// re-expand self-referential macros forever.
func (e *macroEnv) expandTokens(tokens []macroToken, hidden map[string]struct{}) []macroToken {
	return e.expandOnce(tokens, hidden)
}

func (e *macroEnv) expandOnce(tokens []macroToken, hidden map[string]struct{}) []macroToken {
	var out []macroToken
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
				if fm, ok := e.tables.function[name]; ok && !isHidden(hidden, name) {
					args, end, err := parseMacroCallArgs(tokens, j+1)
					if err == nil {
						// Arguments are expanded with the caller's hide-set (not this
						// macro's), so a macro name used as an argument is still expandable.
						expandedArgs := make([][]macroToken, len(args))
						for ai, arg := range args {
							expandedArgs[ai] = e.expandTokens(arg, hidden)
						}
						repl := e.substituteMacro(name, fm, expandedArgs)
						// The replacement body is expanded with this macro hidden so a
						// self-referential macro terminates after one expansion.
						out = append(out, e.expandTokens(repl, withMacro(hidden, name))...)
						i = end + 1
						continue
					}
				}
			}
			// Object-like macros expand even when the next token is '('.
			// glibc `#define __ASSERT_VOID_CAST (void)` used as `__ASSERT_VOID_CAST (0)`
			// must become `(void) (0)`, not be left unexpanded or eaten as a call.
			if body, ok := e.tables.object[name]; ok && !isHidden(hidden, name) {
				repl := e.getObjectBodyTokens(name, body)
				out = append(out, e.expandTokens(repl, withMacro(hidden, name))...)
				i++
				continue
			}
		}
		out = append(out, tokens[i])
		i++
	}
	return out
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
	for start < end && isMacroWhitespaceToken(tokens[start]) {
		start++
	}
	for end > start && isMacroWhitespaceToken(tokens[end-1]) {
		end--
	}
	return tokens[start:end]
}

func (e *macroEnv) substituteMacro(name string, fm functionMacro, args [][]macroToken) []macroToken {
	body := e.getFunctionBodyTokens(name, fm)
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
				// Keep a space after the comma: #__VA_ARGS__ stringifies the
				// token sequence, and the source usually has one there.
				va = append(va, macroToken{macroTokPunct, ","}, macroToken{macroTokWhitespace, " "})
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
		// Produce the expansion of one body token: # param, a param, or the token.
		var produced []macroToken
		if body[i].kind == macroTokPunct && body[i].text == "#" {
			j := skipWhitespaceTokens(body, i+1)
			if j < len(body) && body[j].kind == macroTokIdent {
				if arg, ok := argMap[body[j].text]; ok {
					produced = []macroToken{{macroTokString, stringifyMacroTokens(arg)}}
					i = j + 1
				}
			}
			if produced == nil {
				// Not a stringification; keep the '#' as an ordinary token.
				produced = []macroToken{body[i]}
				i++
			}
		} else if body[i].kind == macroTokIdent {
			if arg, ok := argMap[body[i].text]; ok {
				produced = arg
				i++
			}
		}
		if produced == nil {
			produced = []macroToken{body[i]}
			i++
		}
		// token pasting: produced ## rhs, with whitespace around ## ignored.
		// Chains such as a##b##c and #x##suffix fold in one go.
		for {
			k := skipWhitespaceTokens(body, i)
			if k >= len(body) || body[k].kind != macroTokPunct || body[k].text != "##" {
				break
			}
			rhs := skipWhitespaceTokens(body, k+1)
			if rhs >= len(body) {
				break
			}
			produced = pasteMacroTokens(produced, resolveBodyToken(body[rhs], argMap))
			i = rhs + 1
		}
		out = append(out, produced...)
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

// macroParenBalance counts unmatched '(' minus ')' in text. Comments and
// string/char literals are ignored, so a parenthesis inside either does not
// change the result. The caller uses it to join a macro invocation that spans
// several source lines before expansion.
func macroParenBalance(text string) int {
	depth := 0
	for _, t := range tokenizeMacroSource(text) {
		if t.kind != macroTokPunct {
			continue
		}
		switch t.text {
		case "(":
			depth++
		case ")":
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

// sanitizeMacroBodyTokens turns comments into one space. Translation phase 3
// removes comments before macro expansion; leaving a // comment in a body would
// comment out the rest of the expansion and swallow the caller's semicolon.
func sanitizeMacroBodyTokens(tokens []macroToken) []macroToken {
	for i := range tokens {
		if tokens[i].kind == macroTokComment {
			tokens[i] = macroToken{kind: macroTokWhitespace, text: " "}
		}
	}
	return tokens
}

// isMacroWhitespaceToken reports whether t separates tokens in a macro body.
func isMacroWhitespaceToken(t macroToken) bool {
	return t.kind == macroTokWhitespace || t.kind == macroTokNewline
}

// escapeStringifyText escapes the characters that would end or alter the
// enclosing string literal produced by # (backslash and double quote).
func escapeStringifyText(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' || text[i] == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

func stringifyMacroTokens(tokens []macroToken) string {
	var b strings.Builder
	b.WriteByte('"')
	// ISO C 6.10.3.2: a run of whitespace between tokens becomes one space,
	// and leading/trailing whitespace is dropped. Keeping a raw newline here
	// produced an unterminated string literal for multi-line arguments.
	pendingSpace := false
	for _, t := range tokens {
		if isMacroWhitespaceToken(t) || t.kind == macroTokComment {
			if b.Len() > 1 {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		switch t.kind {
		case macroTokString:
			inner := t.text
			if len(inner) >= 2 && inner[0] == '"' && inner[len(inner)-1] == '"' {
				inner = inner[1 : len(inner)-1]
			}
			// ISO C 6.10.3.2: escape \ and " of a string literal inside the result.
			b.WriteString(`\"`)
			b.WriteString(escapeStringifyText(inner))
			b.WriteString(`\"`)
		case macroTokChar:
			b.WriteString(escapeStringifyText(t.text))
		default:
			b.WriteString(t.text)
		}
	}
	b.WriteByte('"')
	return b.String()
}
