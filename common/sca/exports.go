package sca

import "github.com/yaklang/yaklang/common/sca/analyzer"

var Exports = map[string]interface{}{
	"ScanImageFromContext":     ScanDockerImageFromContext,
	"ScanContainerFromContext": ScanDockerContainerFromContext,
	"ScanImageFromFile":        ScanDockerImageFromFile,
	"endpoint":                 _withEndPoint,
	"scanMode":                 _withScanMode,
	"concurrent":               _withConcurrent,
	"analyzers":                _withAnalayzers,

	"ALL_MODE":      analyzer.AllMode,
	"PKG_MODE":      analyzer.PkgMode,
	"LANGUAGE_MODE": analyzer.LanguageMode,

	"DPKG_ALALYZER":             analyzer.TypDPKG,
	"RPM_ALALYZER":              analyzer.TypRPM,
	"APK_ALALYZER":              analyzer.TypAPK,
	"RUBY_BUNDLER_ANALYZER":     analyzer.TypRubyBundler,
	"RUST_CARGO_ANALYZER":       analyzer.TypRustCargo,
	"RUBY_GEMSPEC_ANALYZER":     analyzer.TypRubyGemSpec,
	"PYTHON_POETRY_ANALYZER":    analyzer.TypPythonPoetry,
	"PYTHON_PIPENV_ANALYZER":    analyzer.TypPythonPIPEnv,
	"PYTHON_PIP_ANALYZER":       analyzer.TypPythonPIP,
	"PYTHON_PACKAGING_ANALYZER": analyzer.TypPythonPackaging,
	"PHP_COMPOSER_ANALYZER":     analyzer.TypPHPComposer,
	"NODE_YARN_ANALYZER":        analyzer.TypNodeYarn,
	"NODE_PNPM_ANALYZER":        analyzer.TypNodePnpm,
	"NODE_NPM_ANALYZER":         analyzer.TypNodeNpm,
	"JAVA_POM_ANALYZER":         analyzer.TypJavaPom,
	"JAVA_GRADLE_ANALYZER":      analyzer.TypJavaGradle,
	"JAVA_JAR_ANALYZER":         analyzer.TypJavaJar,
	"GO_MOD_ANALYZER":           analyzer.TypGoMod,
	"GO_BINARY_ANALYZER":        analyzer.TypGoBinary,
	"CLANG_CONAN_ANALYZER":      analyzer.TypClangConan,
}
