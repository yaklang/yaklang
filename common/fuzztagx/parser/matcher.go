package parser

import "regexp"

func (t *TagRule) Match(s string) {
	var matchRule func(i int, s string, rule Rule) (map[int]interface{}, int, bool)
	matchRule = func(i int, s string, rule Rule) (map[int]interface{}, int, bool) {
		result := map[int]interface{}{}
		current := 0
		ok := false
		switch ret := rule.(type) {
		case *TagRule:
			ret.Match(s)
		case string:
			res := regexp.MustCompile(ret).FindIndex([]byte(s))
			result[i] = s[res[0]:res[1]]
			current = res[1]
			ok = true
		case *RuleWithFlag:
			switch ret.Flag {
			case "*":
				matchRule(i, s, ret.Rule)
				ok = true
			case "+":
				matchRule(i, s, ret.Rule)
			}
		}
		return result, current, ok
	}
	current := 0
	for i, rule := range t.Rules {
		matched, p, _ := matchRule(i, s[current:], rule)
		current = p
		_ = matched
	}
}
