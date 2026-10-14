package java2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var javaPreHandlerExtensions = map[string]struct{}{
	".java": {}, ".properties": {}, ".yaml": {}, ".yml": {}, ".json": {},
	".xml": {}, ".jsp": {}, ".jspx": {}, ".ftl": {}, ".ftlh": {}, ".ftlx": {},
	".vm": {}, ".html": {}, ".htm": {},
}

// MatchPreHandlerFile reports whether a path should enter Java pre-handler
// scanning. Directory excludes such as .github/ and .mvn/ belong in compile
// exclude patterns; this function handles Java-specific include rules only.
func MatchPreHandlerFile(path string) bool {
	path = normalizePreHandlerPath(path)
	if path == "" {
		return false
	}
	lowerPath := strings.ToLower(path)
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".class" {
		return !strings.Contains(path, "/")
	}
	if _, ok := javaPreHandlerExtensions[extension]; ok {
		return true
	}
	return strings.EqualFold(path, "pom.xml") || strings.HasSuffix(lowerPath, "/pom.xml")
}

func normalizePreHandlerPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	for strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	return strings.Trim(path, "/")
}

// preHandlerCompileExclude is built on first use, not in package init.
// java2ssa is linked into the AOT runtime's ssafront group, and building the
// globs calls ssaconfig, which lives in the separately pruned ssa group. An
// init-time call would run in every binary that keeps ssafront and panic when
// ssa itself was pruned. Compiling the globs per file would also recompile
// every pattern on each FilterPreHandlerFile call.
//
// Known limitation: this secondary filter always applies the built-in default
// excludes and is not affected by WithDisableDefaultCompileExcludes — the escape
// hatch only controls the ScanProjectFiles matcher.
var preHandlerCompileExclude ssaconfig.CompileExcludeFunc

func preHandlerExcluded(path string) bool {
	if preHandlerCompileExclude == nil {
		preHandlerCompileExclude = ssaconfig.BuildCompileExcludeFunc(nil, "", true)
	}
	return preHandlerCompileExclude(path)
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	if preHandlerExcluded(path) {
		return false
	}
	return MatchPreHandlerFile(path)
}
