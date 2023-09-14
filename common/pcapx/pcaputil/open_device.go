package pcaputil

import (
	"fmt"
	"github.com/google/gopacket/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"sort"
	"strings"
)

func PcapInterfaceEqNetInterface(piface pcap.Interface, iface *net.Interface) bool {
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

type ConvertIfaceNameError struct {
	name string
}

func (e *ConvertIfaceNameError) Error() string {
	return fmt.Sprintf("convert iface name failed: %s", e.name)
}

func NewConvertIfaceNameError(name string) *ConvertIfaceNameError {
	return &ConvertIfaceNameError{
		name: name,
	}
}

var cachedFindAllDevs = utils.CacheFunc(60, pcap.FindAllDevs)

func IfaceNameToPcapIfaceName(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", utils.Errorf("fetch net.Interface failed: %s", err)
	}

	devs, err := cachedFindAllDevs()
	if err != nil {
		return "", utils.Errorf("find pcap dev failed: %s", err)
	}

	for _, dev := range devs {
		if PcapInterfaceEqNetInterface(dev, iface) {
			return dev.Name, nil
		}
	}
	return "", NewConvertIfaceNameError(name)
}

func GetPublicInternetPcapHandler() (*pcap.Handle, error) {
	iface, _, _, err := netutil.GetPublicRoute()
	if err != nil {
		return nil, err
	}
	ifaceName, err := IfaceNameToPcapIfaceName(iface.Name)
	if err != nil {
		return nil, err
	}
	return OpenIfaceLive(ifaceName)
}

func OpenFile(filename string) (*pcap.Handle, error) {
	handler, err := pcap.OpenOffline(filename)
	if err != nil {
		return nil, utils.Errorf("pcap.OpenOffline failed: %s", err)
	}
	return handler, nil
}

func OpenIfaceLive(iface string) (*pcap.Handle, error) {
	handler, err := pcap.OpenLive(iface, 65535, true, pcap.BlockForever)
	if err != nil {
		return nil, utils.Errorf("pcap.OpenLive %s failed: %v", iface, err)
	}
	log.Infof("open iface %s success\n", iface)
	return handler, nil
}
