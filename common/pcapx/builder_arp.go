package pcapx

import (
	"errors"
	"github.com/gopacket/gopacket/layers"
	"net"
)

var arpLayerExports = map[string]interface{}{
	"arp_request":   WithArp_RequestAuto,
	"arp_requestEx": WithArp_RequestEx,
	"arp_replyEx":   WithArp_ReplyToEx,
	"arp_reply":     WithArp_ReplyTo,
}

func init() {
	for k, v := range arpLayerExports {
		Exports[k] = v
	}
}

type ArpConfig func(arp *layers.ARP) error

func WithArp_RequestEx(selfIP, selfMac string, remoteIP string) ArpConfig {
	local := []byte(net.ParseIP(selfIP).To4())
	srcMac, _ := net.ParseMAC(selfMac)
	dstIP := []byte(net.ParseIP(remoteIP).To4())
	return func(arp *layers.ARP) error {
		arp.HwAddressSize = 6
		arp.ProtAddressSize = 4
		arp.Operation = layers.ARPRequest
		arp.SourceProtAddress = local
		if arp.SourceProtAddress == nil {
			return errors.New("invalid source ip: " + string(local))
		}
		arp.SourceHwAddress = srcMac
		arp.DstHwAddress = make([]byte, 6)
		arp.DstProtAddress = dstIP
		if arp.DstProtAddress == nil {
			return errors.New("invalid destination ip: " + string(dstIP))
		}
		return nil
	}
}

func WithArp_RequestAuto(ip string) ArpConfig {
	// where is ip?
	return func(arp *layers.ARP) error {
		iface, _, local, err := getPublicRoute()
		if err != nil {
			return err
		}
		srcMac := iface.HardwareAddr
		return WithArp_RequestEx(local.String(), srcMac.String(), ip)(arp)
	}
}

func WithArp_ReplyToEx(srcTarget, srcMac, targetIp, targetMac string) ArpConfig {
	return func(arp *layers.ARP) error {
		arp.Operation = layers.ARPReply
		arp.SourceProtAddress = net.ParseIP(srcTarget)
		if arp.SourceProtAddress == nil {
			return errors.New("invalid source ip: " + srcTarget)
		}
		arp.SourceHwAddress, _ = net.ParseMAC(srcMac)
		arp.DstHwAddress, _ = net.ParseMAC(targetMac)
		arp.DstProtAddress = net.ParseIP(targetIp)
		if arp.DstProtAddress == nil {
			return errors.New("invalid destination ip: " + targetIp)
		}
		return nil
	}
}

func WithArp_ReplyTo(targetIp, targetMac string) ArpConfig {
	iface, _, local, err := getPublicRoute()
	if err != nil {
		return func(arp *layers.ARP) error {
			return err
		}
	}
	srcMac := iface.HardwareAddr
	return WithArp_ReplyToEx(local.String(), srcMac.String(), targetIp, targetMac)
}
