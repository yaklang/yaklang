package generate

import (
	"bytes"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"math/rand"
)

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

// find all index of sub in s
func bytesIndexAll(s []byte, sep []byte, nocase bool) []data.Matched {
	var cmp func([]byte, []byte) bool
	if nocase {
		cmp = bytes.EqualFold
	} else {
		cmp = bytes.EqualFold
	}

	var indexes []data.Matched
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if cmp(s[i:i+len(sep)], sep) {
			indexes = append(indexes, data.Matched{
				Pos: i,
				Len: len(sep),
			})
		}
	}

	return indexes
}

func contentRuleMap(rules []*rule.ContentRule) map[modifier.Modifier][]*rule.ContentRule {
	var mp = make(map[modifier.Modifier][]*rule.ContentRule)
	for _, r := range rules {
		mp[r.Modifier] = append(mp[r.Modifier], r)
	}
	return mp
}
