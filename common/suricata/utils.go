package suricata

import (
	"bytes"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

var sRe = regexp.MustCompile(`(?i)\|(?P<single>[0-9a-f][0-9a-f])( (?P<after>[0-9a-f][0-9a-f]))*\|`)

func unquoteAndParseHex(s string) string {
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

var unquoteReplacer = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\;`, `;`)

func unquoteString(s string) string {
	if !(strings.HasSuffix(s, `"`) && strings.HasPrefix(s, `"`)) {
		return s
	}
	s = strings.Trim(s, `"`)
	var tmp string
	tmp = unquoteReplacer.Replace(s)
	for tmp != s {
		s = tmp
		tmp = unquoteReplacer.Replace(s)
	}
	return tmp
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
func bytesIndexAll(s []byte, sep []byte, nocase bool) []matched {
	var cmp func([]byte, []byte) bool
	if nocase {
		cmp = bytes.EqualFold
	} else {
		cmp = bytes.EqualFold
	}

	var indexes []matched
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if cmp(s[i:i+len(sep)], sep) {
			indexes = append(indexes, matched{
				pos: i,
				len: len(sep),
			})
		}
	}

	return indexes
}

func negIf(flag bool, val bool) bool {
	if flag {
		return !val
	}
	return val
}

func nocaseFilter(input []byte) []byte {
	var buf = make([]byte, len(input))
	copy(buf, input)
	for i := 0; i < len(buf); i++ {
		if buf[i] >= 'a' && buf[i] <= 'z' {
			if randBool() {
				buf[i] = buf[i] - 'a' + 'A'
			}
		} else if buf[i] >= 'A' && buf[i] <= 'Z' {
			if randBool() {
				buf[i] = buf[i] - 'A' + 'a'
			}
		}
	}
	return buf
}

func randBool() bool {
	return rand.Int63()%2 == 0
}
