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
	// documentation trees: prose and generated API pages, never runnable code
	"doc",
	"docs",
	"documentation",
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
	// test code and fixtures (aggregated across languages). Directory names are
	// matched case-insensitively, so Test/, Tests/ and TEST/ prune too: the
	// camel-case spelling is the common C#/.NET shape, and a test tree that
	// slips into the pool only produces false positives (fixture credentials,
	// sample payloads).
	"TestResults",
	"__mocks__",
	"__tests__",
	"e2e",
	"fixtures",
	"integration-test",
	"integration-tests",
	"mocks",
	"spec",
	"specs",
	"test",
	"test-cases",
	"test-classes",
	"test-fixtures",
	"test-utils",
	"testcase",
	"testcases",
	"testdata",
	"testing",
	"tests",
	"testutil",
	"testutils",
	"unittest",
	"unittests",
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
	// Non-code assets (all languages): media, office documents, images, fonts
	// and source maps never carry runnable code, so scanning them only adds
	// decode work and encoding noise. Decompilable JVM archives (jar/war/ear)
	// stay out of this list on purpose; they are scan targets.
	"**/*.mp4",
	"**/*.m4v",
	"**/*.mkv",
	"**/*.mov",
	"**/*.avi",
	"**/*.wmv",
	"**/*.flv",
	"**/*.webm",
	"**/*.mp3",
	"**/*.wav",
	"**/*.flac",
	"**/*.aac",
	"**/*.ogg",
	"**/*.m4a",
	"**/*.wma",
	"**/*.doc",
	"**/*.docx",
	"**/*.xls",
	"**/*.xlsx",
	"**/*.ppt",
	"**/*.pptx",
	"**/*.pdf",
	"**/*.odt",
	"**/*.ods",
	"**/*.odp",
	"**/*.rtf",
	"**/*.png",
	"**/*.jpg",
	"**/*.jpeg",
	"**/*.gif",
	"**/*.bmp",
	"**/*.ico",
	"**/*.webp",
	"**/*.tif",
	"**/*.tiff",
	"**/*.psd",
	"**/*.ttf",
	"**/*.otf",
	"**/*.woff",
	"**/*.woff2",
	"**/*.eot",
	"**/*.map",
	"**/*.zip",
	"**/*.tar",
	"**/*.tgz",
	"**/*.gz",
	"**/*.bz2",
	"**/*.xz",
	"**/*.7z",
	"**/*.rar",
	// OS metadata (both root-level and nested)
	".DS_Store",
	"**/.DS_Store",
	"Thumbs.db",
	"**/Thumbs.db",
	// Documentation entry files are excluded by name in
	// IsDocumentationFilePath (README, CHANGELOG, LICENSE, ...), applied by
	// BuildCompileExcludeFunc together with these globs. Only those names are
	// dropped: SKILL.md and notes.md must still reach the rules that read them.
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
	// JS/TS. Stylesheets are dropped whole (minified and bundled ones included):
	// no builtin rule reads a .css file. Scripts keep every extension a rule
	// filters on (.js, .jsx, .mjs, .cjs, .ts, .tsx); only minified/bundled
	// output is excluded by name.
	"**/*.min.js",
	"**/*.bundle.js",
	"**/*.css",
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
//
// Every "**/x" glob is also emitted in its root form "x": gobwas glob requires
// "**/" to be followed by a separator, so "**/x" never matches a root-level
// "x" — without the root form, a root-level *_test.go, *.min.js, ... would
// slip past the excludes. Explicitly listed root forms and the directory-name
// expansion below are deduplicated.
func DefaultCompileExcludePatterns() []string {
	patterns := make([]string, 0, len(DefaultCompileExcludeGlobs)*2+len(DefaultCompileExcludeDirNames)*4)
	seen := make(map[string]bool, cap(patterns))
	add := func(pattern string) {
		if pattern == "" || seen[pattern] {
			return
		}
		seen[pattern] = true
		patterns = append(patterns, pattern)
	}
	for _, pattern := range DefaultCompileExcludeGlobs {
		add(pattern)
		if root, ok := strings.CutPrefix(pattern, "**/"); ok {
			add(root)
		}
	}
	for _, dir := range DefaultCompileExcludeDirNames {
		add(dir)
		add(dir + "/**")
		add("**/" + dir)
		add("**/" + dir + "/**")
	}
	return patterns
}

// IsDefaultCompileExcludeDirName reports whether a directory base name is in the
// built-in default exclude list (DefaultCompileExcludeDirNames). User-configured
// patterns are not consulted here.
func IsDefaultCompileExcludeDirName(name string) bool {
	for _, dir := range DefaultCompileExcludeDirNames {
		if name == dir {
			return true
		}
	}
	return false
}

// documentationFileBaseNames are documentation entry points: prose files that
// describe a project instead of running in it. They are dropped from every
// project walk by IsDocumentationFilePath.
var documentationFileBaseNames = []string{
	"readme",
	"changelog",
	"changes",
	"history",
	"news",
	"contributing",
	"code_of_conduct",
	"security",
	"support",
	"authors",
	"contributors",
	"notice",
	"license",
	"licence",
	"copying",
	"copyright",
	"install",
	"upgrading",
	"maintainers",
}

// documentationFileExtensions are the suffixes a documentation entry point may
// carry. Only prose formats are listed, so a source file that happens to share
// a word with a document name (security.go, license.java) stays in the pool.
var documentationFileExtensions = map[string]bool{
	"":          true,
	".md":       true,
	".markdown": true,
	".mdx":      true,
	".rst":      true,
	".txt":      true,
	".adoc":     true,
}

// IsDocumentationFilePath reports whether path is a documentation entry point
// such as README.md, CHANGELOG-1.2.txt or CONTRIBUTING.rst.
//
// These files describe code, they never run in it, yet a README snippet like
// `password = "..."` matches a hardcoded-credential rule and license or
// contributor lists match config-shape patterns. Matching is by base name, so
// documentation trees are covered too (docs/README.md). Only the listed names
// with a prose suffix are dropped: SKILL.md, notes.md and every file inside a
// non-documentation directory still reach the rules that read them.
func IsDocumentationFilePath(path string) bool {
	path = normalizeCompileExcludePath(path)
	if path == "" {
		return false
	}
	base := filepath.Base(path)
	if base == "" || base == "." || base == "/" {
		return false
	}
	lower := strings.ToLower(base)
	for _, name := range documentationFileBaseNames {
		if !strings.HasPrefix(lower, name) {
			continue
		}
		rest := lower[len(name):]
		if rest == "" {
			return true
		}
		switch rest[0] {
		case '.', '-', '_':
			return documentationFileExtensions[filepath.Ext(lower)]
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

	// Built-in DIRECTORY names are matched case-insensitively: a test tree
	// spelled Tests/ or Test/ (the C#/.NET and some Java layouts) must prune
	// exactly like test/. Only directory segments take part — lowering the
	// file globs as well was wrong, because "**/*Test.php" then matched
	// "test.php". User patterns stay case-sensitive.
	defaultDirNames := make(map[string]bool, len(DefaultCompileExcludeDirNames))
	if includeDefaults {
		for _, name := range DefaultCompileExcludeDirNames {
			lower := strings.ToLower(strings.TrimSpace(name))
			if lower == "" {
				continue
			}
			defaultDirNames[lower] = true
		}
	}

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
		if includeDefaults && IsDocumentationFilePath(path) {
			return true
		}
		path = normalizeCompileExcludePath(path)
		if hasExcludedDirSegment(path, defaultDirNames) {
			return true
		}
		for _, g := range compiled {
			if g.Match(path) {
				return true
			}
		}
		return false
	}
}

// hasExcludedDirSegment reports whether any directory segment of path is one
// of the built-in excluded directory names, compared case-insensitively.
func hasExcludedDirSegment(path string, names map[string]bool) bool {
	if len(names) == 0 || path == "" {
		return false
	}
	for {
		slash := strings.IndexByte(path, '/')
		if slash < 0 {
			// The last segment is a file name for a file match and a directory
			// name for the directory walk's SkipDir check; the directory walk
			// also matches it through the compiled globs, so a trailing segment
			// is checked here too.
			return names[strings.ToLower(path)]
		}
		if names[strings.ToLower(path[:slash])] {
			return true
		}
		path = path[slash+1:]
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
