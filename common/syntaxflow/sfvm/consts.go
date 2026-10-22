package sfvm

// FileFilterParamKey is an internal opcode parameter stored in
// FileFilterMethodItem. It is not a SyntaxFlow keyword. The string values are
// part of the persisted opcode JSON and must stay stable.
type FileFilterParamKey string

const (
	// FileFilterParamPatternNotList marks pattern_regex_not. Values[0] is the
	// positive pattern and every later value is a negative pattern.
	FileFilterParamPatternNotList FileFilterParamKey = "__sf_pattern_not_list"
	// FileFilterParamPatternNot marks a negative-only content regexp
	// (pattern_not_regex / not_regexp). On a chained hit the hit itself is the
	// positive match.
	FileFilterParamPatternNot FileFilterParamKey = "__sf_pattern_not"
)

func (k FileFilterParamKey) String() string { return string(k) }

// FileFilterParamOn reports whether a persisted opcode parameter is set.
func FileFilterParamOn(params map[string]string, key FileFilterParamKey) bool {
	return params != nil && params[string(key)] == "1"
}

// SetFileFilterParam records an internal file-filter flag. A nil map is allocated.
func SetFileFilterParam(params map[string]string, key FileFilterParamKey) map[string]string {
	if params == nil {
		params = make(map[string]string, 1)
	}
	params[string(key)] = "1"
	return params
}
