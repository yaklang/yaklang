package template2java

import (
	"path/filepath"
	"regexp"
	"strings"
)

// javaReservedWords are the keywords that cannot be used as a package segment
// or an identifier. A webapp directory may legitimately be named "public" or
// "static"; the generated Java then needs a distinct name.
var javaReservedWords = map[string]struct{}{
	"abstract": {}, "assert": {}, "boolean": {}, "break": {}, "byte": {},
	"case": {}, "catch": {}, "char": {}, "class": {}, "const": {},
	"continue": {}, "default": {}, "do": {}, "double": {}, "else": {},
	"enum": {}, "extends": {}, "final": {}, "finally": {}, "float": {},
	"for": {}, "goto": {}, "if": {}, "implements": {}, "import": {},
	"instanceof": {}, "int": {}, "interface": {}, "long": {}, "native": {},
	"new": {}, "package": {}, "private": {}, "protected": {}, "public": {},
	"return": {}, "short": {}, "static": {}, "strictfp": {}, "super": {},
	"switch": {}, "synchronized": {}, "this": {}, "throw": {}, "throws": {},
	"transient": {}, "try": {}, "void": {}, "volatile": {}, "while": {},
	"true": {}, "false": {}, "null": {}, "var": {}, "record": {},
	"sealed": {}, "permits": {}, "yield": {}, "non": {}, "module": {},
	"open": {}, "opens": {}, "requires": {}, "exports": {}, "to": {},
	"uses": {}, "provides": {}, "with": {}, "transitive": {},
}

// escapeJavaIdentifier prefixes a reserved word so it stays a valid identifier.
// Java keywords are case-sensitive: a segment named "Public" is already valid.
func escapeJavaIdentifier(name string) string {
	if _, reserved := javaReservedWords[name]; reserved {
		return "_" + name
	}
	return name
}

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
		part = escapeJavaIdentifier(part)
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
	return escapeJavaIdentifier(name)
}
