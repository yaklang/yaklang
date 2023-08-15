package suricata

import (
	"github.com/yaklang/yaklang/common/log"
	"math"
	"strconv"
	"strings"
)

type Generator interface {
	Gen() []byte
}

type Surigen struct {
	rules []*ContentRule
	gen   map[Modifier]Generator
}

func NewSurigen(contentRules []*ContentRule) (*Surigen, error) {
	g := &Surigen{
		rules: contentRules,
		gen:   make(map[Modifier]Generator),
	}
	err := g.parse()
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Surigen) Gen() ([]byte, error) {
	mp := make(map[Modifier][]byte)
	for k, gener := range g.gen {
		mp[k] = gener.Gen()
	}
	return HTTPCombination(mp), nil
}

func (g *Surigen) parse() error {
	// parse rules
	for _, rule := range g.rules {
		// special part use special generator
		switch rule.Modifier {
		case HTTPStatCode:
			// do something
			continue
		default:
			g.parse2ContentGen(rule)
		}
	}

	// set Len
	for _, payload := range g.gen {
		payload, ok := payload.(*ContentGen)
		if !ok {
			continue
		}
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
		payload.Len = 1 << math.Ilogb(float64(payload.Len+1))
	}

	return nil
}

func (g *Surigen) parse2ContentGen(rule *ContentRule) {
	var mdf *ContentGen
	if g.gen[rule.Modifier] == nil {
		mdf = &ContentGen{}
	} else {
		mdf = g.gen[rule.Modifier].(*ContentGen)
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
		return
	}

	mdf.Modifiers = append(mdf.Modifiers, cm)
	g.gen[rule.Modifier] = mdf
}
