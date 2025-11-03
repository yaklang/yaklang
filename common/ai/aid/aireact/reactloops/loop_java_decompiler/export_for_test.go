package loop_java_decompiler

// Export internal functions and variables for external tests
// This file is always compiled (unlike export_test.go which is only compiled during tests in the same package)

var (
	// Export action functions for testing
	DecompileJarAction    = decompileJarAction
	RewriteJavaFileAction = rewriteJavaFileAction

	// Export helper functions for testing
	GenerateDecompilationReport = generateDecompilationReport
	CheckJavaFileSyntax         = checkJavaFileSyntax
	TrySSACompilation           = trySSACompilation
	TryJavacCompilation         = tryJavacCompilation
	CheckBasicJavaSyntax        = checkBasicJavaSyntax
)
