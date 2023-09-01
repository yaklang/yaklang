package generate

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"math/rand"
	"strings"
)

var _ Generator = (*TCPGen)(nil)

type TCPGen struct {
	r       *rule.Rule
	payload ModifierGenerator
}

func newTCPGen(r *rule.Rule) (Generator, error) {
	if r.ContentRuleConfig == nil {
		return nil, errors.New("empty content rule config")
	}

	g := &TCPGen{
		r: r,
	}

	for mdf, rr := range contentRuleMap(r.ContentRuleConfig.ContentRules) {
		switch mdf {
		case modifier.TCPHDR:
			// 暂时不太想支持，和其他tcpconfig冲突比较严重,用的也不多
			log.Warnf("tcp.hdr modifier won't support in tcp generator")
		case modifier.Default:
			g.payload = parse2ContentGen(rr, WithNoise(noiseAll))
		default:
			// There are someone using http modifier in tcp...
			if modifier.IsHTTPModifier(mdf) {
				gen, err := newHTTPGen(r)
				if err != nil {
					return nil, errors.Wrap(err, "new http gen failed")
				}
				g.payload = gen
				return g, nil
			}
			log.Warnf("not support modifier %v", mdf)
		}
	}

	if g.payload == nil {
		g.payload = &ContentGen{
			Len:   32,
			noise: noiseAll,
		}
	}

	return g, nil
}

func (g *TCPGen) Gen() []byte {
	var opts []any
	tcpConfig := g.r.ContentRuleConfig.TcpConfig
	if g.r.ContentRuleConfig.TcpConfig == nil {
		tcpConfig = &rule.TCPLayerRule{}
	}

	if tcpConfig.Ack != nil && *tcpConfig.Ack > 0 {
		opts = append(opts, pcapx.WithTCP_Ack(*tcpConfig.Ack))
	} else {
		opts = append(opts, pcapx.WithTCP_Ack(uint32(rand.Uint32())))
	}

	if tcpConfig.Seq != nil && *tcpConfig.Seq > 0 {
		opts = append(opts, pcapx.WithTCP_Seq(*tcpConfig.Seq))
	} else {
		opts = append(opts, pcapx.WithTCP_Seq(uint32(rand.Uint32())))
	}

	if tcpConfig.TCPMss != nil {
		opts = append(opts, pcapx.WithTCP_OptionMSS(uint32(tcpConfig.TCPMss.Generate())))
	}

	var Len = len(opts)
	if tcpConfig.Flags != "" && !strings.HasPrefix(tcpConfig.Flags, "!") {
		for _, flag := range tcpConfig.Flags {
			switch flag {
			case 'S':
				opts = append(opts, pcapx.WithTCP_Flags("syn"))
			case 'F':
				opts = append(opts, pcapx.WithTCP_Flags("fin"))
			case 'R':
				opts = append(opts, pcapx.WithTCP_Flags("rst"))
			case 'P':
				opts = append(opts, pcapx.WithTCP_Flags("psh"))
			case 'A':
				opts = append(opts, pcapx.WithTCP_Flags("ack"))
			case 'U':
				opts = append(opts, pcapx.WithTCP_Flags("urg"))
			case 'E':
				opts = append(opts, pcapx.WithTCP_Flags("ece"))
			case 'C':
				opts = append(opts, pcapx.WithTCP_Flags("cwr"))
			}
		}
	}
	// syn by default
	if len(opts) == Len {
		opts = append(opts, pcapx.WithTCP_Flags("ack"))
	}

	if tcpConfig.Window != nil && !tcpConfig.NegativeWindow && *tcpConfig.Window > 0 {
		opts = append(opts, pcapx.WithTCP_Window(uint16(*tcpConfig.Window)))
	} else {
		opts = append(opts, pcapx.WithTCP_Window(uint16(rand.Intn(2048))))
	}

	opts = append(opts, pcapx.WithIPv4_SrcIP(g.r.SourceAddress.Generate()))
	opts = append(opts, pcapx.WithIPv4_DstIP(g.r.DestinationAddress.Generate()))
	opts = append(opts, pcapx.WithTCP_SrcPort(g.r.SourcePort.GetAvailablePort()))
	opts = append(opts, pcapx.WithTCP_DstPort(g.r.DestinationPort.GetAvailablePort()))
	opts = append(opts, pcapx.WithPayload(g.payload.Gen()))

	raw, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		log.Errorf("generate tcp packet failed: %s", err)
		return nil
	}
	return raw
}
