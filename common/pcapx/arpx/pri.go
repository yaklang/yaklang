package arpx

import (
	"fmt"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sort"
	"strings"
)

func _pcapInterfaceEqNetInterface(piface pcap.Interface, iface *net.Interface) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		log.Errorf("fetch iface[%v] addrs failed: %s", iface.Name, err)
		return false
	}

	var pIfaceAddrs []string
	var ifaceAddrs []string

	for _, addr := range piface.Addresses {
		pIfaceAddrs = append(pIfaceAddrs, addr.IP.String())
	}

	for _, addr := range addrs {
		ipValue, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ifaceAddrs = append(ifaceAddrs, ipValue.String())
	}

	if pIfaceAddrs == nil || ifaceAddrs == nil {
		log.Debugf("no iIfaceAddrs[pcap:%v] or ifaceAddrs[net:%v]", piface.Name, iface.Name)
		return false
	}

	sort.Strings(pIfaceAddrs)
	sort.Strings(ifaceAddrs)
	return utils.CalcSha1(strings.Join(pIfaceAddrs, "|")) == utils.CalcSha1(strings.Join(ifaceAddrs, "|"))
}

type convertIfaceNameError struct {
	name string
}

func (e *convertIfaceNameError) Error() string {
	return fmt.Sprintf("convert iface name failed: %s", e.name)
}

func newConvertIfaceNameError(name string) *convertIfaceNameError {
	return &convertIfaceNameError{
		name: name,
	}
}

func _ifaceNameToPcapIfaceName(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", utils.Errorf("fetch net.Interface failed: %s", err)
	}

	devs, err := pcap.FindAllDevs()
	if err != nil {
		return "", utils.Errorf("find pcap dev failed: %s", err)
	}

	for _, dev := range devs {
		if _pcapInterfaceEqNetInterface(dev, iface) {
			return dev.Name, nil
		}
	}
	return "", newConvertIfaceNameError(name)
}
