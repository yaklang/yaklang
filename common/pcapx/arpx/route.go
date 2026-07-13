package arpx

import (
	"net"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

func RouteAndArpWithTimeout(t time.Duration, target string) (net.HardwareAddr, error) {
	iface, targetIP, _, err := netutil.Route(t, target)
	if err != nil {
		return nil, err
	}

	if targetIP.String() == utils.FixForParseIP(target) {
		return iface.HardwareAddr, nil
	}

	return ArpWithTimeout(t, iface.Name, targetIP.String())
}
