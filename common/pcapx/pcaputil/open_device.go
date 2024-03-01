package pcaputil

import (
	"fmt"
	"github.com/google/gopacket/pcap"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"runtime"
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

func PcapIfaceNameToNetInterface(ifaceName string) (*net.Interface, error) {
	devs, err := cachedFindAllDevs()
	if err != nil {
		return nil, utils.Errorf("find pcap dev failed: %s", err)
	}
	for _, dev := range devs {
		if dev.Name == ifaceName {
			// windows 下的 pcap dev name 与 net.Interface.Name 不一致
			if runtime.GOOS == "windows" {
				if len(dev.Addresses) < 0 {
					return nil, utils.Errorf("addresses length is too short: %s", ifaceName)
				}
				iface, err := netutil.FindInterfaceByIP(dev.Addresses[0].IP.String())
				if err != nil {
					return nil, utils.Errorf("fetch net.Interface failed: %s", err)
				}
				if PcapInterfaceEqNetInterface(dev, &iface) {
					return &iface, nil
				}
			} else {
				iface, err := net.InterfaceByName(dev.Name)
				if err != nil {
					return nil, utils.Errorf("fetch net.Interface failed: %s", err)
				}
				if PcapInterfaceEqNetInterface(dev, iface) {
					return iface, nil
				}
			}
		}
	}
	return nil, utils.Errorf("no iface found: %s", ifaceName)
}

func AllDevices() []*pcap.Interface {
	ifs, err := pcap.FindAllDevs()
	if err != nil {
		log.Errorf("find pcap dev failed: %s", err)
	}
	return lo.Map(ifs, func(item pcap.Interface, index int) *pcap.Interface {
		return &item
	})
}

func GetPcapInterfaceByIndex(i int) (*pcap.Interface, error) {
	devs, err := cachedFindAllDevs()
	if err != nil {
		return nil, utils.Errorf("find pcap dev failed: %s", err)
	}
	if i < 0 || i >= len(devs) {
		return nil, utils.Errorf("index out of range: %d", i)
	}
	return &devs[i], nil
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
	log.Infof("open iface %s success", iface)
	return handler, nil
}
