package match

import (
	"fmt"

	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

func newPCREMatch(r *rule.ContentRule) (matchHandler, error) {
	matcher, err := r.PCREParsed.Matcher()
	if err != nil {
		return nil, fmt.Errorf("compile pcre %q: %w", r.PCRE, err)
	}
	return func(c *matchContext) error {
		var indexes []data.Matched
		buffer := c.GetBuffer(r.Modifier)
		if (r.PCREParsed.RawBytes() || r.RawBytes) && c.PK != nil {
			buffer = c.PK.Data()
		}
		if r.DotPrefix {
			buffer = dotPrefixBuffer(buffer)
		}

		jumpPosition, hasJump := c.takeJumpCursor(r.Modifier)

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
		if hasJump {
			indexes = slices.DeleteFunc(indexes, func(match data.Matched) bool {
				absolute := match.Pos + baseOffset
				if r.PCREParsed.StartsWith() {
					return absolute != jumpPosition
				}
				return absolute < jumpPosition
			})
		}
		if r.PCREParsed.StartsWith() && !hasJump {
			indexes = slices.DeleteFunc(indexes, func(match data.Matched) bool {
				return match.Pos != 0
			})
		}
		if r.Negative {
			if len(indexes) > 0 {
				c.Reject()
			}
			return nil
		}
		if !c.Must(len(indexes) > 0) {
			return nil
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
