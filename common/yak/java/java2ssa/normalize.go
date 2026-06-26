package java2ssa

import "github.com/yaklang/yaklang/common/yak/java/javasyntax"

// The decompiled-Java normalization and unicode/record preprocessing now live in the
// dependency-light leaf package javasyntax, so the decompiler (common/javaclassparser) can
// validate its own output with the exact same rules the SSA frontend applies here. These
// thin wrappers preserve the existing call sites in builder.go unchanged.

func normalizeDecompiledJava(src string) string {
	return javasyntax.NormalizeDecompiledJava(src)
}

func preprocessJavaRecordPatternSwitchCases(src string) string {
	return javasyntax.PreprocessJavaRecordPatternSwitchCases(src)
}

func preprocessJavaUnicodeEscapes(src string) string {
	return javasyntax.PreprocessJavaUnicodeEscapes(src)
}
