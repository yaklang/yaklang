package template2java

import (
	"path/filepath"
	"regexp"
	"strings"
)

func validatePackagePath(path string) string {
	slash := filepath.ToSlash(path)
	pkgPath := regexp.MustCompile(`[^a-zA-Z0-9_/.]`).ReplaceAllString(slash, "_")
	pkgPath = strings.Replace(pkgPath, "/", ".", -1)
	pkgPath = strings.Trim(pkgPath, ".")
	return "tmp2java_" + pkgPath
}

func validateClassName(fileName string) string {
	isValidClassName := func(s string) bool {
		if s == "" {
			return false
		}
		return regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_]*$`).MatchString(s)
	}
	name := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(fileName, "_")
	name = strings.TrimLeft(name, "_")
	if !isValidClassName(name) {
		name = "_" + name
	}
	return name
}
