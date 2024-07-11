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

	if nmac, ok := s.macTable.Load(host); ok {
		dstMac = nmac.(net.HardwareAddr)
	}

	if s.config.RemoteMac == nil {
		if !utils.IsLoopback(host) {
			err := s.getGatewayMac()
			if err != nil {
				return nil, err
			}
			// Ethernet
			opts = append(opts, pcapx.WithEthernet_DstMac(s.config.Iface.HardwareAddr))
			opts = append(opts, pcapx.WithEthernet_SrcMac(dstMac))
		} else {
			loopback = true
			opts = append(opts, pcapx.WithLoopback(loopback))
		}
	} else {
		opts = append(opts,
			pcapx.WithEthernet_DstMac(dstMac),
			pcapx.WithEthernet_SrcMac(s.config.Iface.HardwareAddr),
		)
	}

	var ipSrc string
	if loopback {
		ipSrc = net.ParseIP("127.0.0.1").String()
	} else {
		ipSrc = s.config.SourceIP.String()
	}

	// IPv4
	opts = append(opts, pcapx.WithIPv4_Flags(layers.IPv4DontFragment))
	opts = append(opts, pcapx.WithIPv4_Version(4))
	opts = append(opts, pcapx.WithIPv4_NextProtocol(layers.IPProtocolTCP))
	opts = append(opts, pcapx.WithIPv4_TTL(64))
	opts = append(opts, pcapx.WithIPv4_ID(40000+rand.Intn(10000)))
	opts = append(opts, pcapx.WithIPv4_SrcIP(ipSrc))
	opts = append(opts, pcapx.WithIPv4_DstIP(host))
	opts = append(opts, pcapx.WithIPv4_Option(nil, nil))

	opts = append(opts,
		pcapx.WithTCP_SrcPort(rand.Intn(65534)+1),
		pcapx.WithTCP_DstPort(port),
		pcapx.WithTCP_Flags(pcapx.TCP_FLAG_SYN),
		pcapx.WithTCP_Window(1024),
		pcapx.WithTCP_Options(layers.TCPOptionKindMSS, []byte{5, 0xb4}),
		pcapx.WithTCP_Seq(500000+rand.Intn(10000)),
	)

	packetBytes, err = pcapx.PacketBuilder(opts...)
	if err != nil {
		return nil, err
	}
	return packetBytes, nil
}
