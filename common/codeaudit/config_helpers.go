package codeaudit

import "regexp"

var regexpCache = map[string]*regexp.Regexp{}

// regexpCompile compiles a regex with caching.
func regexpCompile(pattern string) *regexp.Regexp {
	if re, ok := regexpCache[pattern]; ok {
		return re
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	regexpCache[pattern] = re
	return re
}
