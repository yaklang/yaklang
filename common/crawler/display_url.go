package crawler

import (
	"fmt"
	"net/url"
	"strings"
)

const maxCrawlerDisplayURLBytes = 16 * 1024
const maxCrawlerDisplayTextBytes = 4 * 1024

// SanitizeTextForDisplay bounds untrusted response metadata and escapes C0,
// C1, and Unicode line-separator controls before it is embedded in a textual
// crawler report. It does not redact ordinary text or credentials; URLs must
// still use RedactURLForDisplay.
func SanitizeTextForDisplay(raw string) string {
	if len(raw) > maxCrawlerDisplayTextBytes {
		return fmt.Sprintf("[OMITTED: display text exceeded %d bytes; original_bytes=%d]", maxCrawlerDisplayTextBytes, len(raw))
	}
	return escapeAIJSDisplayControls(raw)
}

// RedactURLForDisplay returns a fragment-free URL suitable for tool output,
// logs, and model context. It is deliberately display-only: callers must keep
// using the original URL for requests, queue identity, and coverage accounting.
//
// Userinfo is removed and credential-like query values are replaced while the
// original query ordering, separators, harmless values, and escaping are kept
// byte-for-byte wherever possible.
func RedactURLForDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > maxCrawlerDisplayURLBytes {
		return fmt.Sprintf("[OMITTED: display URL exceeded %d bytes; original_bytes=%d]", maxCrawlerDisplayURLBytes, len(raw))
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return "[REDACTED: malformed URL]"
	}
	parsed.User = nil
	parsed.Fragment = ""
	parsed.RawQuery = redactDisplayRawQuery(parsed.RawQuery)
	return escapeAIJSDisplayControls(parsed.String())
}

func redactDisplayRawQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	var result strings.Builder
	result.Grow(len(rawQuery))
	fieldStart := 0
	for i := 0; i <= len(rawQuery); i++ {
		if i != len(rawQuery) && rawQuery[i] != '&' && rawQuery[i] != ';' {
			continue
		}
		result.WriteString(redactDisplayQueryField(rawQuery[fieldStart:i]))
		if i < len(rawQuery) {
			result.WriteByte(rawQuery[i])
		}
		fieldStart = i + 1
	}
	return result.String()
}

func redactDisplayQueryField(field string) string {
	key := field
	if equal := strings.IndexByte(field, '='); equal >= 0 {
		key = field[:equal]
	}
	decodedKey, err := url.QueryUnescape(key)
	if err != nil {
		decodedKey = key
	}
	if !isSensitiveCredentialName(decodedKey) {
		return field
	}
	return key + "=[REDACTED]"
}
