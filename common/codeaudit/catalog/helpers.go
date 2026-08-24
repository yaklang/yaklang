package catalog

import (
	"regexp"
	"strings"
)

var compiledCache = map[string]*regexp.Regexp{}

// mustCompile compiles a regex pattern, with caching.
func mustCompile(pattern string) *regexp.Regexp {
	if re, ok := compiledCache[pattern]; ok {
		return re
	}
	re := regexp.MustCompile(pattern)
	compiledCache[pattern] = re
	return re
}

// matchSimple checks if content contains a substring matching the regex pattern.
func matchSimple(content, pattern string) bool {
	re := mustCompile(pattern)
	return re.MatchString(content)
}

// containsStar checks if a string contains an asterisk wildcard.
func containsStar(s string) bool {
	return strings.Contains(s, "*")
}

// containsStr checks if a string contains a substring (case-insensitive).
func containsStr(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// isPlaceholder checks if a value looks like a placeholder, not a real secret.
func isPlaceholder(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"'`)
	if v == "" {
		return true
	}
	lower := strings.ToLower(v)
	placeholders := []string{
		"changeme", "password", "123456", "admin", "xxx",
		"your_password", "yourpassword", "example", "test",
		"null", "none", "secret", "${", "todo", "placeholder",
		"your_secret", "yoursecret", "your-api-key",
		"replace_me", "replaceme", "fill_me", "fillme",
	}
	for _, p := range placeholders {
		if lower == p {
			return true
		}
	}
	// Check for env var references
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		return true
	}
	return false
}

// maskSecret masks the middle part of a secret for display.
func maskSecret(v string) string {
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	return strings.Split(content, "\n")
}

// trimSpace trims whitespace from a string.
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// hasPrefix checks if a string has a given prefix.
func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// findLineWith finds and returns the first line containing the given substring.
func findLineWith(content, sub string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, sub) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
