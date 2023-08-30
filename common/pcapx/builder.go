package pcapx

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	linkExport = map[string]any{
		"LINK_TYPE_ETHERNET":  layers.LinkTypeEthernet,
		"LINK_TYPE_LOOP":      layers.LinkTypeLoop,
		"LINK_TYPE_RAW":       layers.LinkTypeRaw,
		"LINK_TYPE_LINUX_SLL": layers.LinkTypeLinuxSLL,
		"LINK_TYPE_IPV4":      layers.LinkTypeIPv4,
		"LINK_TYPE_IPV6":      layers.LinkTypeIPv6,
		"LINK_TYPE_NULL":      layers.LinkTypeNull,
	}
)

type BuilderConfig struct {
	Loopback bool
	Payload  []byte
}

type BuilderConfigOption func(config *BuilderConfig)

func WithPayload(payload []byte) BuilderConfigOption {
	return func(config *BuilderConfig) {
		config.Payload = payload
	}
}

func WithLoopback() BuilderConfigOption {
	return func(config *BuilderConfig) {
		config.Loopback = true
	}
}

func PacketBuilder(opts ...any) ([]byte, error) {
	var (
		baseConfig = &BuilderConfig{}

		// transport layer
		udpConfig   *layers.UDP
		tcpConfig   *layers.TCP
		icmp4Config *layers.ICMPv4

		// link and network
		arpConfig      *layers.ARP
		ip4Config      *layers.IPv4
		ethernetConfig *layers.Ethernet
	)
	for _, opt := range opts {
		switch optFunc := opt.(type) {
		case BuilderConfigOption:
			optFunc(baseConfig)
		case UDPOption:
			if udpConfig == nil {
				udpConfig = &layers.UDP{SrcPort: 80, DstPort: 80}
			}
			err := optFunc(udpConfig)
			if err != nil {
				return nil, utils.Errorf("set udp config failed: %s", err)
			}
		case TCPOption:
			if tcpConfig == nil {
				tcpConfig = NewDefaultTCPLayer()
			}
			err := optFunc(tcpConfig)
			if err != nil {
				return nil, utils.Errorf("set tcp config failed: %s", err)
			}
		case ICMPOption:
			if icmp4Config == nil {
				icmp4Config = &layers.ICMPv4{}
			}
			err := optFunc(icmp4Config)
			if err != nil {
				return nil, utils.Errorf("set icmp4 config failed: %s", err)
			}
		case ArpConfig:
			if arpConfig == nil {
				arpConfig = &layers.ARP{
					AddrType:        layers.LinkTypeEthernet,
					Protocol:        layers.EthernetTypeARP,
					HwAddressSize:   6,
					ProtAddressSize: 4,
				}
			}
			err := optFunc(arpConfig)
			if err != nil {
				return nil, err
			}
		case IPv4Option:
			if ip4Config == nil {
				ip4Config = NewDefaultIPv4Layer()
			}
			err := optFunc(ip4Config)
			if err != nil {
				return nil, utils.Errorf("set ipv4 config failed: %s", err)
			}
		case EthernetOption:
			if ethernetConfig == nil {
				ethernetConfig = &layers.Ethernet{
					EthernetType: layers.EthernetTypeIPv4,
				}
			}
			err := optFunc(ethernetConfig)
			if err != nil {
				return nil, utils.Errorf("set ethernet config failed: %s", err)
			}
		default:
			log.Errorf("PacketBuilder: unknown option type: %T", optFunc)
			return nil, utils.Errorf("PacketBuilder: unknown option type: %T", optFunc)
		}
	}

	/**
	LinkLayer can be Ethernet(Default) or Loopback
	*/
	var linkLayer *layers.Ethernet
	if baseConfig.Loopback {
		linkLayer = &layers.Ethernet{EthernetType: layers.EthernetTypeIPv4}
	} else {
		if ethernetConfig == nil {
			var err error
			linkLayer, err = GetPublicToServerLinkLayerIPv4()
			if err != nil {
				return nil, utils.Errorf("PacketBuilder: %v", err)
			}
		} else {
			linkLayer = ethernetConfig
		}
	}
	_ = linkLayer

	/*
		check network layer?
	*/
	var fetched = false
	var networkLayerRaw any
	for _, l := range []any{
		arpConfig, ip4Config,
	} {
		if funk.IsEmpty(l) {
			if fetched {
				return nil, utils.Errorf("PacketBuilder: only one network layer is allowed, need ip / arp layer")
			} else {
				fetched = true
				networkLayerRaw = l
			}
		}
	}
	if networkLayerRaw == nil {
		return nil, utils.Errorf("PacketBuilder: network layer is empty")
	}

	var networkLayer gopacket.SerializableLayer
	var ipEnabled bool
	if !funk.IsEmpty(ip4Config) {
		ipEnabled = true
		networkLayer = ip4Config
		if ip4Config.Version == 6 {
			linkLayer.EthernetType = layers.EthernetTypeIPv6
		}
	} else if !funk.IsEmpty(arpConfig) {
		networkLayer = arpConfig
		linkLayer.EthernetType = layers.EthernetTypeARP
	} else {
		return nil, utils.Errorf("PacketBuilder: network layer is empty")
	}
	_ = networkLayer

	var err error
	if ipEnabled {
		// TCP/IP Stack!
		// TransportLayer can be TCP(Default) / ICMP / IGMP / UDP ...
		var transportLayer gopacket.SerializableLayer
	TRANS:
		if tcpConfig != nil {
			tcpLayer := tcpConfig
			ip4Layer := networkLayer.(*layers.IPv4)
			err := tcpLayer.SetNetworkLayerForChecksum(ip4Layer)
			if err != nil {
				return nil, utils.Errorf("TCP checksum failed: %s", err)
			}
			ip4Layer.Protocol = layers.IPProtocolTCP
			transportLayer = tcpLayer
		} else if icmp4Config != nil {
			transportLayer = icmp4Config
			ip4Layer := networkLayer.(*layers.IPv4)
			ip4Layer.Protocol = layers.IPProtocolICMPv4
		} else if udpConfig != nil {
			ip4Layer := networkLayer.(*layers.IPv4)
			ip4Layer.Protocol = layers.IPProtocolUDP
			err := udpConfig.SetNetworkLayerForChecksum(ip4Layer)
			if err != nil {
				return nil, utils.Errorf("UDP checksum failed: %s", err)
			}
			transportLayer = udpConfig
		} else {
			log.Warn("PacketBuilder: tcp layer is empty, use default")
			tcpConfig = NewDefaultTCPLayer()
			goto TRANS
		}

		if err != nil {
			return nil, utils.Errorf("TCP checksum failed: %s", err)
		}
		var buf = gopacket.NewSerializeBuffer()
		err = gopacket.SerializeLayers(
			buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			linkLayer, networkLayer, transportLayer, gopacket.Payload(baseConfig.Payload),
		)
		if err != nil {
			return nil, utils.Errorf(`gopacket.SerializeLayers failed: %s`, err)
		}
		return buf.Bytes(), nil
	}

	/*
		Fullback
	*/
	if networkLayer == nil {
		return nil, utils.Errorf("PacketBuilder: network layer is empty(BUG)")
	}
	var buf = gopacket.NewSerializeBuffer()
	err = gopacket.SerializeLayers(
		buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		linkLayer, networkLayer, gopacket.Payload(baseConfig.Payload),
	)
	if err != nil {
		return nil, utils.Errorf(`gopacket.SerializeLayers failed: %s`, err)
	}
	return buf.Bytes(), nil
}
