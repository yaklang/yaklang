package match

import (
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
	"time"
)

func init() {
	regexp2.DefaultMatchTimeout = time.Millisecond * 100
}

func newPCREMatch(r *rule.ContentRule) matchHandler {
	matcher, err := r.PCREParsed.Matcher()
	if err != nil {
		return nil
	}
	return func(c *matchContext) error {
		var indexes []data.Matched
		buffer := c.GetBuffer(r.PCREParsed.Modifier())

		if r.PCREParsed.IgnoreEndNewline() {
			if buffer[len(buffer)-1] == '\n' {
				buffer = buffer[:len(buffer)-1]
			}
		}

		indexes = matcher.Match(buffer)
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		if r.PCREParsed.StartsWith() {
			if !c.Must(indexes[0].Pos == 0) {
				return nil
			}
		}

		prevMatch, existed := c.GetPrevMatched(r.PCREParsed.Modifier())

		if r.PCREParsed.Relative() && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos == pm.Pos+pm.Len {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) > 0) {
				return nil
			}
		}

		c.SetPrevMatched(r.PCREParsed.Modifier(), indexes)
		return nil
	}
}
