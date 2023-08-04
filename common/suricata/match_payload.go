package suricata

import (
	"bytes"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	"strconv"
	"strings"
)

func newPayloadMatcher(r *ContentRule, content []byte) func(c *matchContext) error {
	return func(c *matchContext) error {
		buffer := make([]byte, len(content))
		copy(buffer, content)
		if r.Nocase {
			r.Content = bytes.ToLower(r.Content)
			buffer = bytes.ToLower(buffer)
		}

		// pcre not implement yet, temporarily skip
		if len(content) == 0 {
			c.Reject()
			return nil
		}

		// match all
		indexes := bytesIndexAll(content, r.Content)
		if !c.Must(len(indexes) > 0) {
			return nil
		}

		// special options startswith
		if r.StartsWith {
			if !c.Must(indexes[0].pos == 0) {
				return nil
			}
			c.Value["prevMatch"] = []matched{indexes[0]}
			return nil
		}

		// special options endswith
		if r.EndsWith {
			targetPos := len(content) - len(r.Content)
			// depth is valid in endswith
			if r.Depth != nil {
				targetPos = *r.Depth - len(r.Content)
			}

			if _, found := slices.BinarySearchFunc(indexes, targetPos, func(m matched, i int) int {
				return m.pos - i
			}); !c.Must(found) {
				c.Value["prevMatch"] = []matched{indexes[0]}
			}

			return nil
		}

		// depth & offset
		// [le,ri]
		if r.Depth != nil || r.Offset != nil {
			le := 0
			ri := len(content)

			if r.Offset != nil {
				le = *r.Offset
			}

			if r.Depth != nil {
				ri = le + *r.Depth - len(r.Content) + 1
			}

			// [lp,rp)
			lp, _ := slices.BinarySearchFunc(indexes, le, func(m matched, i int) int {
				return m.pos - i
			})

			rp, _ := slices.BinarySearchFunc(indexes, ri, func(m matched, i int) int {
				return m.pos - i
			})

			indexes = indexes[lp:rp]
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		// load prev matches for rel checker
		var prevMatch []matched
		loadIfMapEz(c.Value, &prevMatch, "prevMatch")

		// distance
		if r.Distance != nil {
			indexes = slices.DeleteFunc(indexes, func(m matched) bool {
				for _, pm := range prevMatch {
					if m.pos == pm.pos+pm.len+*r.Distance {
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
		if r.Within != nil {
			indexes = slices.DeleteFunc(indexes, func(m matched) bool {
				for _, pm := range prevMatch {
					if m.pos+m.len <= pm.pos+pm.len+*r.Within {
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
				indexes = slices.DeleteFunc(indexes, func(m matched) bool {
					return negIf(neg, pos >= len(content))
				})
			} else {
				// with reletive
				if !c.Must(len(strpos) == 2 && strpos[1] == "relative") {
					return errors.New("isdataat format error")
				}
				indexes = slices.DeleteFunc(indexes, func(m matched) bool {
					return negIf(neg, m.pos+m.len+pos > len(content))
				})
			}
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		if r.DSize != "" && r.Modifier == Default {

		}
		// todo:bsize dsize
		c.Value["prevMatch"] = indexes
		return nil
	}
}
