package javaclassparser

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// Enum constant-body CROSS-CLASS folding.
//
// A constant-specific class body, e.g.
//
//	enum Op { ADD { long apply(long a,long b){return a+b;} }, MUL {...}; abstract long apply(...); }
//
// is compiled by javac into the enum `Op` plus one synthetic subclass per body (`Op$1`, `Op$2`, each
// ACC_ENUM and `extends Op`). Decompiled per-class this yields UNCOMPILABLE output two ways: `Op`'s
// constants do not override the abstract method, and `Op$N` renders `class Op$N extends Op` (an enum
// is not extensible). The only legal Java is to fold each `Op$N`'s members back into its constant:
// `ADD { ...members... }`. That requires the sibling `Op$N` bytes, supplied via the dumper's
// foldSiblingResolver (nil on the single-class path, so this whole feature is a no-op there).

// classObjectHasEnumFlag reports whether the class carries the ACC_ENUM access flag.
func classObjectHasEnumFlag(cf *ClassObject) bool {
	for _, k := range cf.AccessFlagsVerbose {
		if k == "enum" {
			return true
		}
	}
	return false
}

// isGenuineEnum reports whether cf is a real `enum` declaration: ACC_ENUM and extends java.lang.Enum.
// (A synthetic per-constant body subclass `Outer$N` also carries ACC_ENUM but extends the enum itself,
// not java.lang.Enum, and is excluded here.)
func isGenuineEnum(cf *ClassObject) bool {
	return classObjectHasEnumFlag(cf) && cf.GetSupperClassName() == "java/lang/Enum"
}

// isSyntheticEnumConstantSubclass reports whether cf is a javac-synthesized per-constant body subclass
// (`Outer$N`): it carries ACC_ENUM yet extends the enum type rather than java.lang.Enum. Such a class
// can ONLY arise from an enum constant body (you cannot subclass an enum in source), so folding it into
// the enum and suppressing its standalone output is always correct. Anonymous classes inside enum
// methods extend Object/an interface and lack ACC_ENUM; the enum-switch map class `Outer$1` likewise
// extends Object -- neither is matched here.
func isSyntheticEnumConstantSubclass(cf *ClassObject) bool {
	if !classObjectHasEnumFlag(cf) {
		return false
	}
	super := cf.GetSupperClassName()
	return super != "" && super != "java/lang/Enum"
}

// foldEnumConstantBodies returns, for each enum constant whose <clinit> initializer is
// `new Outer$N(name, ordinal, ...)`, the rendered constant body block (" {\n ...members... \n\t}")
// to splice after the constant. Imports required by the folded members are merged into c.FuncCtx so
// the enclosing unit's import block (assembled after this) carries them. Returns nil (no folding) on
// the single-class path, for non-enums, or when JDEC_NO_ENUM_FOLD is set (load-bearing kill-switch).
func (c *ClassObjectDumper) foldEnumConstantBodies(isEnum bool) map[string]string {
	if !isEnum || c.foldSiblingResolver == nil || os.Getenv("JDEC_NO_ENUM_FOLD") != "" {
		return nil
	}
	debug := os.Getenv("JDEC_FOLD_DEBUG") != ""
	enumSimple := c.GetConstructorMethodName()
	if enumSimple == "" {
		return nil
	}
	pkgPath := strings.ReplaceAll(c.PackageName, ".", "/")
	out := map[string]string{}
	for name, raw := range c.fieldDefaultValue {
		sub := enumSubclassSimpleFromNew(strings.TrimSpace(raw), enumSimple)
		if sub == "" {
			continue
		}
		internal := sub
		if pkgPath != "" {
			internal = pkgPath + "/" + sub
		}
		data, ok := c.foldSiblingResolver(internal)
		if !ok || len(data) == 0 {
			if debug {
				log.Infof("enum fold: constant %s -> %s not resolved", name, internal)
			}
			continue
		}
		body := c.renderFoldedConstantBody(data, sub)
		if body == "" {
			if debug {
				log.Infof("enum fold: constant %s -> %s produced empty body", name, internal)
			}
			continue
		}
		if debug {
			log.Infof("enum fold: constant %s folded from %s", name, internal)
		}
		out[name] = body
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// enumSubclassSimpleFromNew extracts the synthetic subclass simple name from a constant's <clinit>
// initializer `new Outer$N(...)`. It returns "" for a body-less constant (`new Outer(...)`, whose
// type is the enum itself) or any non-constructor RHS. The match requires the rendered type's simple
// name to start with `<enumSimple>$`, which is exactly the shape javac emits for a constant body.
func enumSubclassSimpleFromNew(raw, enumSimple string) string {
	if !strings.HasPrefix(raw, "new ") {
		return ""
	}
	rest := strings.TrimSpace(raw[len("new "):])
	open := strings.IndexByte(rest, '(')
	if open < 0 {
		return ""
	}
	typ := strings.TrimSpace(rest[:open])
	if lt := strings.IndexByte(typ, '<'); lt >= 0 {
		typ = strings.TrimSpace(typ[:lt])
	}
	if dot := strings.LastIndexByte(typ, '.'); dot >= 0 {
		typ = typ[dot+1:]
	}
	if !strings.HasPrefix(typ, enumSimple+"$") {
		return ""
	}
	return typ
}

// renderFoldedConstantBody decompiles the synthetic subclass bytes, strips its (synthetic)
// constructor, re-indents the remaining members one level deeper, merges its imports into the
// enclosing unit, and returns the " {\n ...members... \n\t}" block. Returns "" if the subclass has
// no foldable members or could not be decompiled. The sub-dumper has no resolver, so folding never
// recurses.
func (c *ClassObjectDumper) renderFoldedConstantBody(data []byte, subSimple string) (result string) {
	defer func() {
		if e := recover(); e != nil {
			log.Warnf("enum fold: render of %s panicked: %v", subSimple, utils.ErrorStack(e))
			result = ""
		}
	}()
	subObj, err := Parse(data)
	if err != nil {
		return ""
	}
	src, err := subObj.Dump()
	if err != nil || src == "" {
		return ""
	}
	if c.FuncCtx != nil {
		for _, imp := range javaExtractImports(src) {
			c.FuncCtx.Import(imp)
		}
	}
	body := javaClassBodyContent(src)
	if body == "" {
		return ""
	}
	body = javaRemoveConstructors(body, subSimple)
	body = strings.Trim(body, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}
	// Re-indent every non-empty member line one tab deeper: the standalone subclass renders members
	// at one tab; inside an enum constant they sit one level further in.
	var b strings.Builder
	b.WriteString(" {\n")
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("\t")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\t}")
	return b.String()
}

// javaExtractImports returns the import targets ("java.util.List") from top-level `import X;` lines.
func javaExtractImports(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "import ") && strings.HasSuffix(t, ";") {
			out = append(out, strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "import "), ";")))
		}
	}
	return out
}

// javaClassBodyContent returns the text between the first top-level `{` and its matching `}` (the
// class body), scanning comment/quote aware so braces inside strings, char literals, and comments
// are ignored.
func javaClassBodyContent(src string) string {
	open := javaIndexTopBrace(src)
	if open < 0 {
		return ""
	}
	close := javaMatchBrace(src, open)
	if close < 0 {
		return ""
	}
	return src[open+1 : close]
}

// javaRemoveConstructors removes every member declaration whose name is the (degraded) subclass
// simple name, i.e. a constructor `... subSimple(params) { ... }`, including any leading modifiers on
// its line. Scanning is comment/quote aware and brace-balanced.
func javaRemoveConstructors(body, subSimple string) string {
	for {
		idx := javaFindConstructorParen(body, subSimple)
		if idx < 0 {
			return body
		}
		open := javaIndexBraceFrom(body, idx)
		if open < 0 {
			return body
		}
		close := javaMatchBrace(body, open)
		if close < 0 {
			return body
		}
		start := strings.LastIndexByte(body[:idx], '\n') + 1 // start of the constructor's line
		end := close + 1
		if end < len(body) && body[end] == '\n' {
			end++
		}
		body = body[:start] + body[end:]
	}
}

// javaFindConstructorParen returns the index of the `(` of a depth-0, normal-state occurrence of
// `subSimple(` (a constructor signature), or -1.
func javaFindConstructorParen(body, subSimple string) int {
	depth := 0
	st := scanNormal
	for i := 0; i < len(body); i++ {
		st = scanAdvance(body, &i, st, &depth)
		if st != scanNormal || i >= len(body) {
			continue
		}
		switch body[i] {
		case '{':
			depth++
			continue
		case '}':
			depth--
			continue
		case '(':
			if depth != 0 {
				continue
			}
			// look back over spaces for the subSimple identifier
			j := i - 1
			for j >= 0 && (body[j] == ' ' || body[j] == '\t') {
				j--
			}
			endName := j + 1
			startName := endName - len(subSimple)
			if startName >= 0 && body[startName:endName] == subSimple {
				before := startName - 1
				if before < 0 || !isJavaIdentChar(body[before]) {
					return i
				}
			}
		}
	}
	return -1
}

func isJavaIdentChar(b byte) bool {
	return b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// javaIndexTopBrace returns the index of the first `{` encountered in normal (non-comment,
// non-string) state, or -1.
func javaIndexTopBrace(src string) int {
	st := scanNormal
	depth := 0
	for i := 0; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st == scanNormal && i < len(src) && src[i] == '{' {
			return i
		}
	}
	return -1
}

// javaIndexBraceFrom returns the index of the next `{` at/after from in normal state, or -1.
func javaIndexBraceFrom(src string, from int) int {
	st := scanNormal
	depth := 0
	for i := from; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st == scanNormal && i < len(src) && src[i] == '{' {
			return i
		}
	}
	return -1
}

// javaMatchBrace returns the index of the `}` matching the `{` at openIdx, comment/quote aware.
func javaMatchBrace(src string, openIdx int) int {
	depth := 0
	st := scanNormal
	for i := openIdx; i < len(src); i++ {
		st = scanAdvance(src, &i, st, &depth)
		if st != scanNormal || i >= len(src) {
			continue
		}
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

type scanState int

const (
	scanNormal scanState = iota
	scanLineComment
	scanBlockComment
	scanString
	scanChar
)

// scanAdvance consumes lexical state (comments/strings/char literals) starting at index *i, leaving
// *i positioned on a normal-state significant char (or past it for skipped regions) and returning
// the state to use for the next iteration. It only mutates depth for `{`/`}` is left to the caller;
// depth is passed through for signature symmetry. The caller increments i in its for-loop.
func scanAdvance(src string, i *int, st scanState, depth *int) scanState {
	idx := *i
	switch st {
	case scanLineComment:
		if src[idx] == '\n' {
			return scanNormal
		}
		return scanLineComment
	case scanBlockComment:
		if src[idx] == '*' && idx+1 < len(src) && src[idx+1] == '/' {
			*i = idx + 1
			return scanNormal
		}
		return scanBlockComment
	case scanString:
		if src[idx] == '\\' {
			*i = idx + 1
			return scanString
		}
		if src[idx] == '"' {
			return scanNormal
		}
		return scanString
	case scanChar:
		if src[idx] == '\\' {
			*i = idx + 1
			return scanChar
		}
		if src[idx] == '\'' {
			return scanNormal
		}
		return scanChar
	default:
		switch src[idx] {
		case '/':
			if idx+1 < len(src) && src[idx+1] == '/' {
				*i = idx + 1
				return scanLineComment
			}
			if idx+1 < len(src) && src[idx+1] == '*' {
				*i = idx + 1
				return scanBlockComment
			}
		case '"':
			return scanString
		case '\'':
			return scanChar
		}
		return scanNormal
	}
}
