package pcapx

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
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

// WithPayload 为 pcapx.PacketBuilder 设置数据包的负载(Payload)内容
// 在 yak 中通过 pcapx.WithPayload 调用
// 参数:
//   - payload: 负载字节数据
//
// 返回值:
//   - 一个 pcapx.PacketBuilder 可接收的构建配置选项
//
// Example:
// ```
// // 该示例为示意性用法：构造带负载的数据包
// raw = pcapx.PacketBuilder(pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"), pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.WithPayload([]byte("hello")))~
// println(len(raw))
// ```
func WithPayload(payload []byte) BuilderConfigOption {
	return func(config *BuilderConfig) {
		config.Payload = payload
	}
}

func WithLoopback(b bool) BuilderConfigOption {
	return func(config *BuilderConfig) {
		config.Loopback = b
	}
}

// PacketBuilder 按照传入的各层配置选项组合并序列化出一个完整的数据包字节
// 在 yak 中通过 pcapx.PacketBuilder 调用，可叠加链路层/网络层/传输层等多种选项
// 参数:
//   - opts: 一组数据包构建选项(如 pcapx.ipv4_srcIp、pcapx.tcp_dstPort、pcapx.WithPayload 等)
//
// 返回值:
//   - 序列化后的数据包字节
//   - 构建过程中的错误
//
// Example:
// ```
// // 该示例为示意性用法：构建一个 TCP SYN 数据包
// raw = pcapx.PacketBuilder(
//
//	pcapx.ipv4_srcIp("1.1.1.1"), pcapx.ipv4_dstOp("2.2.2.2"),
//	pcapx.tcp_srcPort(12345), pcapx.tcp_dstPort(80), pcapx.tcp_flag("syn"),
//
// )~
// println(len(raw))
// ```
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
		loopbackConfig *layers.Loopback
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
					Protocol:        layers.EthernetTypeIPv4,
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
		case LoopbackOption:
			if loopbackConfig == nil {
				loopbackConfig = &layers.Loopback{
					Family: layers.ProtocolFamilyIPv4,
				}
			}
			err := optFunc(loopbackConfig)
			if err != nil {
				if err != nil {
					return nil, utils.Errorf("set loopback config failed: %s", err)
				}

			}
		default:
			log.Errorf("PacketBuilder: unknown option type: %T", optFunc)
			return nil, utils.Errorf("PacketBuilder: unknown option type: %T", optFunc)
		}
	}

	/**
	LinkLayer can be Ethernet(Default) or Loopback
	*/
	var linkLayer gopacket.SerializableLayer
	if baseConfig.Loopback {
		if loopbackConfig == nil {
			loopbackConfig = &layers.Loopback{
				Family: layers.ProtocolFamilyIPv4,
			}
		}
		linkLayer = loopbackConfig
	} else if ethernetConfig != nil {
		linkLayer = ethernetConfig
	} else {
		var err error
		linkLayer, err = GetPublicToServerLinkLayerIPv4()
		if err != nil {
			log.Errorf("PacketBuilder: %v", err)
			linkLayer = &layers.Ethernet{
				EthernetType: layers.EthernetTypeIPv4,
				SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 1},
				DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 2},
			}
		}
	}

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
			linkLayer.(*layers.Ethernet).EthernetType = layers.EthernetTypeIPv6
		}
	} else if !funk.IsEmpty(arpConfig) {
		networkLayer = arpConfig
		linkLayer.(*layers.Ethernet).EthernetType = layers.EthernetTypeARP
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
