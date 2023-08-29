package generate

import (
	"encoding/hex"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"strconv"
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
	var toServer, toClient bool
	if g.r.ContentRuleConfig.Flow != nil {
		toServer = g.r.ContentRuleConfig.Flow.ToServer
		toClient = g.r.ContentRuleConfig.Flow.ToClient
	}

	if !toServer && !toClient {
		toServer = true
		toClient = true
	}

	tcpConfig := g.r.ContentRuleConfig.TcpConfig
	if g.r.ContentRuleConfig.TcpConfig == nil {
		tcpConfig = &rule.TCPLayerRule{}
	}

	var (
		seq = 1000 + rand.Intn(5553)
		ack = 1000 + rand.Intn(5553)
	)

	if tcpConfig.Ack != nil && *tcpConfig.Ack > 0 {
		ack = *tcpConfig.Ack
	}

	if tcpConfig.Seq != nil && *tcpConfig.Seq > 0 {
		seq = *tcpConfig.Seq
	}

	var tcpLayer = &layers.TCP{
		Seq: uint32(seq),
		Ack: uint32(ack),
	}

	var tcpMss uint32
	switch tcpConfig.TCPMssOp {
	case 1:
		tcpMss = uint32(tcpConfig.TCPMssNum1)
	case 2:
		tcpMss = uint32(tcpConfig.TCPMssNum1 + rand.Intn(200))
	case 3:
		tcpMss = uint32(rand.Intn(tcpConfig.TCPMssNum1))
	case 4:
		tcpMss = uint32(tcpConfig.TCPMssNum1 + rand.Intn(tcpConfig.TCPMssNum2-tcpConfig.TCPMssNum1))
	default:
		tcpMss = 0x05b4
	}

	if tcpMss > 0xffff {
		tcpMss = 0xffff
	}

	if tcpMss > 0 {
		bytes, _ := hex.DecodeString(strconv.FormatInt(int64(tcpMss), 16))
		if len(bytes) > 0 {
			tcpLayer.Options = append(tcpLayer.Options, layers.TCPOption{
				OptionType: layers.TCPOptionKindMSS,
				OptionData: bytes,
			})
		}
	}

	if tcpConfig.Flags != "" && !strings.HasPrefix(tcpConfig.Flags, "!") {
		for _, flag := range tcpConfig.Flags {
			switch flag {
			case 'S':
				tcpLayer.SYN = true
			case 'F':
				tcpLayer.FIN = true
			case 'R':
				tcpLayer.RST = true
			case 'P':
				tcpLayer.PSH = true
			case 'A':
				tcpLayer.ACK = true
			case 'U':
				tcpLayer.URG = true
			case 'E':
				tcpLayer.ECE = true
			case 'C':
				tcpLayer.CWR = true
			}
		}
	}

	if tcpConfig.Window != nil && !tcpConfig.NegativeWindow && *tcpConfig.Window > 0 {
		tcpLayer.Window = uint16(*tcpConfig.Window)
	}

	// 定义IPv4报文头部
	ipLayer := &layers.IPv4{
		Version:  4,                    // 版本号
		TTL:      64,                   // 生存时间
		Protocol: layers.IPProtocolTCP, // 协议类型
	}

	// IP 层
	ipLayer.SrcIP = net.ParseIP(utils.GetLocalIPAddress())
	if ipLayer.SrcIP == nil {
		log.Error("fetch local ip address failed")
		return nil
	}
	ipLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())
	tcpLayer.SrcPort = layers.TCPPort(uint16(g.r.SourcePort.GetAvailablePort()))
	tcpLayer.DstPort = layers.TCPPort(uint16(g.r.DestinationPort.GetAvailablePort()))
	if toClient && !toServer {
		ipLayer.SrcIP, ipLayer.DstIP = ipLayer.DstIP, ipLayer.SrcIP
		tcpLayer.SrcPort, tcpLayer.DstPort = tcpLayer.DstPort, tcpLayer.SrcPort
	}

	dstPort := uint16(g.r.DestinationPort.GetAvailablePort())
	srcPort := uint16(g.r.SourcePort.GetAvailablePort())
	tcpLayer.SrcPort = layers.TCPPort(srcPort)
	tcpLayer.DstPort = layers.TCPPort(dstPort)

	_ = tcpLayer.SetNetworkLayerForChecksum(ipLayer)

	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, ipLayer, tcpLayer, gopacket.Payload(g.payload.Gen()))
	if err != nil {
		log.Errorf("serialize layers failed: %s", err)
		return nil
	}
	return buffer.Bytes()
}
