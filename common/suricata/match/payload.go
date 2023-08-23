package match

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
	"strconv"
	"strings"
)

func newPayloadMatcher(r *rule.ContentRule, mdf modifier.Modifier) matchHandler {
	if r.PCRE != "" {
		// pcre match
		return newPCREMatch(r)
	}
	return func(c *matchContext) error {
		if len(r.Content) == 0 {
			return nil
		}

		var indexes []data.Matched
		buffer := c.GetBuffer(mdf)

		defer func() {
			if r.Negative && c.IsRejected() {
				c.Recover()
			} else if r.Negative && !c.IsRejected() {
				c.Reject()
			}
		}()

		// match all
		indexes = bytesIndexAll(buffer, r.Content, r.Nocase)
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		// special options startswith
		if r.StartsWith {
			if !c.Must(indexes[0].Pos == 0) {
				return nil
			}
			c.Value["prevMatch"] = []data.Matched{indexes[0]}
			return nil
		}

		// special options endswith
		if r.EndsWith {
			targetPos := len(buffer) - len(r.Content)
			// depth is valid in endswith
			if r.Depth != nil {
				targetPos = *r.Depth - len(r.Content)
			}

			if _, found := slices.BinarySearchFunc(indexes, targetPos, func(m data.Matched, i int) int {
				return m.Pos - i
			}); !c.Must(found) {
				c.Value["prevMatch"] = []data.Matched{indexes[0]}
			}

			return nil
		}

		// depth & offset
		// [le,ri]
		if r.Depth != nil || r.Offset != nil {
			le := 0
			ri := len(buffer)

			if r.Offset != nil {
				le = *r.Offset
			}

			if r.Depth != nil {
				ri = le + *r.Depth - len(r.Content) + 1
			}

			// [lp,rp)
			lp, _ := slices.BinarySearchFunc(indexes, le, func(m data.Matched, i int) int {
				return m.Pos - i
			})

			rp, _ := slices.BinarySearchFunc(indexes, ri, func(m data.Matched, i int) int {
				return m.Pos - i
			})

			indexes = indexes[lp:rp]
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// load prev matches for rel checker
		prevMatch, existed := c.GetPrevMatched(mdf)

		// distance
		if r.Distance != nil && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos >= pm.Pos+pm.Len+*r.Distance {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// within
		if r.Within != nil && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos+m.Len <= pm.Pos+pm.Len+*r.Within {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}
		// isdataat
		if r.IsDataAt != "" {
			strpos := strings.Split(r.IsDataAt, ",")
			var neg bool
			var strnum string
			if len(strpos[0]) > 1 && strpos[0][0] == '!' {
				neg = true
				strnum = strpos[0][1:]
			} else {
				strnum = strpos[0]
			}
			pos, err := strconv.Atoi(strnum)
			if err != nil {
				return errors.Wrap(err, "isdataat format error")
			}
			if len(strpos) == 1 {
				// no relative
				indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
					return negIf(neg, pos >= len(buffer))
				})
			} else {
				// with reletive
				if !c.Must(len(strpos) == 2 && strpos[1] == "relative") {
					return errors.New("isdataat format error")
				}
				indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
					return negIf(neg, m.Pos+m.Len+pos > len(buffer))
				})
			}
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// todo:bsize dsize
		if r.DSize != "" && r.Modifier == modifier.Default {

		}

		c.SetPrevMatched(mdf, indexes)
		return nil
	}
}
