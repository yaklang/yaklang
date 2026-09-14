package codeaudit

import "regexp"

// regexpCache is the single shared compilation cache for the Go-side regex
// matching that remains in codeaudit (CMS fingerprint markers). All rule
// matching itself is delegated to the SyntaxFlow source rules in sfaudit.
var regexpCache = map[string]*regexp.Regexp{}

// regexpCompileCached compiles a regex with caching. It returns nil when the
// pattern is invalid (failures are cached too), so callers can fall back to
// plain string matching instead of panicking.
func regexpCompileCached(pattern string) *regexp.Regexp {
	if re, ok := regexpCache[pattern]; ok {
		return re
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		regexpCache[pattern] = nil
		return nil
	}
	regexpCache[pattern] = re
	return re
}
