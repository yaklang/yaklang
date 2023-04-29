package utils

import (
	"github.com/gobwas/glob"
	"regexp"
)

func interfaceToStr(i interface{}) string {
	return InterfaceToString(i)
}

func MatchAnyOfSubString(i interface{}, re ...string) bool {
	raw := interfaceToStr(i)
	for _, subStr := range re {
		if IContains(raw, subStr) {
			return true
		}
	}
	return false
}

func MatchAllOfSubString(i interface{}, re ...string) bool {
	if len(re) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, subStr := range re {
		if !IContains(raw, subStr) {
			return false
		}
	}
	return true
}

func MatchAnyOfGlob(
	i interface{}, re ...string) bool {
	raw := interfaceToStr(i)
	for _, r := range re {
		if glob.MustCompile(r).Match(raw) {
			return true
		}
	}
	return false
}

func MatchAllOfGlob(
	i interface{}, re ...string) bool {
	if len(re) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, r := range re {
		if !glob.MustCompile(r).Match(raw) {
			return false
		}
	}
	return true
}

func MatchAnyOfRegexp(
	i interface{},
	re ...string) bool {
	raw := interfaceToStr(i)
	for _, r := range re {
		result, err := regexp.MatchString(r, raw)
		if err != nil {
			continue
		}
		if result {
			return true
		}
	}
	return false
}

func MatchAllOfRegexp(
	i interface{},
	re ...string) bool {
	if len(re) <= 0 {
		return false
	}

	raw := interfaceToStr(i)
	for _, r := range re {
		result, err := regexp.MatchString(r, raw)
		if err != nil {
			return false
		}
		if !result {
			return false
		}
	}
	return true
}
