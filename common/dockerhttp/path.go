package dockerhttp

import "net/url"

// escapeName preserves object names as a single escaped URL path component.
func escapeName(name string) string { return url.PathEscape(name) }
