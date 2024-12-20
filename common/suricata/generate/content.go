package generate

import (
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/bytemap"
	"github.com/yaklang/yaklang/common/suricata/rule"
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

	additionModifiers []ByteMapModifier
	postHandler       func([]byte) []byte

	// try to trim the content to tryLen
	// 0 -> the field is not valid
	tryLen int

	noLeftTrim  bool
	noRightTrim bool
}

func (c *ContentGen) Gen() []byte {
	if c.Len == 0 {
		return nil
	}
	if c.Len >= 1<<20 {
		log.Warnf("content length too large, generator aborted, plz check rules")
		return nil
	}
	bm := bytemap.NewByteMap(c.Len)
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

	if c.postHandler != nil {
		return c.postHandler(bm.Bytes())
	}
	return bm.Bytes()
}

func parse2ContentGen(rules []*rule.ContentRule, opts ...ContentGenOpt) *ContentGen {
	mdf := &ContentGen{
		noise: noiseAll,
	}

	for _, opt := range opts {
		opt(mdf)
	}

	for _, r := range rules {
		var cm *ContentModifier
		switch {
		case r.Negative:
			// ignore
		case r.PCRE != "":
			// PCRE
			generator, err := r.PCREParsed.Generator()
			if err != nil {
				log.Warnf("new regexp generator from rule failed:%v", err)
			}
			mdf.Modifiers = append(mdf.Modifiers,
				&RegexpModifier{generator},
			)
		case r.StartsWith:
			cm = &ContentModifier{
				NoCase:  r.Nocase,
				Content: r.Content,
				Offset:  0,
			}
			mdf.noLeftTrim = true
		case r.EndsWith:
			if r.Depth != nil {
				cm = &ContentModifier{
					NoCase:  r.Nocase,
					Content: r.Content,
					Offset:  *r.Depth - len(r.Content),
				}
			} else {
				cm = &ContentModifier{
					NoCase:  r.Nocase,
					Content: r.Content,
					Offset:  -len(r.Content),
				}
			}
			mdf.noRightTrim = true
		case r.Within == nil && r.Distance == nil && r.Depth == nil && r.Offset == nil:
			cm = &ContentModifier{
				NoCase:  r.Nocase,
				Content: r.Content,
				Range:   math.MaxInt,
			}
		default:
			cm = &ContentModifier{
				NoCase:   r.Nocase,
				Relative: r.Distance != nil || r.Within != nil,
				Content:  r.Content,
			}

			cm.Range = math.MaxInt

			// absolute offset
			if r.Offset != nil {
				cm.Offset = *r.Offset
				mdf.noLeftTrim = true
			}
			if r.Depth != nil {
				cm.Range = *r.Depth - len(r.Content)
				mdf.noLeftTrim = true
			}

			// relative offset
			if r.Distance != nil {
				cm.Offset = *r.Distance
			}
			if r.Within != nil {
				cm.Range = *r.Within - len(r.Content) - cm.Offset
			}
		}

		if r.IsDataAt != "" {
			var neg bool
			var relative bool
			var pos int
			strs := strings.Split(r.IsDataAt, ",")
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
						log.Warnf("parse isdataat modifier:" + r.IsDataAt)
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
				cm.Filter = func(free []int, payload *bytemap.ByteMap, cm *ContentModifier) []int {
					var res []int
					for _, v := range free {
						if neg && mdf.Len <= v+pos+len(cm.Content)-1 || !neg && mdf.Len > v+pos+len(cm.Content)-1 {
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

		// try fix
		if cm.Range < 0 {
			cm.Range = 0
		}

		mdf.Modifiers = append(mdf.Modifiers, cm)
	}

	mdf.Modifiers = append(mdf.Modifiers, mdf.additionModifiers...)
	mdf.setLen()
	return mdf
}

func (c *ContentGen) setLen() {
	for _, m := range c.Modifiers {
		switch m := m.(type) {
		case *ContentModifier:
			if m.Offset >= 0 {
				if m.Offset+len(m.Content) > c.Len || m.Relative {
					c.Len += m.Offset + len(m.Content)
				}
			} else {
				if -m.Offset > c.Len || m.Relative {
					c.Len = -m.Offset
				}
			}
		case *RegexpModifier:
			c.Len += len(m.Generator.Generate())
		}
	}
	if c.Len > 2 {
		c.Len = 1 << (math.Ilogb(float64(c.Len-1)) + 1)
	}
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

func WithPostHandler(f func([]byte) []byte) ContentGenOpt {
	return func(gen *ContentGen) {
		gen.postHandler = f
	}
}

func WithPostModifier(mdf ByteMapModifier) ContentGenOpt {
	return func(gen *ContentGen) {
		gen.additionModifiers = append(gen.additionModifiers, mdf)
	}
}
