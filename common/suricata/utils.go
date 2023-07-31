package suricata

import (
	"bytes"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strconv"
	"strings"
)

var sRe = regexp.MustCompile(`(?i)\|(?P<single>[0-9a-f][0-9a-f])( (?P<after>[0-9a-f][0-9a-f]))*\|`)

func unquoteString(s string) string {
	rawStr, err := strconv.Unquote(s)
	if err != nil {
		return strings.Trim(strings.TrimSpace(s), `"`)
	}
	return sRe.ReplaceAllStringFunc(rawStr, func(origin string) string {
		origin = strings.Trim(origin, " |")
		origin = strings.ReplaceAll(origin, " ", "")
		var originBytes, _ = codec.DecodeHex(origin)
		return string(originBytes)
	})
}

// setIfNotZero set dst to src if dst is not zero value
// attention that dst should not be nil
func setIfNotZero[T comparable](dst *T, src T) bool {
	var zero T
	if zero == *dst {
		*dst = src
		return true
	}
	return false
}

// loadIfMapEz load value from map to dst if key exists
func loadIfMapEz[T any](m map[string]any, dst *T, key string) {
	if v, ok := m[key]; ok {
		if v, ok := v.(T); ok {
			*dst = v
		}
	}
}

func atoi(i string) int {
	parsed, _ := strconv.Atoi(i)
	return parsed
}

func atoistar(i string) *int {
	if i == "" {
		return nil
	}
	parsed, _ := strconv.Atoi(i)
	return &parsed
}

// find all index of sub in s
func bytesIndexAll(s []byte, sep []byte) []matched {
	var indexes []matched
	for {
		pos := bytes.Index(s, sep)
		if pos == -1 {
			break
		}
		indexes = append(indexes, matched{
			pos: pos,
			len: len(sep),
		})
		if pos+1 < len(s) {
			s = s[pos+1:]
		} else {
			break
		}
	}
	return indexes
}

func binarySearch[T any](slice []T, cmp func(T) int) int {
	if len(slice) == 1 {
		if cmp(slice[0]) < 0 {
			return 1
		}
		return 0
	}
	l := 0
	r := len(slice)
	for l < r {
		mid := (l + r) / 2
		if cmp(slice[mid]) < 0 {
			l = mid + 1
		} else {
			r = mid
		}
	}
	return l
}

func sliceFilter[T any](slice []T, filter func(T) bool) []T {
	var ret []T
	for _, v := range slice {
		if filter(v) {
			ret = append(ret, v)
		}
	}
	return ret
}

func negIf(flag bool, val bool) bool {
	if flag {
		return !val
	}
	return val
}
