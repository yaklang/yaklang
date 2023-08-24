package generate

import (
	"bytes"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var _ Generator = (*HTTPGen)(nil)

type HTTPGen struct {
	rules []*rule.ContentRule
	gen   map[modifier.Modifier]ModifierGenerator
}

func newHTTPGen(r *rule.Rule) (*HTTPGen, error) {
	g := &HTTPGen{
		rules: r.ContentRuleConfig.ContentRules,
		gen:   make(map[modifier.Modifier]ModifierGenerator),
	}

	// parse rules
	for mdf, r := range contentRuleMap(g.rules) {
		// special part use special generator
		// designed but not in using tempetarily
		switch mdf {
		case modifier.HTTPStatCode:
			g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseDigit), WithTryLen(3))
		case modifier.HTTPRequestBody, modifier.HTTPResponseBody:
			g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseAll))
		case modifier.HTTPContentLen:
			g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseDigit))
		case modifier.HTTPMethod:
			g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseChar), WithTryLen(3))
		case modifier.HTTPHeaderNames:
			g.gen[mdf] = parse2DirectGen(r)
		case modifier.HTTPHeader, modifier.HTTPHeaderRaw:
			g.gen[mdf] = parse2HeaderGen(r)
		case modifier.HTTPRequestLine, modifier.HTTPResponseLine, modifier.HTTPStart:
			if len(bytes.Fields(r[0].Content)) == 3 {
				g.gen[mdf] = parse2DirectGen(r)
			} else {
				g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseVisable))
			}
		default:
			g.gen[mdf] = parse2ContentGen(r, WithNoise(noiseVisable))
		}
	}

	return g, nil
}

func (g *HTTPGen) Gen() []byte {
	mp := make(map[modifier.Modifier][]byte)
	for k, gener := range g.gen {
		mp[k] = gener.Gen()
	}
	return lowhttp.FixHTTPPacketCRLF(HTTPCombination(mp), false)
}
