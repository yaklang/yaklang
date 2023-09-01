package sca

import "github.com/yaklang/yaklang/common/sca/analyzer"

var Exports = map[string]interface{}{
	"ScanImageFromContext":     ScanDockerImageFromContext,
	"ScanContainerFromContext": ScanDockerContainerFromContext,
	"ScanImageFromFile":        ScanDockerImageFromFile,
	"ScanFilesystem":           ScanFilesystem,

	// options
	"endpoint":   _withEndPoint,
	"scanMode":   _withScanMode,
	"concurrent": _withConcurrent,
	"analyzers":  _withAnalayzers,

	// use prefix + type name as key
	// e.g. "ANALYZER_TYPE_DPKG"
	// keep friendly for completion
	"MODE_ALL":      analyzer.AllMode,
	"MODE_PKG":      analyzer.PkgMode,
	"MODE_LANGUAGE": analyzer.LanguageMode,

	"ANALYZER_TYPE_DPKG":             analyzer.TypDPKG,
	"ANALYZER_TYPE_RPM":              analyzer.TypRPM,
	"ANALYZER_TYPE_APK":              analyzer.TypAPK,
	"ANALYZER_TYPE_RUBY_BUNDLER":     analyzer.TypRubyBundler,
	"ANALYZER_TYPE_RUST_CARGO":       analyzer.TypRustCargo,
	"ANALYZER_TYPE_RUBY_GEMSPEC":     analyzer.TypRubyGemSpec,
	"ANALYZER_TYPE_PYTHON_POETRY":    analyzer.TypPythonPoetry,
	"ANALYZER_TYPE_PYTHON_PIPENV":    analyzer.TypPythonPIPEnv,
	"ANALYZER_TYPE_PYTHON_PIP":       analyzer.TypPythonPIP,
	"ANALYZER_TYPE_PYTHON_PACKAGING": analyzer.TypPythonPackaging,
	"ANALYZER_TYPE_PHP_COMPOSER":     analyzer.TypPHPComposer,
	"ANALYZER_TYPE_NODE_YARN":        analyzer.TypNodeYarn,
	"ANALYZER_TYPE_NODE_PNPM":        analyzer.TypNodePnpm,
	"ANALYZER_TYPE_NODE_NPM":         analyzer.TypNodeNpm,
	"ANALYZER_TYPE_JAVA_POM":         analyzer.TypJavaPom,
	"ANALYZER_TYPE_JAVA_GRADLE":      analyzer.TypJavaGradle,
	"ANALYZER_TYPE_JAVA_JAR":         analyzer.TypJavaJar,
	"ANALYZER_TYPE_GO_MOD":           analyzer.TypGoMod,
	"ANALYZER_TYPE_GO_BINARY":        analyzer.TypGoBinary,
	"ANALYZER_TYPE_CLANG_CONAN":      analyzer.TypClangConan,
}
