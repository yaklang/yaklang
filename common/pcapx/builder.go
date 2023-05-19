package pcapx

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
		arpConfig      *ArpLayerBuilderConfig
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
		case ArpLayerBuilderConfigOption:
			if arpConfig == nil {
				arpConfig = &ArpLayerBuilderConfig{}
			}
			optFunc(arpConfig)
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

	// NetworkLayer can be IPv4(Default) or ARP
	if !funk.Any(ipConfig, arpConfig) {
		return nil, utils.Errorf("PacketBuilder: network layer is empty")
	}

	var networkLayer gopacket.SerializableLayer
	var ipEnabled bool
	if ipConfig != nil {
		ipEnabled = true
		networkLayer = ipConfig.Create()
	} else if arpConfig != nil {
		networkLayer = arpConfig.Create()
	} else {
		return nil, utils.Errorf("PacketBuilder: network layer is empty")
	}
	_ = networkLayer

	var err error
	if ipEnabled {
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
