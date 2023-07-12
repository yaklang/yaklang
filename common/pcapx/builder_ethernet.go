package pcapx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

var ethernetLayerExports = map[string]interface{}{
	"ethernet_srcMac": EthernetLayerBuilderWithSrcMacStr,
	"ethernet_dstMac": EthernetLayerBuilderWithDstMacStr,
	"ethernet_type":   EthernetLayerBuilderWithEthernetType,
}

type EthernetLayerBuilderConfig struct {
	SrcMac       net.HardwareAddr
	DstMac       net.HardwareAddr
	EthernetType layers.EthernetType
}

type EthernetLayerBuilderOption func(config *EthernetLayerBuilderConfig)

func EthernetLayerBuilderWithSrcMac(srcMac net.HardwareAddr) EthernetLayerBuilderOption {
	return func(config *EthernetLayerBuilderConfig) {
		config.SrcMac = srcMac
	}
}

func EthernetLayerBuilderWithSrcMacStr(srcMacStr string) EthernetLayerBuilderOption {
	srcMac, err := net.ParseMAC(srcMacStr)
	if err != nil {
		log.Errorf("EthernetLayerBuilderWithSrcMac: %v", err)
		return func(config *EthernetLayerBuilderConfig) {}
	}
	return EthernetLayerBuilderWithSrcMac(srcMac)
}

func EthernetLayerBuilderWithDstMac(dstMac net.HardwareAddr) EthernetLayerBuilderOption {
	return func(config *EthernetLayerBuilderConfig) {
		config.DstMac = dstMac
	}
}

func EthernetLayerBuilderWithDstMacStr(dstMacStr string) EthernetLayerBuilderOption {
	dstMac, err := net.ParseMAC(dstMacStr)
	if err != nil {
		log.Errorf("EthernetLayerBuilderWithDstMacStr: %v", err)
		return func(config *EthernetLayerBuilderConfig) {}
	}
	return EthernetLayerBuilderWithDstMac(dstMac)
}

func EthernetLayerBuilderWithEthernetType(i any) EthernetLayerBuilderOption {
	return func(config *EthernetLayerBuilderConfig) {
		switch ret := strings.ToLower(utils.InterfaceToString(i)); ret {
		case "ipv4", "ip4", "ip":
			config.EthernetType = layers.EthernetTypeIPv4
		case "ipv6", "ip6":
			config.EthernetType = layers.EthernetTypeIPv6
		case "arp":
			config.EthernetType = layers.EthernetTypeARP
		case "mpls":
			config.EthernetType = layers.EthernetTypeMPLSUnicast
		case "pppoe":
			config.EthernetType = layers.EthernetTypePPPoEDiscovery
		case "pppoesession":
			config.EthernetType = layers.EthernetTypePPPoESession
		case "dot1q":
			config.EthernetType = layers.EthernetTypeDot1Q
		default:
			config.EthernetType = layers.EthernetType(utils.Atoi(ret))
		}
	}
}

func (l *EthernetLayerBuilderConfig) Create() *layers.Ethernet {
	return &layers.Ethernet{
		SrcMAC:       nil,
		DstMAC:       nil,
		EthernetType: 0,
	}
}
