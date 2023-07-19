package chaosmaker

import (
	"encoding/hex"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"strconv"
	"strings"
)

func init() {
	chaosMap.Store("suricata-tcp", &tcpHandler{})
}

type tcpHandler struct {
}

var _ chaosHandler = (*tcpHandler)(nil)

func (t *tcpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *suricata.Rule) chan *ChaosTraffic {
	if rule.Protocol != "tcp" {
		return nil
	}

	if rule.ContentRuleConfig == nil {
		return nil
	}

	if rule.ContentRuleConfig.TcpConfig == nil {
		return nil
	}
	tcpConfig := rule.ContentRuleConfig.TcpConfig
	var toServer, toClient bool
	if rule.ContentRuleConfig.Flow != nil {
		toServer = rule.ContentRuleConfig.Flow.ToServer
		toClient = rule.ContentRuleConfig.Flow.ToClient
	}

	if !toServer && !toClient {
		toServer = true
		toClient = true
	}

	var (
		seq = 1000 + rand.Intn(5553)
		ack = 1000 + rand.Intn(5553)
	)

	if tcpConfig.Ack > 0 {
		ack = tcpConfig.Ack
	}

	if tcpConfig.Seq > 0 {
		seq = tcpConfig.Seq
	}

	var baseTCPLayer = &layers.TCP{
		Seq: uint32(seq),
		Ack: uint32(ack),
	}

	if tcpConfig.TCPMss != "" {
		// a-b
		// >a <a
		var tcpMss uint32
		if mss := tcpConfig.TCPMss; strings.Contains(mss, "-") {
			min, max, _ := splitRangeString(tcpConfig.TCPMss)
			if max <= 0 || min > max {
				tcpMss = 0x05b4
			} else {
				tcpMss = uint32(min + rand.Intn(max-min))
			}
		} else if strings.HasPrefix(mss, ">") {
			var min, _ = strconv.Atoi(strings.Trim(mss, ">"))
			if min <= 0 {
				tcpMss = 0x05b4
			} else {
				tcpMss = uint32(min + rand.Intn(200))
				if tcpMss > 0xffff {
					tcpMss = 0xffff
				}
			}
		} else if strings.HasPrefix(mss, "<") {
			var max, _ = strconv.Atoi(strings.Trim(mss, "<"))
			if max <= 0 {
				tcpMss = 0x05b4
			} else {
				tcpMss = uint32(max - rand.Intn(max))
				if tcpMss > 0xffff {
					tcpMss = 0xffff
				}
			}
		} else {
			tcpMssInt, _ := strconv.Atoi(mss)
			if tcpMssInt > 0 {
				tcpMss = uint32(tcpMssInt)
			}
		}

		if tcpMss > 0 {
			bytes, _ := hex.DecodeString(strconv.FormatInt(int64(tcpMss), 16))
			if len(bytes) > 0 {
				baseTCPLayer.Options = append(baseTCPLayer.Options, layers.TCPOption{
					OptionType: layers.TCPOptionKindMSS,
					OptionData: bytes,
				})
			}
		}
	}

	if tcpConfig.Flags != "" && !strings.HasPrefix(tcpConfig.Flags, "!") {
		for _, flag := range tcpConfig.Flags {
			switch flag {
			case 'S':
				baseTCPLayer.SYN = true
			case 'F':
				baseTCPLayer.FIN = true
			case 'R':
				baseTCPLayer.RST = true
			case 'P':
				baseTCPLayer.PSH = true
			case 'A':
				baseTCPLayer.ACK = true
			case 'U':
				baseTCPLayer.URG = true
			case 'E':
				baseTCPLayer.ECE = true
			case 'C':
				baseTCPLayer.CWR = true
			}
		}
	}

	if !tcpConfig.NegativeWindow && tcpConfig.Window > 0 {
		baseTCPLayer.Window = uint16(tcpConfig.Window)
	}

	// 定义IPv4报文头部
	baseIPLayer := &layers.IPv4{
		Version:  4,                    // 版本号
		TTL:      64,                   // 生存时间
		Protocol: layers.IPProtocolTCP, // 协议类型
	}

	toBytes := func(ipLayer *layers.IPv4, tcpLayer *layers.TCP, payloads ...gopacket.Payload) []byte {
		var actPayloads []byte
		if len(payloads) > 0 {
			for _, p := range payloads {
				actPayloads = append(actPayloads, p...)
			}
		}

		tcpLayer.SetNetworkLayerForChecksum(ipLayer)

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{
			FixLengths:       true,
			ComputeChecksums: true,
		}, ipLayer, tcpLayer, gopacket.Payload(actPayloads))
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
		ch <- TCPIPBytesToChaosTraffic(makerRule, rule, raw)
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
			baseTCPLayer.SrcPort = layers.TCPPort(srcPort)
			baseTCPLayer.DstPort = layers.TCPPort(dstPort)

			for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
				feedback(toBytes(baseIPLayer, baseTCPLayer, gopacket.Payload(payloads)))
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
			baseTCPLayer.SrcPort = layers.TCPPort(srcPort)
			baseTCPLayer.DstPort = layers.TCPPort(dstPort)

			for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
				feedback(toBytes(baseIPLayer, baseTCPLayer, gopacket.Payload(payloads)))
			}
		}
	}()
	return ch
}

func (t *tcpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}

func splitRangeString(input string) (int, int, error) {
	// 使用 strings.Split 函数将字符串按照 "-" 分割成两部分
	splitStr := strings.Split(input, "-")

	// 检查分割后的字符串长度是否正确
	if len(splitStr) != 2 {
		return 0, 0, fmt.Errorf("Invalid input format: %s", input)
	}

	// 使用 strconv.Atoi 函数将字符串转换为整数
	minimum, err := strconv.Atoi(splitStr[0])
	if err != nil {
		return 0, 0, fmt.Errorf("Invalid minimum value: %s", splitStr[0])
	}

	maximum, err := strconv.Atoi(splitStr[1])
	if err != nil {
		return 0, 0, fmt.Errorf("Invalid maximum value: %s", splitStr[1])
	}

	return minimum, maximum, nil
}

func TCPIPBytesToChaosTraffic(makerRule *rule.Storage, r *suricata.Rule, raw []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:    makerRule,
		SuricataRule: r,
		TCPIPPayload: raw,
	}
}
