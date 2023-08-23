package match

import (
	"bytes"
	"github.com/yaklang/yaklang/common/suricata/data"
)

func negIf(flag bool, val bool) bool {
	if flag {
		return !val
	}
	return val
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
