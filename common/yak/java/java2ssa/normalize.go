package java2ssa

import "regexp"

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

func normalizeDecompiledJava(src string) string {
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
