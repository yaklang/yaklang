package rust

import "strings"

const legacyHashLength = 16

// demangleLegacy decodes the _ZN...17h<hash>E scheme. It rejects C++ names that
// share the prefix: a Rust name ends with a 16-hex-digit hash.
func demangleLegacy(name string) (string, error) {
	if !strings.HasPrefix(name, "_ZN") {
		return "", errNotRust
	}
	body := name[3:]
	if pos := strings.LastIndex(body, "E."); pos >= 0 {
		body = body[:pos+1]
	}
	if len(body) <= legacyHashLength+3 || body[len(body)-1] != 'E' {
		return "", errNotRust
	}
	hashStart := len(body) - legacyHashLength - 4
	if body[hashStart:hashStart+3] != "17h" {
		return "", errNotRust
	}
	hash := body[hashStart+3 : len(body)-1]
	if !isHex(hash) || distinctDigits(hash) < 5 {
		return "", errNotRust
	}
	body = body[:hashStart]

	var parts []string
	for len(body) > 0 {
		length, rest, ok := legacyLength(body)
		if !ok || length > len(rest) {
			return "", errNotRust
		}
		part, ok := legacyIdentifier(rest[:length])
		if !ok {
			return "", errNotRust
		}
		parts = append(parts, part)
		body = rest[length:]
	}
	if len(parts) == 0 {
		return "", errNotRust
	}
	return strings.Join(parts, "::"), nil
}

// legacyLength reads <decimal>["_"]. A trailing underscore separates the length
// from an identifier that starts with a digit.
func legacyLength(text string) (int, string, bool) {
	if text == "" || text[0] < '0' || text[0] > '9' {
		return 0, "", false
	}
	value := 0
	i := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		value = value*10 + int(text[i]-'0')
		if value > maxIdentLength {
			return 0, "", false
		}
		i++
	}
	if i < len(text) && text[i] == '_' {
		i++
	}
	return value, text[i:], true
}

// legacyIdentifier expands the $...$ escape forms of pre-v0 Rust names.
func legacyIdentifier(raw string) (string, bool) {
	raw = strings.TrimPrefix(raw, "_$")
	var out strings.Builder
	for i := 0; i < len(raw); {
		switch raw[i] {
		case '.':
			if strings.HasPrefix(raw[i:], "..") {
				out.WriteString("::")
				i += 2
				continue
			}
			out.WriteByte('.')
			i++
		case '$':
			if code, width := legacyEscape(raw[i:]); width > 0 {
				out.WriteString(code)
				i += width
				continue
			}
			out.WriteByte('$')
			i++
		default:
			out.WriteByte(raw[i])
			i++
		}
	}
	return out.String(), true
}

// legacyEscape returns the replacement text and the number of bytes consumed.
func legacyEscape(text string) (string, int) {
	switch {
	case strings.HasPrefix(text, "$C$"):
		return ",", 3
	case strings.HasPrefix(text, "$SP$"):
		return "@", 4
	case strings.HasPrefix(text, "$BP$"):
		return "*", 4
	case strings.HasPrefix(text, "$RF$"):
		return "&", 4
	case strings.HasPrefix(text, "$LT$"):
		return "<", 4
	case strings.HasPrefix(text, "$GT$"):
		return ">", 4
	case strings.HasPrefix(text, "$LP$"):
		return "(", 4
	case strings.HasPrefix(text, "$RP$"):
		return ")", 4
	case len(text) >= 5 && text[1] == 'u' && text[4] == '$':
		high, okHigh := hexDigit(text[2])
		low, okLow := hexDigit(text[3])
		if !okHigh || !okLow {
			return "", 0
		}
		return string([]byte{byte(high<<4 | low)}), 5
	}
	return "", 0
}

func isHex(text string) bool {
	for i := 0; i < len(text); i++ {
		if _, ok := hexDigit(text[i]); !ok {
			return false
		}
	}
	return len(text) == legacyHashLength
}

func distinctDigits(text string) int {
	var seen uint16
	for i := 0; i < len(text); i++ {
		digit, _ := hexDigit(text[i])
		seen |= 1 << digit
	}
	count := 0
	for seen != 0 {
		seen &= seen - 1
		count++
	}
	return count
}
