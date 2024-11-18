package finscan

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
)

var loopbackIP net.IP

func init() {
	loopbackIP = net.ParseIP("127.0.0.1")
}

type TcpFlag string

const (
	SYN TcpFlag = "syn"
	FIN TcpFlag = "fin"
	RST TcpFlag = "rst"
	ACK TcpFlag = "ack"
)

// dstMac 为空的话，会尝试自动去取一个
func (s *Scanner) createTCPWithDstMac(dstIp net.IP, dstPort int, dstMac net.HardwareAddr) (_ []gopacket.SerializableLayer, loopback bool, _ error) {
	var baseLayer gopacket.SerializableLayer
	var err error

	if dstMac == nil {
		if !utils.IsLoopback(dstIp.String()) {
			baseLayer, err = s.getDefaultCacheEthernet(dstIp.String(), dstPort)
			loopback = false
			if baseLayer == nil {
				return nil, loopback, errors.Errorf("can't get default ethernet: %v", err)
			}
		} else {
			baseLayer = s.getLoopbackLinkLayer()
			loopback = true
		}
	} else {
		baseLayer = &layers.Ethernet{
			SrcMAC:       s.iface.HardwareAddr,
			DstMAC:       dstMac,
			EthernetType: layers.EthernetTypeIPv4,
		}
	}

	ip4 := layers.IPv4{
		Version:  4,
		TTL:      255,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    s.defaultSrcIp,
		DstIP:    dstIp,
	}
	if loopback {
		ip4.SrcIP = loopbackIP
	}
	tcp := layers.TCP{
		SrcPort: layers.TCPPort(rand.Intn(65534) + 1),
		DstPort: layers.TCPPort(dstPort),
		Window:  1024,
		//Options: []layers.TCPOption{
		//	{
		//		OptionType:   layers.TCPOptionKindMSS,
		//		OptionLength: 4,
		//		OptionData:   []byte{5, 0xb4},
		//	},
		//},
	}
	s.config.TcpSetter(&tcp)
	if tcp.RST {
		tcp.SYN = false
		tcp.Window = 0
		tcp.Options = nil
	}
	err = tcp.SetNetworkLayerForChecksum(&ip4)
	if err != nil {
		return nil, loopback, errors.Errorf("ip4 set network layer checksum failed: %s", err)
	}
	log.Debug("packet all set")
	return []gopacket.SerializableLayer{
		baseLayer, &ip4, &tcp,
	}, loopback, nil
}
