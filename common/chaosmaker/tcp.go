package chaosmaker

import (
	"encoding/hex"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/suricata/generate"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
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

func (t *tcpHandler) Generator(maker *ChaosMaker, makerRule *rule.Storage, rule *surirule.Rule) chan *pcapx.ChaosTraffic {
	if rule.Protocol != "tcp" {
		return nil
	}

	if rule.ContentRuleConfig == nil {
		return nil
	}

	tcpConfig := rule.ContentRuleConfig.TcpConfig
	if rule.ContentRuleConfig.TcpConfig == nil {
		tcpConfig = &surirule.TCPLayerRule{}
	}

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

	if tcpConfig.Ack != nil && *tcpConfig.Ack > 0 {
		ack = *tcpConfig.Ack
	}

	if tcpConfig.Seq != nil && *tcpConfig.Seq > 0 {
		seq = *tcpConfig.Seq
	}

	var baseTCPLayer = &layers.TCP{
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
			baseTCPLayer.Options = append(baseTCPLayer.Options, layers.TCPOption{
				OptionType: layers.TCPOptionKindMSS,
				OptionData: bytes,
			})
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

	if tcpConfig.Window != nil && !tcpConfig.NegativeWindow && *tcpConfig.Window > 0 {
		baseTCPLayer.Window = uint16(*tcpConfig.Window)
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

	ch := make(chan *pcapx.ChaosTraffic)
	feedback := func(raw []byte) {
		if raw == nil {
			return
		}
		ch <- TCPIPBytesToChaosTraffic(makerRule, rule, raw)
	}
	go func() {
		defer close(ch)

		ploadgen, err := generate.NewRulegen(rule)
		if err != nil {
			log.Errorf("create ploadgen failed: %s", err)
			return
		}

		// IP 层
		if toServer {
			baseIPLayer.SrcIP = net.ParseIP(maker.LocalIPAddress)
			if baseIPLayer.SrcIP == nil {
				log.Error("fetch local ip address failed")
				return
			}
			baseIPLayer.DstIP = net.ParseIP(utils.GetRandomIPAddress())
		} else if toClient {
			baseIPLayer.DstIP = net.ParseIP(maker.LocalIPAddress)
			if baseIPLayer.DstIP == nil {
				log.Error("fetch local ip address failed")
				return
			}
			baseIPLayer.SrcIP = net.ParseIP(utils.GetRandomIPAddress())
		}

		dstPort := uint16(rule.DestinationPort.GetAvailablePort())
		srcPort := uint16(rule.SourcePort.GetHighPort())
		baseTCPLayer.SrcPort = layers.TCPPort(srcPort)
		baseTCPLayer.DstPort = layers.TCPPort(dstPort)

		for i := 0; i < rule.ContentRuleConfig.Thresholding.Repeat(); i++ {
			feedback(toBytes(baseIPLayer, baseTCPLayer, ploadgen.Gen()))
		}
	}()
	return ch
}

func (t *tcpHandler) MatchBytes(i interface{}) bool {
	//TODO implement me
	panic("implement me")
}

func TCPIPBytesToChaosTraffic(makerRule *rule.Storage, r *surirule.Rule, raw []byte) *pcapx.ChaosTraffic {
	return &pcapx.ChaosTraffic{
		TCPIPPayload: raw,
	}
}
