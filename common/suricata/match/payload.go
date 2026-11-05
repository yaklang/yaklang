package match

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/suricata/data"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/numrange"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
)

func newPayloadMatcher(r *rule.ContentRule, mdf modifier.Modifier) (matchHandler, error) {
	var bsizeRange, dsizeRange *numrange.NumRange
	var err error
	if r.BSize != "" {
		bsizeRange, err = numrange.ParseNumRange(r.BSize)
		if err != nil {
			return nil, errors.Wrap(err, "invalid bsize")
		}
	}
	if r.DSize != "" {
		dsizeRange, err = numrange.ParseNumRange(r.DSize)
		if err != nil {
			return nil, errors.Wrap(err, "invalid dsize")
		}
	}

	var contentMatcher matchHandler
	if r.PCREParsed != nil {
		contentMatcher, err = newPCREMatch(r)
		if err != nil {
			return nil, err
		}
	}

	return func(c *matchContext) error {
		buffer := c.GetBuffer(mdf)
		if bsizeRange != nil && !c.Must(bsizeRange.Match(len(buffer))) {
			return nil
		}
		if dsizeRange != nil {
			packetPayload := c.GetBuffer(modifier.Default)
			if transportLayer := c.PK.TransportLayer(); transportLayer != nil {
				packetPayload = transportLayer.LayerPayload()
			}
			if !c.Must(dsizeRange.Match(len(packetPayload))) {
				return nil
			}
		}
		if contentMatcher != nil {
			return contentMatcher(c)
		}
		if len(r.Content) == 0 {
			if r.IsDataAt == "" {
				return nil
			}
			return matchStandaloneIsDataAt(c, buffer, r.IsDataAt)
		}

		indexes := bytesIndexAll(buffer, r.Content, r.Nocase)

		defer func() {
			if r.Negative && c.IsRejected() {
				c.Recover()
			} else if r.Negative && !c.IsRejected() {
				c.Reject()
			}
		}()

		if !c.Must(len(indexes) > 0) {
			return nil
		}

		if r.StartsWith {
			if !c.Must(indexes[0].Pos == 0) {
				return nil
			}
			c.SetPrevMatched(mdf, []data.Matched{indexes[0]})
			return nil
		}

		if r.EndsWith {
			targetPos := len(buffer) - len(r.Content)
			if r.Depth != nil {
				targetPos = *r.Depth - len(r.Content)
			}

			idx, found := slices.BinarySearchFunc(indexes, targetPos, func(m data.Matched, i int) int {
				return m.Pos - i
			})
			if !c.Must(found) {
				return nil
			}
			c.SetPrevMatched(mdf, []data.Matched{indexes[idx]})
			return nil
		}

		if r.Depth != nil || r.Offset != nil {
			left := 0
			if r.Offset != nil {
				left = *r.Offset
			}
			if !c.Must(left >= 0) {
				return nil
			}
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				if m.Pos < left {
					return true
				}
				return r.Depth != nil && (*r.Depth < 0 || m.Pos+m.Len > left+*r.Depth)
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

		prevMatch, existed := c.GetPrevMatched(mdf)

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

		if r.Within != nil && existed {
			indexes = slices.DeleteFunc(indexes, func(m data.Matched) bool {
				for _, pm := range prevMatch {
					if m.Pos >= pm.Pos+pm.Len && m.Pos+m.Len <= pm.Pos+pm.Len+*r.Within {
						return false
					}
				}
				return true
			})
			if !c.Must(len(indexes) != 0) {
				return nil
			}
		}

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
				indexes = slices.DeleteFunc(indexes, func(data.Matched) bool {
					return negIf(neg, pos >= len(buffer))
				})
			} else {
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

		c.SetPrevMatched(mdf, indexes)
		return nil
	}, nil
}

func matchStandaloneIsDataAt(c *matchContext, buffer []byte, raw string) error {
	parts := strings.Split(raw, ",")
	if len(parts) != 1 {
		return errors.New("standalone isdataat does not support relative")
	}
	value := strings.TrimSpace(parts[0])
	negative := strings.HasPrefix(value, "!")
	value = strings.TrimPrefix(value, "!")
	position, err := strconv.Atoi(value)
	if err != nil || position < 0 {
		return errors.Wrap(err, "isdataat format error")
	}
	c.Must(negIf(negative, position < len(buffer)))
	return nil
}

func attachPayloadMatcher(c *matchContext) error {
	contentRules := c.Rule.ContentRuleConfig.ContentRules
	if len(contentRules) == 0 {
		if bsize, ok := c.Rule.SettingMap["bsize"]; ok {
			contentRules = append(contentRules, &rule.ContentRule{BSize: bsize})
		}
		if dsize, ok := c.Rule.SettingMap["dsize"]; ok {
			if len(contentRules) == 0 {
				contentRules = append(contentRules, &rule.ContentRule{DSize: dsize})
			} else {
				contentRules[0].DSize = dsize
			}
		}
	}
	for _, r := range contentRules {
		matcher, err := newPayloadMatcher(r, r.Modifier)
		if err != nil {
			return err
		}
		c.Attach(matcher)
	}
	return nil
}

func attachFastPattern(c *matchContext) error {
	idx := slices.IndexFunc(c.Rule.ContentRuleConfig.ContentRules, func(rule *rule.ContentRule) bool {
		return rule.FastPattern && !rule.Negative
	})
	if idx == -1 {
		return nil
	}

	fastPatternRule := c.Rule.ContentRuleConfig.ContentRules[idx]
	fastPatternRule = fastPatternCopy(fastPatternRule)
	if fastPatternRule.Modifier == modifier.FileData {
		c.Attach(newFileDataMatcher(fastPatternRule))
		return nil
	}
	matcher, err := newPayloadMatcher(fastPatternRule, fastPatternRule.Modifier)
	if err != nil {
		return err
	}
	c.Attach(func(c *matchContext) error {
		prevMatched, prevModifier := c.prevMatched, c.prevModifier
		defer func() {
			c.prevMatched, c.prevModifier = prevMatched, prevModifier
		}()
		return matcher(c)
	})
	return nil
}
