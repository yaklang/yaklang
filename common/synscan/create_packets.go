package synscan

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
)

var loopbackIP net.IP

func init() {
	loopbackIP = net.ParseIP("127.0.0.1")
}

// dstMac 为空的话，会尝试自动去取一个
//func (s *Scanner) createTCPWithDstMac(dstIp net.IP, dstPort int, syn bool, rst bool, dstMac net.HardwareAddr, gateway string) (_ []gopacket.SerializableLayer, loopback bool, _ error) {
//	var baseLayer gopacket.SerializableLayer
//	var err error
//	if dstMac == nil {
//		if !utils.IsLoopback(dstIp.String()) {
//			baseLayer, err = s.getDefaultCacheEthernet(dstIp.String(), dstPort, gateway)
//			if err != nil {
//				return nil, false, err
//			}
//		} else {
//			baseLayer = s.getLoopbackLinkLayer()
//			loopback = true
//		}
//	} else {
//		baseLayer = &layers.Ethernet{
//			SrcMAC:       s.iface.HardwareAddr,
//			DstMAC:       dstMac,
//			EthernetType: layers.EthernetTypeIPv4,
//		}
//	}
//
//	ip4 := layers.IPv4{
//		Version:  4,
//		TTL:      255,
//		Protocol: layers.IPProtocolTCP,
//		SrcIP:    s.defaultSrcIp,
//		DstIP:    dstIp,
//	}
//	if loopback {
//		ip4.SrcIP = loopbackIP
//	}
//	tcp := layers.TCP{
//		SrcPort: layers.TCPPort(rand.Intn(65534) + 1),
//		DstPort: layers.TCPPort(dstPort),
//		SYN:     syn,
//		RST:     rst,
//		Window:  1024,
//		Options: []layers.TCPOption{
//			{
//				OptionType:   layers.TCPOptionKindMSS,
//				OptionLength: 4,
//				OptionData:   []byte{5, 0xb4},
//			},
//		},
//	}
//	if tcp.RST {
//		tcp.SYN = false
//		tcp.Window = 0
//		tcp.Options = nil
//	}
//	err = tcp.SetNetworkLayerForChecksum(&ip4)
//	if err != nil {
//		return nil, loopback, errors.Errorf("ip4 set network layer checksum failed: %s", err)
//	}
//
//	if baseLayer == nil {
//		baseLayer = &layers.Loopback{
//			Family: layers.ProtocolFamilyIPv4,
//		}
//	}
//	return []gopacket.SerializableLayer{
//		baseLayer, &ip4, &tcp,
//	}, loopback, nil
//}

func (s *Scanner) createTCPWithDstMac(dstIp net.IP, dstPort int, syn bool, rst bool, dstMac net.HardwareAddr, gateway string) (_ []byte, loopback bool, _ error) {
	var packetBytes []byte
	var err error
	var opts []any

	if dstMac == nil {
		if !utils.IsLoopback(dstIp.String()) {
			_, err = s.getDefaultCacheEthernet(dstIp.String(), dstPort, gateway)
			if err != nil {
				return nil, false, err
			}
			// Ethernet
			opts = append(opts, pcapx.WithEthernet_DstMac(s.iface.HardwareAddr))
			opts = append(opts, pcapx.WithEthernet_SrcMac(s.defaultDstHw))

		} else {
			loopback = true
			opts = append(opts, pcapx.WithLoopback(loopback))
		}
	} else {
		opts = append(opts,
			pcapx.WithEthernet_DstMac(dstMac),
			pcapx.WithEthernet_SrcMac(s.iface.HardwareAddr),
		)
	}

	var ipSrc string
	if loopback {
		ipSrc = loopbackIP.String()
	} else {
		ipSrc = s.defaultSrcIp.String()
	}

	// IPv4
	opts = append(opts, pcapx.WithIPv4_ID(40000+rand.Intn(10000)))
	opts = append(opts, pcapx.WithIPv4_Flags(layers.IPv4DontFragment))
	opts = append(opts, pcapx.WithIPv4_Version(4))
	opts = append(opts, pcapx.WithIPv4_NextProtocol(layers.IPProtocolTCP))
	opts = append(opts, pcapx.WithIPv4_TTL(128))
	opts = append(opts, pcapx.WithIPv4_ID(40000+rand.Intn(10000)))
	opts = append(opts, pcapx.WithIPv4_SrcIP(ipSrc))
	opts = append(opts, pcapx.WithIPv4_DstIP(dstIp.String()))
	opts = append(opts, pcapx.WithIPv4_Option(nil, nil))

	if rst {
		opts = append(opts,
			pcapx.WithTCP_SrcPort(rand.Intn(65534)+1),
			pcapx.WithTCP_DstPort(dstPort),
			pcapx.WithTCP_Flags(pcapx.TCP_FLAG_RST),
			pcapx.WithTCP_Options(nil, nil),
		)
	}
	if syn {
		opts = append(opts,
			pcapx.WithTCP_SrcPort(rand.Intn(65534)+1),
			pcapx.WithTCP_DstPort(dstPort),
			pcapx.WithTCP_Flags(pcapx.TCP_FLAG_SYN),
			pcapx.WithTCP_Window(1024),
			pcapx.WithTCP_Options(layers.TCPOptionKindMSS, []byte{5, 0xb4}),
			pcapx.WithTCP_Seq(500000+rand.Intn(10000)),
		)
	}

	packetBytes, err = pcapx.PacketBuilder(opts...)
	if err != nil {
		return nil, loopback, err
	}

	return packetBytes, loopback, nil
}

func (s *Scanner) createSynTCP(dstIp net.IP, dstPort int, dstMac net.HardwareAddr, gateway string) ([]byte, bool, error) {
	return s.createTCPWithDstMac(dstIp, dstPort, true, false, dstMac, gateway)
}

func (s *Scanner) createRstTCP(dstIp net.IP, dstPort int, dstMac net.HardwareAddr, gateway string) ([]byte, bool, error) {
	return s.createTCPWithDstMac(dstIp, dstPort, false, true, dstMac, gateway)
}
