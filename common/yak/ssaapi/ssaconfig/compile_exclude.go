package ssaconfig

import (
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
)

// CompileExcludeFunc matches project paths that should be skipped during SSA compile scan.
type CompileExcludeFunc func(path string) bool

// DefaultCompileExcludeDirNames are directory base names skipped during recursive scan.
// Each name is also expanded into glob patterns in DefaultCompileExcludePatterns().
var DefaultCompileExcludeDirNames = []string{
	".git",
	".idea",
	".svn",
	".vscode",
	"test",
	"testdata",
}

// DefaultCompileExcludeGlobs are built-in glob patterns merged into every compile exclude matcher.
var DefaultCompileExcludeGlobs = []string{
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
}

// DefaultCompileExcludePatterns returns all built-in exclude globs, including directory names.
func DefaultCompileExcludePatterns() []string {
	patterns := make([]string, 0, len(DefaultCompileExcludeGlobs)+len(DefaultCompileExcludeDirNames)*2)
	patterns = append(patterns, DefaultCompileExcludeGlobs...)
	for _, dir := range DefaultCompileExcludeDirNames {
		patterns = append(patterns, "**/"+dir, "**/"+dir+"/**")
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

// BuildCompileExcludeFunc merges userPatterns with DefaultCompileExcludePatterns().
func BuildCompileExcludeFunc(userPatterns []string, basePath string) CompileExcludeFunc {
	var compiled []glob.Glob
	seenPatterns := make(map[string]bool)
	patterns := append(append([]string(nil), userPatterns...), DefaultCompileExcludePatterns()...)

	addPattern := func(pattern string) {
		pattern = strings.TrimSpace(pattern)
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
		if strings.HasSuffix(pattern, "/") {
			base := strings.TrimSuffix(pattern, "/")
			return []string{base, base + "/**"}
		}
		return []string{pattern}
	}

	for _, pattern := range patterns {
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
		for _, g := range compiled {
			if g.Match(path) {
				return true
			}
		}
		return false
	}
}

// ResolveCompileExcludeFunc returns exclude when set, otherwise the built-in default matcher.
func ResolveCompileExcludeFunc(exclude CompileExcludeFunc) CompileExcludeFunc {
	if exclude != nil {
		return exclude
	}
	return BuildCompileExcludeFunc(nil, "")
}
