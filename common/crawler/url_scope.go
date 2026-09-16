package crawler

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// WithURLScope restricts scheduling and redirects to explicit HTTP(S) origins
// and path subtrees. It intersects the existing hostname scope; candidates
// outside this scope can still be reported as discovered, without fetching.
func WithURLScope(prefixes ...string) (ConfigOpt, error) {
	scopes := make([]*url.URL, 0, len(prefixes))
	for _, raw := range prefixes {
		raw = strings.TrimSpace(raw)
		if SanitizeTextForDisplay(raw) != raw {
			return nil, fmt.Errorf("invalid URL scope: control characters or oversized input")
		}
		u, err := url.Parse(raw)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return nil, fmt.Errorf("invalid URL scope: expected an HTTP(S) origin/path without credentials, query or fragment")
		}
		u.Path = path.Clean("/" + strings.TrimPrefix(u.Path, "/"))
		scopes = append(scopes, u)
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("URL scope must contain at least one origin/path")
	}
	return func(c *Config) { c.urlScopes = scopes }, nil
}

func scopePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func inURLScopes(u *url.URL, scopes []*url.URL) bool {
	if u == nil {
		return false
	}
	candidatePath := path.Clean("/" + strings.TrimPrefix(u.Path, "/"))
	for _, scope := range scopes {
		if u.Scheme != scope.Scheme || normalizeHostnameOrPattern(u.Hostname()) != normalizeHostnameOrPattern(scope.Hostname()) || scopePort(u) != scopePort(scope) {
			continue
		}
		if scope.Path == "/" || candidatePath == scope.Path || strings.HasPrefix(candidatePath, scope.Path+"/") {
			return true
		}
	}
	return false
}
