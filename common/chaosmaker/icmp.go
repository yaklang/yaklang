package chaosmaker

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"strconv"
	"strings"
)

func init() {
	chaosMap.Store("suricata-icmp", &icmpHandler{})
}

type icmpHandler struct {
}

var _ chaosHandler = (*icmpHandler)(nil)

func (h *icmpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *suricata.Rule) chan *pcapx.ChaosTraffic {
	if rule.Protocol != "icmp" {
		return nil
	}

	if rule.ContentRuleConfig == nil {
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

	var baseICMPLayer = &layers.ICMPv4{}

	if IcmpConfig := rule.ContentRuleConfig.IcmpConfig; IcmpConfig != nil {
		var code, typeCode uint8
		if IcmpConfig.ICode != "" {
			code = uint8(parseCondition(IcmpConfig.ICode))
		}
		if IcmpConfig.IType != "" {
			typeCode = uint8(parseCondition(IcmpConfig.IType))
		}
		baseICMPLayer.TypeCode = layers.CreateICMPv4TypeCode(code, typeCode)
		baseICMPLayer.Id = uint16(IcmpConfig.ICMPId)
		baseICMPLayer.Seq = uint16(IcmpConfig.ICMPSeq)
	}

	// 定义IPv4报文头部
	baseIPLayer := &layers.IPv4{
		Version:  4,                       // 版本号
		TTL:      64,                      // 生存时间
		Protocol: layers.IPProtocolICMPv4, // 协议类型
	}

	toBytes := func(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, payloads ...gopacket.Payload) []byte {
		var actPayloads []byte
		if len(payloads) > 0 {
			for _, p := range payloads {
				actPayloads = append(actPayloads, p...)
			}
		}

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}, ipLayer, icmpLayer, gopacket.Payload(actPayloads))
		if err != nil {
			log.Error(err)
			return nil
		}
		return buffer.Bytes()
	}

	ch := make(chan *pcapx.ChaosTraffic)
	feedback := func(raw []byte) {
		if raw == nil {
			return
		}
		if toClient {
			ch <- ICMPIPInboundBytesToChaosTraffic(makerRule, rule, raw)
		}

		if toServer {
			ch <- ICMPIPOutboundBytesToChaosTraffic(makerRule, rule, raw)
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
			extraRules = append(extraRules, r.PCREStringGenerator(1)...)
		}

		for _, r := range extraRules {
			payloads += string(r.Content)
		}

		if toServer {
			// IP 层
			baseIPLayer.SrcIP = net.ParseIP(maker.LocalIPAddress)
			if baseIPLayer.SrcIP == nil {
				log.Error("fetch local ip address failed")
				return
			}
			baseIPLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())

			for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
				feedback(toBytes(baseIPLayer, baseICMPLayer, gopacket.Payload(payloads)))
			}
		}

		if toClient {
			baseIPLayer.DstIP = net.ParseIP(maker.LocalIPAddress)
			if baseIPLayer.DstIP == nil {
				log.Error("fetch local ip address failed")
				return
			}
			baseIPLayer.SrcIP = net.ParseIP(utils.GetRandomIPAddress())

			for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
				feedback(toBytes(baseIPLayer, baseICMPLayer, gopacket.Payload(payloads)))
			}
		}
	}()
	return ch
}

func (h *icmpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}

func parseCondition(condition string) int {
	var min, max, fixed int
	var err error

	switch {
	case strings.Contains(condition, "<>"):
		parts := strings.Split(condition, "<>")
		min, err = strconv.Atoi(parts[0])
		max, err = strconv.Atoi(parts[1])
		if err != nil || min >= max {
			log.Warn("ICMP规则<>`左右两侧数值不符合要求")
			return 0
		}
		return min + rand.Intn(max-min)
	case strings.Contains(condition, ">"):
		min, err = strconv.Atoi(condition[1:])
		if err != nil {
			log.Warn("ICMP规则`>`右侧数值不符合要求")
			return 0
		}
		return min + 1
	case strings.Contains(condition, "<"):
		min, err = strconv.Atoi(condition[1:])
		if err != nil {
			log.Warn("ICMP规则`<`右侧数值不符合要求")
			return 0
		}
		return min - 1
	default:
		fixed, err = strconv.Atoi(condition)
		if err != nil {
			log.Warn("ICMP规则格式错误")
			return 0
		}
		return fixed
	}
}

func ICMPIPInboundBytesToChaosTraffic(makerRule *rule.Storage, r *suricata.Rule, raw []byte) *pcapx.ChaosTraffic {
	return &pcapx.ChaosTraffic{
		ICMPIPInboundPayload: raw,
	}
}

func ICMPIPOutboundBytesToChaosTraffic(makerRule *rule.Storage, r *suricata.Rule, raw []byte) *pcapx.ChaosTraffic {
	return &pcapx.ChaosTraffic{
		ICMPIPOutboundPayload: raw,
	}
}
