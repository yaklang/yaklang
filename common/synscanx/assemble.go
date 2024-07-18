package synscanx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
)

func (s *Scannerx) assembleSynPacket(host string, port int) ([]byte, error) {
	var packetBytes []byte
	var err error
	var opts []any
	var loopback bool

	dstMac := s.config.RemoteMac
	srcMac := s.config.SourceMac
	if mac, ok := s.macCacheTable.Load(host); ok {
		dstMac = mac.(net.HardwareAddr)
	}

	if s.config.RemoteMac == nil {
		if !utils.IsLoopback(host) {
			dstMac, err = s.getGatewayMac()
			if err != nil {
				return nil, utils.Errorf("get gateway mac failed: %s", err)
			}
			// Ethernet
			opts = append(opts, pcapx.WithEthernet_SrcMac(srcMac))
			opts = append(opts, pcapx.WithEthernet_DstMac(dstMac))
		} else {
			// Loopback
			loopback = true
			opts = append(opts, pcapx.WithLoopback(loopback))
		}
	} else {
		opts = append(opts,
			pcapx.WithEthernet_SrcMac(srcMac),
			pcapx.WithEthernet_DstMac(dstMac),
		)
	}

	var ipSrc string
	if loopback {
		ipSrc = net.ParseIP("127.0.0.1").String()
		host = ipSrc
	} else {
		ipSrc = s.config.SourceIP.String()
	}
	//srcPort := rand.Intn(65534) + 1
	srcPort := 12345
	// IPv4
	opts = append(opts, pcapx.WithIPv4_Flags(layers.IPv4DontFragment))
	opts = append(opts, pcapx.WithIPv4_Version(4))
	opts = append(opts, pcapx.WithIPv4_NextProtocol(layers.IPProtocolTCP))
	opts = append(opts, pcapx.WithIPv4_TTL(64))
	opts = append(opts, pcapx.WithIPv4_ID(40000+rand.Intn(10000)))
	opts = append(opts, pcapx.WithIPv4_SrcIP(ipSrc))
	opts = append(opts, pcapx.WithIPv4_DstIP(host))
	opts = append(opts, pcapx.WithIPv4_Option(nil, nil))

	// TCP
	opts = append(opts,
		pcapx.WithTCP_SrcPort(srcPort),
		pcapx.WithTCP_DstPort(port),
		pcapx.WithTCP_Flags(pcapx.TCP_FLAG_SYN),
		pcapx.WithTCP_Window(1024),
		pcapx.WithTCP_Options(layers.TCPOptionKindMSS, []byte{5, 0xb4}),
		pcapx.WithTCP_Seq(500000+rand.Intn(10000)),
	)

	packetBytes, err = pcapx.PacketBuilder(opts...)
	if err != nil {
		return nil, utils.Wrapf(err, "assembleSynPacket failed")
	}
	return packetBytes, nil
}

func (s *Scannerx) assembleArpPacket(host string) ([]byte, error) {
	var opts []any
	srcMac := s.config.SourceMac.String()
	srcIP := s.config.SourceIP.String()
	opts = append(opts, pcapx.WithEthernet_SrcMac(srcMac))
	opts = append(opts, pcapx.WithEthernet_DstMac(net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}))
	opts = append(opts, pcapx.WithEthernet_NextLayerType(layers.EthernetTypeARP))

	opts = append(opts, pcapx.WithArp_RequestEx(srcIP, srcMac, host))

	packetBytes, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		return nil, err
	}
	return packetBytes, nil

	//srcMac := s.config.SourceMac
	//srcIP := s.config.SourceIP
	//eth := &layers.Ethernet{
	//	SrcMAC:       srcMac,
	//	DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	//	EthernetType: layers.EthernetTypeARP,
	//}
	//arp := &layers.ARP{
	//	AddrType:          layers.LinkTypeEthernet,
	//	Protocol:          layers.EthernetTypeIPv4,
	//	HwAddressSize:     6,
	//	ProtAddressSize:   4,
	//	Operation:         layers.ARPRequest,
	//	SourceHwAddress:   srcMac,
	//	SourceProtAddress: []byte(srcIP.To4()),
	//	DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
	//	DstProtAddress:    []byte(net.ParseIP(host).To4()),
	//}
	//
	//var packetBytes = gopacket.NewSerializeBuffer()
	//err := gopacket.SerializeLayers(
	//	packetBytes,
	//	gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
	//	eth, arp,
	//)
	//if err != nil {
	//	return nil, utils.Wrapf(err, "assembleArpPacket failed")
	//}
	//
	//return packetBytes.Bytes(), nil
}
