package match

import (
	"fmt"

	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func newPCREMatch(r *rule.ContentRule) (matchHandler, error) {
	matcher, err := r.PCREParsed.Matcher()
	if err != nil {
		return nil, fmt.Errorf("compile pcre %q: %w", r.PCRE, err)
	}
	return func(c *matchContext) error {
		var indexes []data.Matched
		buffer := c.GetBuffer(r.Modifier)

		if r.PCREParsed.IgnoreEndNewline() && len(buffer) > 0 && buffer[len(buffer)-1] == '\n' {
			buffer = buffer[:len(buffer)-1]
		}

		baseOffset := 0
		allPrevMatches, existed := c.GetPrevMatched(r.Modifier)
		if existed && r.PCREParsed.Relative() {
			if !c.Must(len(allPrevMatches) > 0) {
				return nil
			}
			prevMatch := allPrevMatches[0]
			baseOffset = prevMatch.Pos + prevMatch.Len
			if !c.Must(baseOffset >= 0 && baseOffset <= len(buffer)) {
				return nil
			}
			buffer = buffer[baseOffset:]
		}

		indexes, err = matcher.Match(buffer)
		if err != nil {
			c.Reject()
			return fmt.Errorf("match pcre %q: %w", r.PCRE, err)
		}
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		if r.PCREParsed.StartsWith() {
			if !c.Must(indexes[0].Pos == 0) {
				return nil
			}
		}
		if baseOffset != 0 {
			for i := range indexes {
				indexes[i].Pos += baseOffset
			}
		}

		c.SetPrevMatched(r.Modifier, indexes)
		return nil
	}, nil
}
