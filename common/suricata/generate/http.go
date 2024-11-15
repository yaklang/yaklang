package generate

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"math/rand"
)

var _ Generator = (*HTTPGen)(nil)

type HTTPGen struct {
	rules   []*rule.ContentRule
	srcAddr *rule.AddressRule
	dstAddr *rule.AddressRule
	srcPort *rule.PortRule
	dstPort *rule.PortRule
	gen     map[modifier.Modifier]ModifierGenerator
}

func newHTTPGen(r *rule.Rule) (Generator, error) {
	if r.ContentRuleConfig == nil {
		return nil, errors.New("empty content rule config")
	}

	g := &HTTPGen{
		rules:   r.ContentRuleConfig.ContentRules,
		gen:     make(map[modifier.Modifier]ModifierGenerator),
		srcPort: r.SourcePort,
		dstPort: r.DestinationPort,
		srcAddr: r.SourceAddress,
		dstAddr: r.DestinationAddress,
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
	var opts []any

	mp := make(map[modifier.Modifier][]byte)
	for k, gener := range g.gen {
		mp[k] = gener.Gen()
	}

	payload := lowhttp.FixHTTPPacketCRLF(HTTPCombination(mp), false)
	opts = append(opts, pcapx.WithPayload(payload))

	opts = append(opts, pcapx.WithIPv4_SrcIP(g.srcAddr.Generate()))
	opts = append(opts, pcapx.WithIPv4_DstIP(g.dstAddr.Generate()))

	if lowhttp.IsResp(payload) {
		opts = append(opts, pcapx.WithTCP_SrcPort(uint16(g.srcPort.GenerateWithDefault(80))))
		opts = append(opts, pcapx.WithTCP_DstPort(uint16(g.dstPort.GetAvailablePort())))
	} else {
		opts = append(opts, pcapx.WithTCP_SrcPort(uint16(g.srcPort.GetAvailablePort())))
		opts = append(opts, pcapx.WithTCP_DstPort(uint16(g.dstPort.GenerateWithDefault(80))))
	}

	opts = append(opts,
		pcapx.WithTCP_Window(rand.Intn(2048)),
		pcapx.WithTCP_Flags("ack|psh"),
		pcapx.WithTCP_Ack(rand.Uint32()),
		pcapx.WithTCP_Seq(rand.Uint32()),
	)

	raw, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		log.Errorf("generate http packet failed: %s", err)
		return nil
	}
	return raw
}
