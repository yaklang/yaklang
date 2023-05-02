package synscan

import (
	"math/rand"
	"net"
	"time"
	"yaklang.io/yaklang/common/utils"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
)

var loopbackIP net.IP

func init() {
	rand.Seed(time.Now().UnixNano())
	loopbackIP = net.ParseIP("127.0.0.1")
}

// dstMac 为空的话，会尝试自动去取一个
func (s *Scanner) createTCPWithDstMac(dstIp net.IP, dstPort int, syn bool, rst bool, dstMac net.HardwareAddr, gateway string) (_ []gopacket.SerializableLayer, loopback bool, _ error) {
	var baseLayer gopacket.SerializableLayer
	var err error

	if dstMac == nil {
		if !utils.IsLoopback(dstIp.String()) {
			baseLayer, err = s.getDefaultCacheEthernet(dstIp.String(), dstPort, gateway)
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
		SYN:     syn,
		RST:     rst,
		Window:  1024, Options: []layers.TCPOption{
			{
				OptionType:   layers.TCPOptionKindMSS,
				OptionLength: 4,
				OptionData:   []byte{5, 0xb4},
			},
		},
	}
	if tcp.RST {
		tcp.SYN = false
		tcp.Window = 0
		tcp.Options = nil
	}
	err = tcp.SetNetworkLayerForChecksum(&ip4)
	if err != nil {
		return nil, loopback, errors.Errorf("ip4 set network layer checksum failed: %s", err)
	}

	return []gopacket.SerializableLayer{
		baseLayer, &ip4, &tcp,
	}, loopback, nil
}

func (s *Scanner) createSynTCP(dstIp net.IP, dstPort int, dstMac net.HardwareAddr, gateway string) ([]gopacket.SerializableLayer, bool, error) {
	return s.createTCPWithDstMac(dstIp, dstPort, true, false, dstMac, gateway)
}

func (s *Scanner) createRstTCP(dstIp net.IP, dstPort int, dstMac net.HardwareAddr, gateway string) ([]gopacket.SerializableLayer, bool, error) {
	return s.createTCPWithDstMac(dstIp, dstPort, false, true, dstMac, gateway)
}
