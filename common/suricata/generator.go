package suricata

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"math"
	"strconv"
	"strings"
)

type PayloadGen struct {
	Modifiers []ByteMapModifier
	Len       int
}

type Surigen struct {
	rules []*ContentRule
	gen   map[Modifier]*PayloadGen
}

func NewSurigen(contentRules []*ContentRule) (*Surigen, error) {
	g := &Surigen{
		rules: contentRules,
		gen:   make(map[Modifier]*PayloadGen),
	}
	err := g.parse()
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Surigen) Gen() (map[Modifier][]byte, error) {
	output := make(map[Modifier][]byte)
	for k, payload := range g.gen {
		bm := NewByteMap(payload.Len)
		for i := 0; i < len(payload.Modifiers); i++ {
			err := payload.Modifiers[i].Modify(bm)
			if err != nil {
				if errors.Is(err, ErrOverFlow) {
					payload.Len <<= 1
				} else {
					return output, errors.Wrap(err, "failed to modify payload")
				}
			}
		}
		bm.FillLeftWithNoise()
		output[k] = bm.Bytes()
	}
	return output, nil
}

func (g *Surigen) parse() error {
	for _, rule := range g.rules {
		if g.gen[rule.Modifier] == nil {
			g.gen[rule.Modifier] = &PayloadGen{}
		}
		var cm *ContentModifier
		switch {
		case rule.Negative:
			// ignore
		case rule.PCRE != "":
			// PCRE
			pcre, err := ParsePCREStr(rule.PCRE)
			if err != nil {
				log.Warnf("parse pcre rule failed:%v", err)
				continue
			}
			generator, err := pcre.Generator()
			if err != nil {
				log.Warnf("new regexp generator from rule failed:%v", err)
			}
			g.gen[generator.modifier].Modifiers = append(g.gen[generator.modifier].Modifiers,
				&RegexpModifier{generator},
			)
		case rule.StartsWith:
			cm = &ContentModifier{
				NoCase:  rule.Nocase,
				Content: rule.Content,
				Offset:  0,
			}
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
			}
			if rule.Depth != nil {
				cm.Range = *rule.Depth - len(rule.Content)
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
						return errors.Wrap(err, "parse isdataat modifier:"+rule.IsDataAt)
					}
					pos = v
				}
			}
			if !relative {
				if neg && g.gen[rule.Modifier].Len > pos {
					g.gen[rule.Modifier].Len = pos
				} else if !neg && g.gen[rule.Modifier].Len <= pos {
					g.gen[rule.Modifier].Len <<= 1
				}
			} else {
				cm.Filter = func(free []int, payload *ByteMap, cm *ContentModifier) []int {
					var res []int
					for _, v := range free {
						if neg && g.gen[rule.Modifier].Len <= v+pos || !neg && g.gen[rule.Modifier].Len > v+pos {
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
		g.gen[rule.Modifier].Modifiers = append(g.gen[rule.Modifier].Modifiers, cm)
	}

	// Set Len
	for _, payload := range g.gen {
		for _, m := range payload.Modifiers {
			switch m := m.(type) {
			case *ContentModifier:
				if m.Offset+len(m.Content) > payload.Len || m.Relative {
					payload.Len += m.Offset + len(m.Content)
				}
			case *RegexpModifier:
				payload.Len += len(m.Generator.Generate())
			}
		}
		payload.Len = 1 << (math.Ilogb(float64(payload.Len+1)) + 1)
	}

	return nil
}
