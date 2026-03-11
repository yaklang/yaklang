package ssa

import (
	"fmt"
	"strings"
)

// SanitizeStableNamePart keeps only ASCII letters and digits in generated names.
func SanitizeStableNamePart(raw string) string {
	if raw == "" {
		return "unnamed"
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	text := strings.Trim(b.String(), "_")
	if text == "" {
		return "unnamed"
	}
	return text
}

// NextStableName generates a sanitized stable name with an incrementing suffix.
func NextStableName(prefix string, seq *int, fallbackPrefix string) string {
	if fallbackPrefix == "" {
		fallbackPrefix = "tmp"
	}
	if prefix == "" {
		prefix = fallbackPrefix
	}

	next := 1
	if seq != nil {
		*seq = *seq + 1
		next = *seq
	}
	return fmt.Sprintf("%s_%d", SanitizeStableNamePart(prefix), next)
}
