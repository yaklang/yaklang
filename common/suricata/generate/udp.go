package generate

import (
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

var _ Generator = (*UDPGen)(nil)

type UDPGen struct {
	r       *rule.Rule
	payload ModifierGenerator
}

func newUDPGen(r *rule.Rule) (Generator, error) {
	if r.ContentRuleConfig == nil {
		return nil, errors.New("empty content rule config")
	}

	g := &UDPGen{
		r: r,
	}

	for mdf, r := range contentRuleMap(r.ContentRuleConfig.ContentRules) {
		switch mdf {
		case modifier.UDPHDR:
			// 暂不支持
		case modifier.Default:
			g.payload = parse2ContentGen(r, WithNoise(noiseAll))
		default:
			log.Warnf("not support modifier %v", mdf)
		}
	}
	return g, nil
}

func (g *UDPGen) Gen() []byte {
	var opts []any
	opts = append(opts, pcapx.WithIPv4_SrcIP(g.r.SourceAddress.Generate()))
	opts = append(opts, pcapx.WithIPv4_DstIP(g.r.DestinationAddress.Generate()))
	opts = append(opts, pcapx.WithUDP_SrcPort(uint16(g.r.SourcePort.GetAvailablePort())))
	opts = append(opts, pcapx.WithUDP_DstPort(uint16(g.r.DestinationPort.GetAvailablePort())))
	opts = append(opts, pcapx.WithPayload(g.payload.Gen()))

	raw, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		log.Errorf("generate udp packet failed: %s", err)
		return nil
	}
	return raw
}
