// Package javasyntax holds the canonical preprocessing + syntax-validation logic for
// decompiled Java source. It is a dependency-light leaf package (only the generated ANTLR
// Java parser and the antlr4util helpers) so it can be imported by BOTH:
//
//   - common/yak/java/java2ssa (the SSA frontend, which normalizes decompiled Java before
//     building SSA), and
//   - common/javaclassparser (the decompiler, which validates its own output and degrades
//     un-parseable method bodies to stubs).
//
// Keeping a single source of truth here is what lets the decompiler guarantee "no syntax
// errors": it validates with exactly the same normalization + grammar the frontend/jdsc use,
// so a method that would be rejected downstream is detected and stubbed at decompile time.
package javasyntax

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

// parseMu serializes ANTLR parses. NewJavaParser/NewJavaLexer share package-level
// static ATN/DFA/PredictionContextCache that are not safe for concurrent use; parallel
// jar decompilation + syntax validation otherwise triggers concurrent map read/write.
var parseMu sync.Mutex

// ---- normalization of known decompiler-output quirks -------------------------------------
//
// These regexes paper over byte-for-byte imperfect (but semantically recoverable) decompiler
// output so the grammar accepts it. They must stay in sync with what the decompiler can emit.

var decompiledEmptyCallablePlaceholder = regexp.MustCompile(`\(\s*\(\s*\)\s*\)`)
var decompiledAnonymousClassMissingCommaNull = regexp.MustCompile(`}\s*null\)`)
var decompiledAnonymousClassMissingCommaCast = regexp.MustCompile(`}\s*\(([A-Za-z_$][A-Za-z0-9_$.<>]*)\)(\s*[A-Za-z_$])`)
var decompiledAnonymousClassMissingCommaThis = regexp.MustCompile(`}[ \t]*(this\b)`)
var decompiledAnonymousClassMissingCommaNew = regexp.MustCompile(`}[ \t]*(new\s+[A-Za-z_$])`)
var decompiledMergeLambdaMissingComma = regexp.MustCompile(`}[ \t]*\(([A-Za-z_$][A-Za-z0-9_$]*\s*,\s*[A-Za-z_$][A-Za-z0-9_$]*)\)[ \t]*->`)
var decompiledBareCallablePlaceholder = regexp.MustCompile(`,\s*\(\s*\)\s*([,)])`)
var decompiledNullIdentifierAssign = regexp.MustCompile(`\bnull\b(\s*=)`)
var decompiledNullIdentifierDot = regexp.MustCompile(`\bnull\b(\s*\.)`)
var decompiledNullIdentifierPlus = regexp.MustCompile(`\bnull\b(\s*\+)`)
var decompiledSyntheticOuterThisAssign = regexp.MustCompile(`(?m)^([ \t]*)[A-Za-z_$][A-Za-z0-9_$.<>]*\.this\s*=\s*[A-Za-z_$][A-Za-z0-9_$.<>]*\.this;`)
var decompiledDuplicateAssignmentTemps = regexp.MustCompile(`(?m)^([ \t]*)[A-Za-z_$][A-Za-z0-9_$.<>\[\]]*\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([A-Za-z_$][A-Za-z0-9_$.]*)\s*,\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*([A-Za-z_$][A-Za-z0-9_$.]*)\s*=\s*(.+);\s*$`)

var javaRecordPatternCaseRegexp = regexp.MustCompile(`(?m)(case\s+[A-Za-z_][\w$.<>]*\s*)\(\s*var\s+([A-Za-z_]\w*)(?:\s*,[\s\S]*?)?\)\s*(->)`)

// NormalizeDecompiledJava rewrites recoverable decompiler-output quirks into grammar-legal forms.
func NormalizeDecompiledJava(src string) string {
	src = decompiledEmptyCallablePlaceholder.ReplaceAllString(src, "(__yak_decompiled_placeholder__)")
	src = decompiledAnonymousClassMissingCommaNull.ReplaceAllString(src, "}, null)")
	src = decompiledAnonymousClassMissingCommaCast.ReplaceAllString(src, "}, ($1)$2")
	src = decompiledAnonymousClassMissingCommaThis.ReplaceAllString(src, "}, $1")
	src = decompiledAnonymousClassMissingCommaNew.ReplaceAllString(src, "}, $1")
	src = decompiledMergeLambdaMissingComma.ReplaceAllString(src, "}, ($1) ->")
	src = decompiledBareCallablePlaceholder.ReplaceAllString(src, ", __yak_decompiled_placeholder__$1")
	src = decompiledNullIdentifierAssign.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledNullIdentifierDot.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledNullIdentifierPlus.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledSyntheticOuterThisAssign.ReplaceAllString(src, "${1}this.this$$0 = this$$0;")
	src = decompiledDuplicateAssignmentTemps.ReplaceAllStringFunc(src, func(match string) string {
		parts := decompiledDuplicateAssignmentTemps.FindStringSubmatch(match)
		if len(parts) != 7 {
			return match
		}
		if parts[3] != parts[5] {
			return match
		}
		return parts[1] + parts[3] + " = " + parts[6] + ";"
	})
	return src
}

// PreprocessJavaRecordPatternSwitchCases relaxes `case Type(var x) ->` record-pattern cases
// into a form the current grammar accepts.
func PreprocessJavaRecordPatternSwitchCases(src string) string {
	if !strings.Contains(src, "case ") || !strings.Contains(src, "->") || !strings.Contains(src, "(var ") {
		return src
	}
	return javaRecordPatternCaseRegexp.ReplaceAllString(src, `$1$2 $3`)
}

// PreprocessJavaUnicodeEscapes expands `\uXXXX` escapes outside of an even backslash run so the
// lexer sees the decoded rune. Mirrors javac's unicode-escape handling.
func PreprocessJavaUnicodeEscapes(src string) string {
	if !strings.Contains(src, `\u`) {
		return src
	}

	var out strings.Builder
	out.Grow(len(src))
	backslashCount := 0

	for i := 0; i < len(src); i++ {
		if src[i] != '\\' {
			out.WriteByte(src[i])
			backslashCount = 0
			continue
		}

		if i+1 >= len(src) || src[i+1] != 'u' || backslashCount%2 != 0 {
			out.WriteByte(src[i])
			backslashCount++
			continue
		}

		j := i + 1
		for j < len(src) && src[j] == 'u' {
			j++
		}
		if j+4 > len(src) {
			out.WriteByte(src[i])
			continue
		}

		hex := src[j : j+4]
		code, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			out.WriteByte(src[i])
			backslashCount++
			continue
		}

		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], rune(code))
		out.Write(buf[:n])
		if rune(code) == '\\' {
			backslashCount = 1
		} else {
			backslashCount = 0
		}
		i = j + 3
	}

	return out.String()
}

// Preprocess applies every decompiled-Java normalization step, in the canonical order used by
// the SSA frontend, and returns grammar-ready source.
func Preprocess(src string) string {
	src = PreprocessJavaUnicodeEscapes(src)
	src = PreprocessJavaRecordPatternSwitchCases(src)
	src = NormalizeDecompiledJava(src)
	return src
}

// Parse preprocesses and parses Java source via the SLL-first ANTLR pipeline, returning the
// compilation-unit AST and any syntax error. It is the cache-less twin of java2ssa.Frontend.
func Parse(src string) (javaparser.ICompilationUnitContext, error) {
	src = Preprocess(src)
	parseMu.Lock()
	defer parseMu.Unlock()
	return antlr4util.ParseASTWithSLLFirst(
		src,
		javaparser.NewJavaLexer,
		javaparser.NewJavaParser,
		nil,
		nil,
		func(parser *javaparser.JavaParser) javaparser.ICompilationUnitContext {
			return parser.CompilationUnit()
		},
	)
}

// Validate reports whether src is syntactically valid Java (after decompiler normalization).
// A nil return means the grammar accepts it; a non-nil error carries the first syntax problem.
func Validate(src string) error {
	_, err := Parse(src)
	return err
}
