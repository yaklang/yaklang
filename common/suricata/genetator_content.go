package suricata

import (
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

type noiseFunc func() byte

func noiseAll() byte {
	return byte(rand.Intn(256))
}

func noiseChar() byte {
	tmp := rand.Intn(52)
	if tmp < 26 {
		return byte(tmp + 'A')
	}
	return byte(tmp - 26 + 'a')
}

func noiseDigit() byte {
	return byte(rand.Intn(10) + '0')
}

func noiseVisable() byte {
	return byte(rand.Intn(95) + 32)
}

func noiseDigitChar() byte {
	tmp := rand.Intn(62)
	if tmp < 10 {
		return byte(tmp + '0')
	}
	if tmp < 36 {
		return byte(tmp - 10 + 'A')
	}
	return byte(tmp - 36 + 'a')
}

type ContentGen struct {
	Modifiers []ByteMapModifier
	Len       int
	noise     noiseFunc

	// try to trim the content to tryLen
	// 0 -> the field is not valid
	tryLen int

	noLeftTrim  bool
	noRightTrim bool
}

func (c *ContentGen) Gen() []byte {
	if c.Len >= 1<<20 {
		log.Warnf("content length too large, generator aborted, plz check rules")
		return nil
	}
	bm := NewByteMap(c.Len)
	for i := 0; i < len(c.Modifiers); i++ {
		err := c.Modifiers[i].Modify(bm)
		if err != nil {
			if errors.Is(err, ErrOverFlow) {
				c.Len <<= 1
				return c.Gen()
			} else {
				return nil
			}
		}
	}
	if c.tryLen != 0 {
		bm.Trim(!c.noLeftTrim, !c.noRightTrim, c.tryLen)
	}
	bm.FillLeftWithNoise(c.noise)
	return bm.Bytes()
}

func parse2ContentGen(rules []*ContentRule, opts ...ContentGenOpt) *ContentGen {
	mdf := &ContentGen{
		noise: noiseAll,
	}

	for _, opt := range opts {
		opt(mdf)
	}

	for _, rule := range rules {
		var cm *ContentModifier
		switch {
		case rule.Negative:
			// ignore
		case rule.PCRE != "":
			// PCRE
			pcre, err := ParsePCREStr(rule.PCRE)
			if err != nil {
				log.Warnf("parse pcre rule failed:%v", err)
			}
			generator, err := pcre.Generator()
			if err != nil {
				log.Warnf("new regexp generator from rule failed:%v", err)
			}
			mdf.Modifiers = append(mdf.Modifiers,
				&RegexpModifier{generator},
			)
		case rule.StartsWith:
			cm = &ContentModifier{
				NoCase:  rule.Nocase,
				Content: rule.Content,
				Offset:  0,
			}
			mdf.noLeftTrim = true
		case rule.EndsWith:
			if rule.Depth != nil {
				cm = &ContentModifier{
					NoCase:  rule.Nocase,
					Content: rule.Content,
					Offset:  *rule.Depth - len(rule.Content),
				}
			} else {
				cm = &ContentModifier{
					NoCase:  rule.Nocase,
					Content: rule.Content,
					Offset:  -len(rule.Content),
				}
			}
			mdf.noRightTrim = true
		case rule.Within == nil && rule.Distance == nil && rule.Depth == nil && rule.Offset == nil:
			cm = &ContentModifier{
				NoCase:  rule.Nocase,
				Content: rule.Content,
				Range:   math.MaxInt,
			}
		default:
			cm := &ContentModifier{
				NoCase:   rule.Nocase,
				Relative: rule.Distance != nil || rule.Within != nil,
				Content:  rule.Content,
			}

			cm.Range = math.MaxInt

			// absolute offset
			if rule.Offset != nil {
				cm.Offset = *rule.Offset
				mdf.noLeftTrim = true
			}
			if rule.Depth != nil {
				cm.Range = *rule.Depth - len(rule.Content)
				mdf.noLeftTrim = true
			}

			// relative offset
			if rule.Distance != nil {
				cm.Offset = *rule.Distance
			}
			if rule.Within != nil {
				cm.Range = *rule.Within - len(rule.Content) - cm.Offset
			}
		}

		if rule.IsDataAt != "" {
			var neg bool
			var relative bool
			var pos int
			strs := strings.Split(rule.IsDataAt, ",")
			for _, str := range strs {
				if strings.Contains(str, "relative") {
					relative = true
				} else {
					str = strings.TrimSpace(str)
					if strings.HasPrefix(str, "!") {
						neg = true
					}
					str = strings.Trim(str, "!")
					v, err := strconv.Atoi(str)
					if err != nil {
						log.Warnf("parse isdataat modifier:" + rule.IsDataAt)
					}
					pos = v
				}
			}
			if !relative {
				if neg && mdf.Len > pos {
					mdf.Len = pos
				} else if !neg && mdf.Len <= pos {
					mdf.Len <<= 1
				}
			} else {
				cm.Filter = func(free []int, payload *ByteMap, cm *ContentModifier) []int {
					var res []int
					for _, v := range free {
						if neg && mdf.Len <= v+pos || !neg && mdf.Len > v+pos {
							res = append(res, v)
						}
					}
					return res
				}
			}
		}

		if cm == nil {
			continue
		}

		mdf.Modifiers = append(mdf.Modifiers, cm)
	}

	return mdf
}

type ContentGenOpt func(*ContentGen)

func WithNoise(noise noiseFunc) ContentGenOpt {
	return func(gen *ContentGen) {
		gen.noise = noise
	}
}

func WithTryLen(Len int) ContentGenOpt {
	return func(gen *ContentGen) {
		gen.tryLen = Len
	}
}
