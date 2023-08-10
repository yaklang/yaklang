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
		baseConfig     = &BuilderConfig{}
		tcpConfig      *TCPBuilderConfig
		arpConfig      *layers.ARP
		ipConfig       *IPv4LayerBuilderConfig
		ethernetConfig *EthernetLayerBuilderConfig
	)
	for _, opt := range opts {
		switch optFunc := opt.(type) {
		case BuilderConfigOption:
			optFunc(baseConfig)
		case TCPBuilderConfigOption:
			if tcpConfig == nil {
				tcpConfig = &TCPBuilderConfig{}
			}
			optFunc(tcpConfig)
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
		case IPv4LayerBuilderConfigOption:
			if ipConfig == nil {
				ipConfig = &IPv4LayerBuilderConfig{}
			}
			optFunc(ipConfig)
		case EthernetLayerBuilderOption:
			if ethernetConfig == nil {
				ethernetConfig = &EthernetLayerBuilderConfig{}
			}
			optFunc(ethernetConfig)
		default:
			log.Errorf("PacketBuilder: unknown option type: %T", optFunc)
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
			log.Warn("PacketBuilder: ethernet layer is empty, use default")
			var err error
			linkLayer, err = GetPublicToServerLinkLayerIPv4()
			if err != nil {
				return nil, utils.Errorf("PacketBuilder: %v", err)
			}
		} else {
			linkLayer = ethernetConfig.Create()
		}
	}
	_ = linkLayer

	/*
		check network layer?
	*/
	var fetched = false
	var networkLayerRaw any
	for _, l := range []any{
		arpConfig, ipConfig,
	} {
		if funk.IsEmpty(l) {
			if fetched {
				return nil, utils.Errorf("PacketBuilder: only one network layer is allowed")
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
	if !funk.IsEmpty(ipConfig) {
		ipEnabled = true
		networkLayer = ipConfig.Create()
		if ipConfig.Version == 6 {
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
		// TransportLayer can be TCP(Default)
		if tcpConfig == nil {
			log.Warn("PacketBuilder: tcp layer is empty, use default")
			tcpConfig = &TCPBuilderConfig{}
		}
		transportLayer := tcpConfig.Create()
		ip4Layer := networkLayer.(*layers.IPv4)
		err := transportLayer.SetNetworkLayerForChecksum(ip4Layer)
		if err != nil {
			return nil, utils.Errorf("TCP checksum failed: %s", err)
		}
		var buf = gopacket.NewSerializeBuffer()
		err = gopacket.SerializeLayers(
			buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			linkLayer, ip4Layer, transportLayer, gopacket.Payload(baseConfig.Payload),
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
