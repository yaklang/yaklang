package yaklib

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

var reMatch = func(pattern string, i interface{}) bool {
	r, err := regexp.Compile(pattern)
	if err != nil {
		_diewith(utils.Errorf("compile[%v] failed: %v", pattern, err))
		return false
	}

	switch ret := i.(type) {
	case []byte:
		return r.Match(ret)
	case string:
		return r.MatchString(ret)
	default:
		_diewith(utils.Errorf("target: %v should be []byte or string", spew.Sdump(i)))
	}
	return false
}

func _find_extractByRegexp(origin interface{}, re string) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return ""
	}
	return r.FindString(utils.InterfaceToString(origin))
}

func _findAll_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllString(utils.InterfaceToString(origin), -1)
}

func _findAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringIndex(utils.InterfaceToString(origin), -1)
}

func _findIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringIndex(utils.InterfaceToString(origin))
}

func _findSubmatch_extractByRegexp(origin interface{}, re string) []string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatch(utils.InterfaceToString(origin))
}

func _findSubmatchIndex_extractByRegexp(origin interface{}, re string) []int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindStringSubmatchIndex(utils.InterfaceToString(origin))
}

func _findSubmatchAll_extractByRegexp(origin interface{}, re string) [][]string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatch(utils.InterfaceToString(origin), -1)
}

func _findSubmatchAllIndex_extractByRegexp(origin interface{}, re string) [][]int {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return nil
	}
	return r.FindAllStringSubmatchIndex(utils.InterfaceToString(origin), -1)
}

func _replaceAllFunc_extractByRegexp(origin interface{}, re string, newStr func(string) string) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllStringFunc(utils.InterfaceToString(origin), newStr)
}

func _replaceAll_extractByRegexp(origin interface{}, re string, newStr interface{}) string {
	r, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("compile %v failed: %s", re, err)
		return utils.InterfaceToString(origin)
	}
	return r.ReplaceAllString(utils.InterfaceToString(origin), utils.InterfaceToString(newStr))
}

func reExtractGroups(i interface{}, raw string) map[string]string {
	re, err := regexp.Compile(raw)
	if err != nil {
		log.Error(err)
		return make(map[string]string)
	}
	var matchIndex = map[int]string{}
	for _, name := range re.SubexpNames() {
		matchIndex[re.SubexpIndex(name)] = name
	}

	result := make(map[string]string)
	for index, value := range re.FindStringSubmatch(utils.InterfaceToString(i)) {
		name, ok := matchIndex[index]
		if !ok {
			name = fmt.Sprint(index)
		}
		result[name] = value
	}
	return result
}

func reExtractGroupsAll(i interface{}, raw string) []map[string]string {
	re, err := regexp.Compile(raw)
	if err != nil {
		log.Error(err)
		return nil
	}
	var matchIndex = map[int]string{}
	for _, name := range re.SubexpNames() {
		matchIndex[re.SubexpIndex(name)] = name
	}

	var results []map[string]string
	for _, matches := range re.FindAllStringSubmatch(utils.InterfaceToString(i), -1) {
		result := make(map[string]string)
		for index, value := range matches {
			name, ok := matchIndex[index]
			if !ok {
				name = fmt.Sprint(index)
			}
			result[name] = value
		}
		results = append(results, result)
	}
	return results
}

var RegexpExport = map[string]interface{}{
	"QuoteMeta":        regexp.QuoteMeta,
	"Compile":          regexp.Compile,
	"CompilePOSIX":     regexp.CompilePOSIX,
	"MustCompile":      regexp.MustCompile,
	"MustCompilePOSIX": regexp.MustCompilePOSIX,

	"Match":                reMatch,
	"Grok":                 Grok,
	"ExtractIPv4":          RegexpMatchIPv4,
	"ExtractIPv6":          RegexpMatchIPv6,
	"ExtractIP":            RegexpMatchIP,
	"ExtractEmail":         RegexpMatchEmail,
	"ExtractPath":          RegexpMatchPathParam,
	"ExtractTTY":           RegexpMatchTTY,
	"ExtractURL":           RegexpMatchURL,
	"ExtractHostPort":      RegexpMatchHostPort,
	"ExtractMac":           RegexpMatchMac,
	"Find":                 _find_extractByRegexp,
	"FindIndex":            _findIndex_extractByRegexp,
	"FindAll":              _findAll_extractByRegexp,
	"FindAllIndex":         _findAllIndex_extractByRegexp,
	"FindSubmatch":         _findSubmatch_extractByRegexp,
	"FindSubmatchIndex":    _findSubmatchIndex_extractByRegexp,
	"FindSubmatchAll":      _findSubmatchAll_extractByRegexp,
	"FindSubmatchAllIndex": _findSubmatchAllIndex_extractByRegexp,
	"FindGroup":            reExtractGroups,
	"FindGroupAll":         reExtractGroupsAll,
	"ReplaceAll":           _replaceAll_extractByRegexp,
	"ReplaceAllWithFunc":   _replaceAllFunc_extractByRegexp,
}
