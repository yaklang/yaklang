package java2ssa

import "regexp"

var decompiledEmptyCallablePlaceholder = regexp.MustCompile(`\(\s*\(\s*\)\s*\)`)
var decompiledAnonymousClassMissingCommaNull = regexp.MustCompile(`}\s*null\)`)
var decompiledAnonymousClassMissingCommaCast = regexp.MustCompile(`}\s*\(([A-Za-z_$][A-Za-z0-9_$.<>]*)\)(\s*[A-Za-z_$])`)
var decompiledNullIdentifierAssign = regexp.MustCompile(`\bnull\b(\s*=)`)
var decompiledNullIdentifierDot = regexp.MustCompile(`\bnull\b(\s*\.)`)
var decompiledNullIdentifierPlus = regexp.MustCompile(`\bnull\b(\s*\+)`)
var decompiledSyntheticOuterThisAssign = regexp.MustCompile(`(?m)^([ \t]*)[A-Za-z_$][A-Za-z0-9_$.<>]*\.this\s*=\s*[A-Za-z_$][A-Za-z0-9_$.<>]*\.this;`)

func normalizeDecompiledJava(src string) string {
	src = decompiledEmptyCallablePlaceholder.ReplaceAllString(src, "(__yak_decompiled_placeholder__)")
	src = decompiledAnonymousClassMissingCommaNull.ReplaceAllString(src, "}, null)")
	src = decompiledAnonymousClassMissingCommaCast.ReplaceAllString(src, "}, ($1)$2")
	src = decompiledNullIdentifierAssign.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledNullIdentifierDot.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledNullIdentifierPlus.ReplaceAllString(src, "__yak_decompiled_null__$1")
	src = decompiledSyntheticOuterThisAssign.ReplaceAllString(src, "${1}this.this$$0 = this$$0;")
	return src
}
