package ssaconfig

import (
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
)

// CompileExcludeFunc matches project paths that should be skipped during SSA compile scan.
type CompileExcludeFunc func(path string) bool

// DefaultCompileExcludeDirNames are directory base names skipped during recursive scan.
// Each name is also expanded into glob patterns in DefaultCompileExcludePatterns().
//
// All languages are aggregated into one list: directory names either mark
// non-source content (version-control metadata, build outputs, dependency
// caches, test code/fixtures) or do not exist in source trees at all, so no
// per-language split is needed. Test code and fixtures are excluded whole:
// tests are not production attack surface, and bundled test archives are what
// makes decompilers hang on large repositories.
var DefaultCompileExcludeDirNames = []string{
	// version-control / IDE / agent metadata
	".claude",
	".codex",
	".cursor",
	".git",
	".gradle",
	".hg",
	".idea",
	".svn",
	".vscode",
	// JS/TS build tooling
	".next",
	".nuxt",
	"coverage",
	"node_modules",
	// Python environments (note: plain "env" is deliberately not excluded —
	// it is a common application source directory name)
	".tox",
	".venv",
	"__pycache__",
	"venv",
	// build outputs (Java/C#/Go/C-C++/JS)
	"bin",
	"build",
	"dist",
	"obj",
	"out",
	"pkg",
	"target",
	// C/C++ build machinery
	"CMakeFiles",
	// test code / fixtures (aggregated across languages)
	"TestResults",
	"__tests__",
	"fixtures",
	"test",
	"test-classes",
	"test-fixtures",
	"testdata",
	"testing",
	"tests",
	// yak tooling
	"yakit-projects",
}

// DefaultCompileExcludeGlobs are built-in glob patterns merged into every compile exclude matcher.
//
// All languages are aggregated into one list. Extension-scoped patterns only
// ever match files of that language, so cross-language sharing is safe.
// Archives (.jar/.war/...) are excluded ONLY when test-related (under a test
// directory or clearly test-named). Blanket archive exclusions such as
// **/*.jar are intentionally avoided: bundled runtime/dependency archives can
// be legitimate scan targets, so only obviously non-source build machinery
// (target/, build/, gradle wrapper, etc.) is excluded by name.
var DefaultCompileExcludeGlobs = []string{
	".github/**",
	".mvn/**",
	"docs/**",
	"eclipse/**",
	"**/Vendor/**",
	"Vendor/**",
	"**/vendor/**",
	"vendor/**",
	"**/target/**",
	"**include/**",
	"**caches/**",
	"**cache/**",
	"**tmp/**",
	"**alipay/**",
	"**includes/**",
	"**temp/**",
	"**zh_cn/**",
	"**zh_en/**",
	"**plugins/**",
	"**PHPExcel/**",
	// OS metadata (both root-level and nested)
	".DS_Store",
	"**/.DS_Store",
	"Thumbs.db",
	"**/Thumbs.db",
	// test archives and Maven integration tests (name-based, language-agnostic).
	// Patterns are written as both root and nested forms because gobwas glob
	// requires "**/" to be followed by a separator: **/x does not match "x".
	// Note: "*test*.jar" also matches "test.jar" itself, which is intended.
	"src/it/**",
	"**/src/it/**",
	"*test*.jar",
	"**/*test*.jar",
	"*Test*.jar",
	"**/*Test*.jar",
	"*test*.war",
	"**/*test*.war",
	"*Test*.war",
	"**/*Test*.war",
	"*test*.zip",
	"**/*test*.zip",
	"*Test*.zip",
	"**/*Test*.zip",
	"*test*.tar",
	"**/*test*.tar",
	"*test*.tar.gz",
	"**/*test*.tar.gz",
	// Java build machinery (no blanket *.jar/*.war/*.class)
	"gradle/wrapper/**",
	"**/gradle/wrapper/**",
	"mvnw",
	"**/mvnw",
	"mvnw.cmd",
	"**/mvnw.cmd",
	"gradlew",
	"**/gradlew",
	"gradlew.bat",
	"**/gradlew.bat",
	// Go
	"**/*_test.go",
	"go.mod",
	"**/go.mod",
	"go.sum",
	"**/go.sum",
	// JS/TS
	"**/*.min.js",
	"**/*.min.css",
	"**/*.bundle.js",
	"**/*.bundle.css",
	"package-lock.json",
	"**/package-lock.json",
	"yarn.lock",
	"**/yarn.lock",
	"pnpm-lock.yaml",
	"**/pnpm-lock.yaml",
	"**/*.spec.js",
	"**/*.spec.ts",
	"**/*.test.js",
	"**/*.test.ts",
	// Python
	"**/*.pyc",
	"**/*.pyo",
	"**/*.pyd",
	"poetry.lock",
	"**/poetry.lock",
	"Pipfile.lock",
	"**/Pipfile.lock",
	"**/requirements*.txt",
	"**/test_*.py",
	"**/*_test.py",
	"pytest.ini",
	"**/pytest.ini",
	"setup.py",
	"**/setup.py",
	"setup.cfg",
	"**/setup.cfg",
	// PHP
	"composer.lock",
	"**/composer.lock",
	"composer.phar",
	"**/composer.phar",
	"**/*Test.php",
	"**/*_test.php",
	"**/phpunit.xml*",
	// C#
	"**/*.dll",
	"**/*.exe",
	"**/*.pdb",
	"**/*.nupkg",
	"project.lock.json",
	"**/project.lock.json",
	"package.lock.json",
	"**/package.lock.json",
	"**/*.csproj.user",
	// C/C++
	"**/cmake-build-*/**",
	"**/cmake-build-*",
	"**/*.o",
	"**/*.obj",
	"**/*.a",
	"**/*.so",
	"**/*.lib",
	"CMakeCache.txt",
	"**/CMakeCache.txt",
	"Makefile",
	"**/Makefile",
	"compile_commands.json",
	"**/compile_commands.json",
	// Lua / Yak
	"**/*.luac",
	"**/*.yakc",
}

// DefaultCompileExcludePatterns returns all built-in exclude globs, including directory names.
func DefaultCompileExcludePatterns() []string {
	patterns := make([]string, 0, len(DefaultCompileExcludeGlobs)+len(DefaultCompileExcludeDirNames)*4)
	patterns = append(patterns, DefaultCompileExcludeGlobs...)
	for _, dir := range DefaultCompileExcludeDirNames {
		patterns = append(patterns, dir, dir+"/**", "**/"+dir, "**/"+dir+"/**")
	}
	return patterns
}

// ShouldSkipCompileDirName reports whether a directory base name is excluded by default.
func ShouldSkipCompileDirName(name string) bool {
	for _, dir := range DefaultCompileExcludeDirNames {
		if name == dir {
			return true
		}
	}
	return false
}

// BuildCompileExcludeFunc merges userPatterns with DefaultCompileExcludePatterns()
// unless includeDefaults is false. When includeDefaults is false, only userPatterns
// are compiled; this is the escape hatch for callers (tests, embedders) that need
// the built-in excludes to not apply at all (see WithCompileDisableDefaultExcludes).
func BuildCompileExcludeFunc(userPatterns []string, basePath string, includeDefaults bool) CompileExcludeFunc {
	var compiled []glob.Glob
	seenPatterns := make(map[string]bool)
	patterns := append([]string(nil), userPatterns...)
	if includeDefaults {
		patterns = append(patterns, DefaultCompileExcludePatterns()...)
	}
	basePath = normalizeCompileExcludePath(basePath)

	addPattern := func(pattern string) {
		pattern = normalizeCompileExcludePath(pattern)
		if pattern == "" {
			return
		}
		if seenPatterns[pattern] {
			return
		}
		seenPatterns[pattern] = true
		g, err := glob.Compile(pattern)
		if err != nil {
			log.Warnf("failed to compile exclude pattern: %v, pattern: %s", err, pattern)
			return
		}
		compiled = append(compiled, g)
	}

	normalizePattern := func(pattern string) []string {
		pattern = normalizeCompileExcludePath(pattern)
		if strings.HasSuffix(pattern, "/") {
			base := strings.TrimSuffix(pattern, "/")
			return []string{base, base + "/**"}
		}
		return []string{pattern}
	}

	for _, pattern := range patterns {
		pattern = normalizeCompileExcludePath(pattern)
		for _, p := range normalizePattern(pattern) {
			addPattern(p)
		}

		relPattern := strings.TrimPrefix(pattern, basePath)
		relPattern = strings.TrimLeft(relPattern, "/")
		if relPattern != pattern {
			for _, p := range normalizePattern(relPattern) {
				addPattern(p)
			}
		}
	}

	return func(path string) bool {
		path = normalizeCompileExcludePath(path)
		for _, g := range compiled {
			if g.Match(path) {
				return true
			}
		}
		return false
	}
}

func normalizeCompileExcludePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(path)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	return path
}

// ResolveCompileExcludeFunc returns exclude when set, otherwise the built-in default matcher.
func ResolveCompileExcludeFunc(exclude CompileExcludeFunc) CompileExcludeFunc {
	if exclude != nil {
		return exclude
	}
	return BuildCompileExcludeFunc(nil, "", true)
}
