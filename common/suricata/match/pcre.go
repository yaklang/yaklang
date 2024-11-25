package match

import (
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
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
		allPrevMatchs, existed := c.GetPrevMatched(r.PCREParsed.Modifier())
		if existed && r.PCREParsed.Relative() {
			preMatch := utils.GetLastElement(allPrevMatchs)
			buffer = buffer[preMatch.Pos+preMatch.Len:]
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

		c.SetPrevMatched(r.PCREParsed.Modifier(), indexes)
		return nil
	}
}
