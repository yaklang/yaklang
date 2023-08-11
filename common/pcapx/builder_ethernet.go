package pcapx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

var ethernetLayerExports = map[string]interface{}{
	"ethernet_srcMac":          WithEthernet_SrcMac,
	"ethernet_dstMac":          WithEthernet_DstMac,
	"ethernet_next_layer_type": WithEthernet_NextLayerType,
}

func init() {
	for k, v := range ethernetLayerExports {
		Exports[k] = v
	}
}

type EthernetOption func(config *layers.Ethernet) error

func WithEthernet_SrcMac(srcMac any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := srcMac.(type) {
		case net.HardwareAddr:
			config.SrcMAC = ret
			return nil
		default:
			var err error
			config.SrcMAC, err = net.ParseMAC(utils.InterfaceToString(ret))
			if err != nil {
				return utils.Errorf("parse %v to mac failed: %s", ret, err)
			}
			return nil
		}
	}
}

func WithEthernet_DstMac(dstMac any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := dstMac.(type) {
		case net.HardwareAddr:
			config.DstMAC = ret
			return nil
		default:
			var err error
			config.DstMAC, err = net.ParseMAC(utils.InterfaceToString(ret))
			if err != nil {
				return utils.Errorf("parse %v to mac failed: %s", ret, err)
			}
			return nil
		}
	}
}

func WithEthernet_NextLayerType(i any) EthernetOption {
	return func(config *layers.Ethernet) error {
		switch ret := strings.ToLower(utils.InterfaceToString(i)); ret {
		case "ipv4", "ip4", "ip":
			config.EthernetType = layers.EthernetTypeIPv4
		case "ipv6", "ip6":
			config.EthernetType = layers.EthernetTypeIPv6
		case "arpx":
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
		return nil
	}
}
