package suricata

import (
	"bytes"
	"math"
)

type Generator interface {
	Gen() []byte
}

type Ploadgen struct {
	rules []*ContentRule
	gen   map[Modifier]Generator
}

func NewPloadgen(contentRules []*ContentRule) (*Ploadgen, error) {
	g := &Ploadgen{
		rules: contentRules,
		gen:   make(map[Modifier]Generator),
	}
	err := g.parse()
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Ploadgen) Gen() ([]byte, error) {
	mp := make(map[Modifier][]byte)
	for k, gener := range g.gen {
		mp[k] = gener.Gen()
	}
	return HTTPCombination(mp), nil
}

func (g *Ploadgen) parse() error {
	// mapping by Modifier
	var mp = make(map[Modifier][]*ContentRule)
	for _, rule := range g.rules {
		mp[rule.Modifier] = append(mp[rule.Modifier], rule)
	}

	// parse rules
	for mdf, rule := range mp {
		// special part use special generator
		// designed but not in using tempetarily
		switch mdf {
		case HTTPStatCode:
			g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseDigit), WithTryLen(3))
		case HTTPRequestBody, HTTPResponseBody:
			g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseAll))
		case HTTPContentLen:
			g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseDigit))
		case HTTPMethod:
			g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseChar), WithTryLen(3))
		case HTTPHeaderNames:
			g.gen[mdf] = parse2DirectGen(rule)
		case HTTPRequestLine, HTTPResponseLine, HTTPStart:
			if len(bytes.Fields(rule[0].Content)) == 3 {
				g.gen[mdf] = parse2DirectGen(rule)
			} else {
				g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseVisable))
			}
		default:
			g.gen[mdf] = parse2ContentGen(rule, WithNoise(noiseVisable))
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
				if m.Offset >= 0 {
					if m.Offset+len(m.Content) > payload.Len || m.Relative {
						payload.Len += m.Offset + len(m.Content)
					}
				} else {
					if -m.Offset > payload.Len || m.Relative {
						payload.Len = -m.Offset
					}
				}
			case *RegexpModifier:
				payload.Len += len(m.Generator.Generate())
			}
		}
		payload.Len = 1 << (math.Ilogb(float64(payload.Len+1)) + 1)
	}

	return nil
}
