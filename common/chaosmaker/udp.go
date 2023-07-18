package chaosmaker

import (
	"encoding/binary"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata"
	"github.com/yaklang/yaklang/common/utils"
	"net"
)

func init() {
	chaosMap.Store("suricata-udp", &udpHandler{})
}

type udpHandler struct {
}

func (h *udpHandler) Generator(maker *ChaosMaker, makerRule *ChaosMakerRule, rule *suricata.Rule) chan *ChaosTraffic {
	if rule.Protocol != "udp" {
		return nil
	}

		if rule.ContentRuleConfig == nil {
			return nil
		}

	if rule.ContentRuleConfig.UdpConfig == nil {
			log.Errorf("[BUG]: not prepared udp config from: %v", rule.Raw)
			return nil
		}
	var toServer bool
	var toClient bool

		if rule.ContentRuleConfig.Flow != nil {
			toServer = rule.ContentRuleConfig.Flow.ToServer
			toClient = rule.ContentRuleConfig.Flow.ToClient
		} else {
			toServer = true
			toClient = true
		}

		var isInUdpHeader = rule.ContentRuleConfig.UdpConfig.UDPHeader

		var baseUDPLayer = &layers.UDP{}

		// 定义IPv4报文头部
		baseIPLayer := &layers.IPv4{
			Version:  4,                    // 版本号
			TTL:      64,                   // 生存时间
			Protocol: layers.IPProtocolUDP, // 协议类型
		}

		toBytes := func(ipLayer *layers.IPv4, udpLayer *layers.UDP, payloads ...gopacket.Payload) []byte {
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
			}, ipLayer, udpLayer, gopacket.Payload(actPayloads))
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
			if toClient {
				ch <- UDPIPInboundBytesToChaosTraffic(makerRule, rule, raw)
			}

			if toServer {
				ch <- UDPIPOutboundBytesToChaosTraffic(makerRule, rule, raw)
			}

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

			if isInUdpHeader {
				baseUDPLayer.Length = binary.BigEndian.Uint16([]byte(payloads))
			}

			if toServer {
				// IP 层
				baseIPLayer.SrcIP = net.ParseIP(maker.LocalIPAddress)
				if baseIPLayer.SrcIP == nil {
					log.Error("fetch local ip address failed")
					return
				}
				baseIPLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())

				// 表示这是对外发送的数据包
				dstPort := uint16(rule.DestinationPort.GetAvailablePort())
				srcPort := uint16(rule.SourcePort.GetHighPort())
				baseUDPLayer.SrcPort = layers.UDPPort(srcPort)
				baseUDPLayer.DstPort = layers.UDPPort(dstPort)

				for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
					if isInUdpHeader {
						feedback(toBytes(baseIPLayer, baseUDPLayer))
						continue
					}
					feedback(toBytes(baseIPLayer, baseUDPLayer, gopacket.Payload(payloads)))
				}
			}

			if toClient {
				baseIPLayer.DstIP = net.ParseIP(maker.LocalIPAddress)
				if baseIPLayer.DstIP == nil {
					log.Error("fetch local ip address failed")
					return
				}
				baseIPLayer.SrcIP = net.ParseIP(utils.GetRandomIPAddress())

				// 这是主机接收到的包
				dstPort := uint16(rule.DestinationPort.GetAvailablePort())
				srcPort := uint16(rule.SourcePort.GetHighPort())
				baseUDPLayer.SrcPort = layers.UDPPort(srcPort)
				baseUDPLayer.DstPort = layers.UDPPort(dstPort)

				for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
					if isInUdpHeader {
						feedback(toBytes(baseIPLayer, baseUDPLayer))
						continue
					}
					feedback(toBytes(baseIPLayer, baseUDPLayer, gopacket.Payload(payloads)))
				}
			}
		}()
		return ch
	}


func (h *udpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}

func UDPIPInboundBytesToChaosTraffic(makerRule *ChaosMakerRule, r *suricata.Rule, raw []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:           makerRule,
		SuricataRule:        r,
		UDPIPInboundPayload: raw,
	}
}

func UDPIPOutboundBytesToChaosTraffic(makerRule *ChaosMakerRule, r *suricata.Rule, raw []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:            makerRule,
		SuricataRule:         r,
		UDPIPOutboundPayload: raw,
	}
}
