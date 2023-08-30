package pcapx

import (
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/arpx"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"time"
)

var (
	PublicGatewayAddress   net.IP
	PublicPreferredAddress net.IP
	PublicInterface        *net.Interface
)

func getPublicRoute() (*net.Interface, net.IP, net.IP, error) {
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
	iface, gw, _, err := getPublicRoute()
	if err != nil {
		return nil, err
	}
	srcMac := iface.HardwareAddr
	dstMac, err := arpx.Arp(iface.Name, gw.String())
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
	var err error
	ethIPv4ToServer, err = GetPublicLinkLayer(layers.EthernetTypeIPv4, true)
	return ethIPv4ToServer, err
}

func GetPublicToServerLinkLayerIPv6() (*layers.Ethernet, error) {
	if ethIPv6ToServer != nil {
		return ethIPv6ToServer, nil
	}
	var err error
	ethIPv6ToServer, err = GetPublicLinkLayer(layers.EthernetTypeIPv6, true)
	return ethIPv6ToServer, err
}

func GetPublicToClientLinkLayerIPv4() (*layers.Ethernet, error) {
	if ethIPv4 != nil {
		return ethIPv4, nil
	}
	var err error
	ethIPv4, err = GetPublicLinkLayer(layers.EthernetTypeIPv4, false)
	return ethIPv4, err
}

func GetPublicToClientLinkLayerIPv6() (*layers.Ethernet, error) {
	if ethIPv6 != nil {
		return ethIPv6, nil
	}
	var err error
	ethIPv6, err = GetPublicLinkLayer(layers.EthernetTypeIPv6, false)
	return ethIPv6, err
}
