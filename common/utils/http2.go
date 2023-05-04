package utils

import "strings"

// lower returns the ASCII lowercase version of b.
func lower(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// The HTTP protocols are defined in terms of ASCII, not Unicode. This file
// contains helper functions which may use Unicode-aware functions which would
// otherwise be unsafe and could introduce vulnerabilities if used improperly.

// AsciiEqualFold is strings.EqualFold, ASCII only. It reports whether s and t
// are equal, ASCII-case-insensitively.
func AsciiEqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if lower(s[i]) != lower(t[i]) {
			return false
		}
	}
	return true
}

// isASCIIPrint returns whether s is ASCII and printable according to
// https://tools.ietf.org/html/rfc20#section-4.2.
func IsASCIIPrint(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

// asciiToLower returns the lowercase version of s if s is ASCII and printable,
// and whether or not it was.
func AsciiToLower(s string) (lower string, ok bool) {
	if !IsASCIIPrint(s) {
		return "", false
	}
	return strings.ToLower(s), true
}
