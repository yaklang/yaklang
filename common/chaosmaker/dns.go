package chaosmaker

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"net"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/suricata"
	"yaklang.io/yaklang/common/utils"
)

var dnsHandler = &chaosHandler{
	Generator: func(maker *ChaosMaker, makerRule *ChaosMakerRule, rule *suricata.Rule) chan *ChaosTraffic {
		if rule.Protocol != "dns" {
			return nil
		}

		if rule.ContentRuleConfig == nil {
			return nil
		}

		dnsRule := rule.ContentRuleConfig.DNS

		var baseUDPLayer = &layers.UDP{}
		var baseDNSLayer = &layers.DNS{}

		// 定义IPv4报文头部
		baseIPLayer := &layers.IPv4{
			Version:  4,                    // 版本号
			TTL:      64,                   // 生存时间
			Protocol: layers.IPProtocolUDP, // 协议类型
		}

		baseDNSLayer.OpCode = layers.DNSOpCode(dnsRule.Opcode)

		baseDNSLayer.QR = !dnsRule.DNSQuery

		if dnsRule.OpcodeNegative && baseDNSLayer.OpCode == layers.DNSOpCode(dnsRule.Opcode) {
			log.Warn("DNS 规则可能存在错误")
		} else if !dnsRule.OpcodeNegative && baseDNSLayer.OpCode != layers.DNSOpCode(dnsRule.Opcode) {
			log.Warn("DNS 规则可能存在错误")
		}

		toBytes := func(ipLayer *layers.IPv4, udpLayer *layers.UDP, dnsLayer *layers.DNS, payloads ...gopacket.Payload) []byte {
			var actPayloads []byte
			if len(payloads) > 0 {
				for _, p := range payloads {
					actPayloads = append(actPayloads, p...)
				}
			}

			udpLayer.SetNetworkLayerForChecksum(ipLayer)

			buffer := gopacket.NewSerializeBuffer()
			err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}, ipLayer, udpLayer, dnsLayer, gopacket.Payload(actPayloads))
			if err != nil {
				log.Error(err)
				return nil
			}
			return buffer.Bytes()
		}

		ch := make(chan *ChaosTraffic)
		feedback := func(raw []byte) {
			if raw == nil {
				return
			}
			ch <- DNSIPBytesToChaosTraffic(makerRule, rule, raw)
		}
		go func() {
			defer close(ch)

			var payloads string
			var extraRules []*suricata.ContentRule
			for _, r := range rule.ContentRuleConfig.ContentRules {
				if r.Negative {
					continue
				}
				payloads += string(r.Content)
				if r.PCRE == "" {
					continue
				}
				extraRules = append(extraRules, r.PCREStringGenerator(2)...)
			}

			for _, r := range extraRules {
				payloads += string(r.Content)
			}

			baseIPLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())

			baseIPLayer.SrcIP = net.ParseIP(maker.LocalIPAddress)

			if baseIPLayer.SrcIP == nil {
				log.Error("fetch local ip address failed")
				return
			}

			var dstPort uint16
			// 这是主机接收到的包
			if rule.DestinationPort.Any {
				dstPort = 53
			} else {
				dstPort = uint16(rule.DestinationPort.GetAvailablePort())
			}
			srcPort := uint16(rule.SourcePort.GetHighPort())
			baseUDPLayer.SrcPort = layers.UDPPort(srcPort)
			baseUDPLayer.DstPort = layers.UDPPort(dstPort)

			for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
				dnsQuestion := layers.DNSQuestion{
					Name:  []byte(payloads),  // 查询名称
					Type:  layers.DNSTypeA,   // 查询类型，A表示查询IPv4地址
					Class: layers.DNSClassIN, // 查询类别，表示Internet
				}
				baseDNSLayer.Questions = []layers.DNSQuestion{dnsQuestion}
				feedback(toBytes(baseIPLayer, baseUDPLayer, baseDNSLayer))
			}

		}()
		return ch
	},
	MatchBytes: nil,
}

func init() {
	chaosMap.Store("suricata-dns", dnsHandler)
}

func DNSIPBytesToChaosTraffic(makerRule *ChaosMakerRule, r *suricata.Rule, raw []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:            makerRule,
		SuricataRule:         r,
		UDPIPOutboundPayload: raw,
	}
}
