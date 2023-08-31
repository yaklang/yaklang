package generate

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

var _ Generator = (*ICMPGen)(nil)

type ICMPGen struct {
	r       *rule.Rule
	payload ModifierGenerator
}

func newICMPGen(r *rule.Rule) (Generator, error) {
	if r.ContentRuleConfig == nil {
		return nil, errors.New("empty content rule config")
	}

	g := &ICMPGen{
		r: r,
	}

	for mdf, rr := range contentRuleMap(r.ContentRuleConfig.ContentRules) {
		switch mdf {
		case modifier.Default:
			g.payload = parse2ContentGen(rr, WithNoise(noiseAll))
		case modifier.ICMPV4HDR:
			log.Warnf("icmpv4hdr modifier won't support in icmp generator")
		case modifier.ICMPV6HDR:
			log.Warnf("icmpv6hdr modifier won't support in icmp generator")
		default:
			log.Warnf("not support modifier %v", mdf)
		}
	}

	if g.payload == nil {
		g.payload = &DirectGen{
			payload: []byte("abcdefghijklmnopqrstuvwabcdefghi"),
		}
	}

	return g, nil
}

func (g *ICMPGen) Gen() []byte {
	var icode int
	var itype int

	var opts []any

	opts = append(opts,
		pcapx.WithIPv4_SrcIP(g.r.SourceAddress.Generate()),
		pcapx.WithIPv4_DstIP(g.r.DestinationAddress.Generate()),
	)

	if g.r.ContentRuleConfig.IcmpConfig.ICMPId != nil {
		opts = append(opts, pcapx.WithICMP_Id(uint16(*g.r.ContentRuleConfig.IcmpConfig.ICMPId)))
	}

	if g.r.ContentRuleConfig.IcmpConfig.ICMPSeq != nil {
		opts = append(opts, pcapx.WithICMP_Sequence(uint16(*g.r.ContentRuleConfig.IcmpConfig.ICMPSeq)))
	}

	if g.r.ContentRuleConfig.IcmpConfig.IType != nil {
		itype = g.r.ContentRuleConfig.IcmpConfig.IType.Generate()
	}
	if g.r.ContentRuleConfig.IcmpConfig.ICode != nil {
		icode = g.r.ContentRuleConfig.IcmpConfig.ICode.Generate()
	}
	opts = append(opts, pcapx.WithICMP_Type(uint8(itype), uint8(icode)))

	opts = append(opts, pcapx.WithPayload(g.payload.Gen()))

	pk, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		log.Errorf("generate icmp packet failed: %s", err)
		return nil
	}
	return pk
}
