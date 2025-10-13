package minimartian

import (
	"regexp"
	"strings"
)

// ProxyHostMatcher matches request hosts against a whitelist of hostname/domain patterns.
// Supported patterns:
//   - "*" matches all hosts
//   - exact host names (case-insensitive)
//   - suffix matches via ".example.com" or "*.example.com"
//   - simple '*' wildcard in other positions (converted to regex)
type ProxyHostMatcher struct {
	matchAll bool
	exact    map[string]struct{}
	suffixes []string
	regex    []*regexp.Regexp
}

// NewProxyHostMatcher builds a ProxyHostMatcher instance from the provided patterns.
func NewProxyHostMatcher(patterns []string) *ProxyHostMatcher {
	m := &ProxyHostMatcher{
		exact: make(map[string]struct{}),
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ToLower(p)
		switch {
		case p == "*":
			m.matchAll = true
		case strings.HasPrefix(p, "*.") || strings.HasPrefix(p, "."):
			suffix := strings.TrimPrefix(p, "*.")
			suffix = strings.TrimPrefix(suffix, ".")
			if suffix == "" {
				m.matchAll = true
				continue
			}
			m.suffixes = append(m.suffixes, suffix)
		case strings.Contains(p, "*"):
			escaped := regexp.QuoteMeta(p)
			escaped = strings.ReplaceAll(escaped, "\\*", ".*")
			if re, err := regexp.Compile("^" + escaped + "$"); err == nil {
				m.regex = append(m.regex, re)
			} else {
				m.exact[p] = struct{}{}
			}
		default:
			m.exact[p] = struct{}{}
		}
	}
	return m
}

// Match reports whether host matches the configured patterns.
func (m *ProxyHostMatcher) Match(host string) bool {
	if m == nil {
		return false
	}
	if m.matchAll {
		return true
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if _, ok := m.exact[host]; ok {
		return true
	}
	for _, suffix := range m.suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	for _, re := range m.regex {
		if re.MatchString(host) {
			return true
		}
	}
	return false
}
