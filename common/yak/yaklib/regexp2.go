package yaklib

import (
	"fmt"
	"github.com/dlclark/regexp2"
	"strings"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func re2Find(data interface{}, text string) string {
	re, err := re2Compile(text)
	if err != nil {
		return ""
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(data))
	if err != nil {
		return ""
	}
	return match.String()
}

func re2FindAll(data interface{}, text string) []string {
	re, err := re2Compile(text)
	if err != nil {
		return nil
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(data))
	if err != nil {
		return nil
	}
	var results []string
	for {
		results = append(results, match.String())
		if nextMatch, err := re.FindNextMatch(match); err == nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

func re2FindSubmatch(i interface{}, rule string) []string {
	re, err := re2Compile(rule)
	if err != nil {
		log.Error(err)
		return nil
	}
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		log.Error(err)
		return nil
	}
	var result = make([]string, match.GroupCount())
	for index, g := range match.Groups() {
		result[index] = g.String()
	}
	return result
}

func re2FindSubmatchAll(i interface{}, raw string) [][]string {
	re, err := re2Compile(raw)
	if err != nil {
		log.Errorf("re2 compile failed: %s", err)
		return nil
	}
	var results [][]string
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		log.Error(err)
		return nil
	}
	for {
		results = append(results, funk.Map(match.Groups(), func(i regexp2.Group) string {
			return i.String()
		}).([]string))
		if nextMatch, err := re.FindNextMatch(match); err == nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

func re2Compile(rawRule string) (*regexp2.Regexp, error) {
	opt := regexp2.ECMAScript | regexp2.Multiline
	var rule string
	if strings.HasPrefix(rawRule, "(?i)") {
		rule = rawRule[4:]
		opt |= regexp2.IgnoreCase
	} else if strings.HasPrefix(rawRule, `(?s)`) {
		rule = rawRule[4:]
		opt |= regexp2.Singleline
	} else if strings.HasPrefix(rawRule, `(?si)`) || strings.HasPrefix(rawRule, `(?si)`) {
		rule = rawRule[5:]
		opt |= regexp2.Singleline | regexp2.IgnoreCase
	} else {
		rule = rawRule
	}
	return regexp2.Compile(rule, regexp2.RegexOptions(opt))
}

func re2ReplaceAll(i interface{}, pattern string, target string) string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return utils.InterfaceToString(i)
	}
	raw := utils.InterfaceToString(i)
	m, err := re.Replace(raw, target, 0, -1)
	if err != nil {
		return raw
	}
	return m
}

func re2ReplaceAllFunc(i interface{}, pattern string, target func(string) string) string {
	re, err := re2Compile(pattern)
	if err != nil {
		log.Error(err)
		return utils.InterfaceToString(i)
	}
	raw := utils.InterfaceToString(i)
	m, err := re.ReplaceFunc(raw, regexp2.MatchEvaluator(func(match regexp2.Match) string {
		return target(match.String())
	}), 0, -1)
	if err != nil {
		return raw
	}
	return m
}

func re2ExtractGroups(i interface{}, raw string) map[string]string {
	re, err := re2Compile(raw)
	if err != nil {
		log.Error(err)
		return make(map[string]string)
	}

	result := make(map[string]string)
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		return make(map[string]string)
	}
	result["__all__"] = match.String()
	for _, value := range match.Groups() {
		if value.Name == "" {
			result[fmt.Sprint(value.Index)] = value.String()
		} else {
			result[value.Name] = value.String()
		}
	}
	return result
}

func re2ExtractGroupsAll(i interface{}, raw string) []map[string]string {
	re, err := re2Compile(raw)
	if err != nil {
		log.Error(err)
		return nil
	}

	var results []map[string]string
	match, err := re.FindStringMatch(utils.InterfaceToString(i))
	if err != nil {
		return nil
	}

	for {
		var result = make(map[string]string)
		result["__all__"] = match.String()
		for _, value := range match.Groups() {
			if value.Name == "" {
				result[fmt.Sprint(value.Index)] = value.String()
			} else {
				result[value.Name] = value.String()
			}
		}
		results = append(results, result)

		if nextMatch, err := re.FindNextMatch(match); err == nil {
			match = nextMatch
		} else {
			break
		}
	}
	return results
}

var Regexp2Export = map[string]interface{}{
	"QuoteMeta": regexp2.Escape,
	"Compile":   re2Compile,
	"CompileWithOption": func(rule string, opt int) (*regexp2.Regexp, error) {
		return regexp2.Compile(rule, regexp2.RegexOptions(opt))
	},
	"OPT_None":                    regexp2.None,
	"OPT_IgnoreCase":              regexp2.IgnoreCase,
	"OPT_Multiline":               regexp2.Multiline,
	"OPT_ExplicitCapture":         regexp2.ExplicitCapture,
	"OPT_Compiled":                regexp2.Compiled,
	"OPT_Singleline":              regexp2.Singleline,
	"OPT_IgnorePatternWhitespace": regexp2.IgnorePatternWhitespace,
	"OPT_RightToLeft":             regexp2.RightToLeft,
	"OPT_Debug":                   regexp2.Debug,
	"OPT_ECMAScript":              regexp2.ECMAScript,
	"OPT_RE2":                     regexp2.RE2,

	"Find":               re2Find,
	"FindAll":            re2FindAll,
	"FindSubmatch":       re2FindSubmatch,
	"FindSubmatchAll":    re2FindSubmatchAll,
	"FindGroup":          re2ExtractGroups,
	"FindGroupAll":       re2ExtractGroupsAll,
	"ReplaceAll":         re2ReplaceAll,
	"ReplaceAllWithFunc": re2ReplaceAllFunc,
}
