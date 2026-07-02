package loop_infosec_recon

import (
	"fmt"
	"strings"
)

// infosecPickFirstHTTPURL normalizes a seed/start URL. When raw contains commas
// (e.g. "http://a,https://b"), the first valid https:// URL is preferred; if none,
// the first valid http:// URL is used.
func infosecPickFirstHTTPURL(raw string) (normalized string, coerced bool, note string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, ""
	}
	if !strings.Contains(raw, ",") {
		return raw, false, ""
	}
	var httpFallback string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if err := infosecValidateHTTPURL(part); err != nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(part), "https://") {
			return part, true, fmt.Sprintf("multiple URLs in input; using first valid https URL: %s", part)
		}
		if httpFallback == "" && strings.HasPrefix(strings.ToLower(part), "http://") {
			httpFallback = part
		}
	}
	if httpFallback != "" {
		return httpFallback, true, fmt.Sprintf("multiple URLs in input; no https URL found, using first valid http URL: %s", httpFallback)
	}
	return raw, false, ""
}
