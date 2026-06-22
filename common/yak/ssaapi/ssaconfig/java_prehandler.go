package ssaconfig

import (
	"path/filepath"
	"strings"
)

var javaPreHandlerExtensions = map[string]struct{}{
	".java": {}, ".properties": {}, ".yaml": {}, ".yml": {}, ".json": {},
	".xml": {}, ".jsp": {}, ".jspx": {}, ".ftl": {}, ".ftlh": {}, ".ftlx": {},
	".vm": {}, ".html": {}, ".htm": {},
}

// MatchJavaPreHandlerFile reports whether a path should enter Java pre-handler
// scanning. Directory excludes such as .github/ and .mvn/ belong in compile
// exclude patterns; this function handles Java-specific include rules only.
func MatchJavaPreHandlerFile(path string) bool {
	path = normalizeCompileExcludePath(path)
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
