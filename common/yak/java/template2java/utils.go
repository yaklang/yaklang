package template2java

import (
	"path/filepath"
	"regexp"
	"strings"
)

func validatePackagePath(path string) string {
	slash := filepath.ToSlash(path)
	parts := strings.FieldsFunc(slash, func(r rune) bool {
		return r == '/' || r == '.'
	})
	sanitized := make([]string, 0, len(parts)+1)
	sanitized = append(sanitized, "tmp2java")
	for _, part := range parts {
		if part == "" {
			continue
		}
		part = regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(part, "_")
		part = strings.TrimLeft(part, "_")
		if part == "" {
			continue
		}
		if regexp.MustCompile(`^[0-9]`).MatchString(part) {
			part = "_" + part
		}
		sanitized = append(sanitized, part)
	}
	return strings.Join(sanitized, ".")
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
