package generate

import (
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"net"
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
	var toServer bool
	var toClient bool

	if g.r.ContentRuleConfig.Flow != nil {
		toServer = g.r.ContentRuleConfig.Flow.ToServer
		toClient = g.r.ContentRuleConfig.Flow.ToClient
	} else {
		toServer = true
		toClient = true
	}

	var udpLayer = &layers.UDP{}

	// 定义IPv4报文头部
	ipLayer := &layers.IPv4{
		Version:  4,                    // 版本号
		TTL:      64,                   // 生存时间
		Protocol: layers.IPProtocolUDP, // 协议类型
	}

	// IP 层
	ipLayer.SrcIP = net.ParseIP(utils.GetLocalIPAddress())
	if ipLayer.SrcIP == nil {
		log.Error("fetch local ip address failed")
		return nil
	}
	ipLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())
	udpLayer.SrcPort = layers.UDPPort(uint16(g.r.SourcePort.GetAvailablePort()))
	udpLayer.DstPort = layers.UDPPort(uint16(g.r.DestinationPort.GetAvailablePort()))
	if toClient && !toServer {
		ipLayer.SrcIP, ipLayer.DstIP = ipLayer.DstIP, ipLayer.SrcIP
		udpLayer.SrcPort, udpLayer.DstPort = udpLayer.DstPort, udpLayer.SrcPort
	}

	_ = udpLayer.SetNetworkLayerForChecksum(ipLayer)

	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}, ipLayer, udpLayer, gopacket.Payload(g.payload.Gen()))
	if err != nil {
		log.Error(err)
		return nil
	}
	return buffer.Bytes()
}
