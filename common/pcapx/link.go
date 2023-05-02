package pcapx

import (
	"github.com/google/gopacket/layers"
	"net"
	"time"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

var (
	PublicGatewayAddress   net.IP
	PublicPreferredAddress net.IP
	PublicInterface        *net.Interface
)

func GetPublicRoute() (*net.Interface, net.IP, net.IP, error) {
	if PublicInterface != nil && PublicGatewayAddress != nil && PublicPreferredAddress != nil {
		return PublicInterface, PublicGatewayAddress, PublicPreferredAddress, nil
	}
	iface, gw, ip, err := netutil.Route(3*time.Second, "8.8.8.8")
	if err != nil {
		return nil, nil, nil, err
	}
	PublicInterface = iface
	PublicPreferredAddress = ip
	PublicGatewayAddress = gw
	return iface, gw, ip, nil
}

func GetPublicLinkLayer(t layers.EthernetType, toServer bool) (*layers.Ethernet, error) {
	iface, gw, _, err := GetPublicRoute()
	if err != nil {
		return nil, err
	}
	srcMac := iface.HardwareAddr
	dstMac, err := utils.Arp(iface.Name, gw.String())
	if err != nil {
		return nil, err
	}
	if !toServer {
		srcMac, dstMac = dstMac, srcMac
	}
	return &layers.Ethernet{
		SrcMAC:       srcMac,
		DstMAC:       dstMac,
		EthernetType: t,
	}, nil
}

var ethIPv4, ethIPv6, ethIPv4ToServer, ethIPv6ToServer *layers.Ethernet

func GetPublicToServerLinkLayerIPv4() (*layers.Ethernet, error) {
	if ethIPv4ToServer != nil {
		return ethIPv4ToServer, nil
	}
	return GetPublicLinkLayer(layers.EthernetTypeIPv4, true)
}

func GetPublicToServerLinkLayerIPv6() (*layers.Ethernet, error) {
	if ethIPv6ToServer != nil {
		return ethIPv6ToServer, nil
	}
	return GetPublicLinkLayer(layers.EthernetTypeIPv6, true)
}

func GetPublicToClientLinkLayerIPv4() (*layers.Ethernet, error) {
	if ethIPv4 != nil {
		return ethIPv4, nil
	}
	return GetPublicLinkLayer(layers.EthernetTypeIPv4, false)
}

func GetPublicToClientLinkLayerIPv6() (*layers.Ethernet, error) {
	if ethIPv6 != nil {
		return ethIPv6, nil
	}
	return GetPublicLinkLayer(layers.EthernetTypeIPv6, false)
}
