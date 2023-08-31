package generate

import (
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"math/rand"
)

var _ Generator = (*DNSGen)(nil)

type DNSGen struct {
	srcIP   rule.AddressRule
	dstIP   rule.AddressRule
	srcPort rule.PortRule
	dstPost rule.PortRule
	opcode  *rule.DNSRule
	query   ModifierGenerator
}

func newDNSGen(r *rule.Rule) (Generator, error) {
	if r.ContentRuleConfig == nil {
		return nil, errors.New("empty content rule config")
	}

	var g = new(DNSGen)

	for mdf, rr := range contentRuleMap(r.ContentRuleConfig.ContentRules) {
		switch mdf {
		case modifier.DNSQuery:
			g.query = parse2ContentGen(rr, WithNoise(noiseDigitChar))
		case modifier.Default:
			// won't support
			log.Warnf("default modifier won't support in dns")
		default:
			log.Warnf("not support modifier %v", mdf)
		}
	}

	g.opcode = r.ContentRuleConfig.DNS

	if r.SourceAddress == nil {
		g.srcIP = rule.AddressRule{
			Any: true,
		}
	} else {
		g.srcIP = *r.SourceAddress
	}

	if r.DestinationAddress == nil {
		g.dstIP = rule.AddressRule{
			Any: true,
		}
	} else {
		g.dstIP = *r.DestinationAddress
	}

	if r.SourcePort == nil {
		g.srcPort = rule.PortRule{
			Any: true,
		}
	} else {
		g.srcPort = *r.SourcePort
	}

	if r.DestinationPort == nil {
		g.dstPost = rule.PortRule{
			Any: true,
		}
	} else {
		g.dstPost = *r.DestinationPort
	}

	return g, nil
}

func (g *DNSGen) Gen() []byte {
	opcode := 0
	if g.opcode != nil {
		if g.opcode.OpcodeNegative {
			possiable := []int{0, 1, 2}
			if g.opcode.Opcode >= 0 && g.opcode.Opcode <= 2 {
				possiable[2], possiable[g.opcode.Opcode] = possiable[g.opcode.Opcode], possiable[2]
				possiable = possiable[:2]
			}
			opcode = possiable[rand.Intn(2)]
		} else {
			opcode = g.opcode.Opcode
		}
	}

	dnsLayer := &layers.DNS{
		ID:     uint16(rand.Intn(65535)),
		OpCode: layers.DNSOpCode(opcode),
		Questions: []layers.DNSQuestion{
			{
				Name:  g.query.Gen(),
				Type:  layers.DNSTypeA,
				Class: layers.DNSClassAny,
			},
		},
		QDCount: 1,
		RD:      true,
	}

	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, dnsLayer)
	if err != nil {
		log.Errorf("serialize layers failed: %s", err)
		return nil
	}

	var opts []any
	opts = append(opts, pcapx.WithIPv4_SrcIP(g.srcIP.Generate()))
	opts = append(opts, pcapx.WithIPv4_DstIP(g.dstIP.Generate()))
	opts = append(opts, pcapx.WithUDP_SrcPort(uint16(g.srcPort.GetAvailablePort())))
	opts = append(opts, pcapx.WithUDP_DstPort(uint16(g.dstPost.GenerateWithDefault(53))))
	opts = append(opts, pcapx.WithPayload(buffer.Bytes()))

	raw, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		log.Errorf("generate dns packet failed: %s", err)
		return nil
	}
	return raw
}
