package utils

import (
	"strings"

	"github.com/dlclark/regexp2"
)

func Regexp2Compile(rawRule string, opts ...int) (string, regexp2.RegexOptions, *regexp2.Regexp, error) {
	var regexp2Opt regexp2.RegexOptions
	if len(opts) > 0 {
		regexp2Opt = regexp2.RegexOptions(opts[0])
	} else {
		regexp2Opt = regexp2.RegexOptions(regexp2.ECMAScript | regexp2.Multiline)
	}

	rule := rawRule
	if strings.HasPrefix(rawRule, "(?") {
		rightParenIndex := strings.IndexRune(rawRule, ')')
		modes := rawRule[2:rightParenIndex]
		shouldResetRule := true
		for _, mode := range strings.Split(modes, "") {
			switch mode {
			case "i":
				regexp2Opt |= regexp2.IgnoreCase
			case "s":
				regexp2Opt |= regexp2.Singleline
			case "m":
				regexp2Opt |= regexp2.Multiline
			case "n":
				regexp2Opt |= regexp2.ExplicitCapture
			case "c":
				regexp2Opt |= regexp2.Compiled
			case "x":
				regexp2Opt |= regexp2.IgnorePatternWhitespace
			case "r":
				regexp2Opt |= regexp2.RightToLeft
			default:
				shouldResetRule = false
			}
		}
		if shouldResetRule {
			rule = rawRule[rightParenIndex+1:]
		}
	} else {
		rule = rawRule
	}
	pattern, err := regexp2.Compile(rule, regexp2Opt)
	return rule, regexp2Opt, pattern, err
}
