package sharkcli

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

func defaultCaptureInterface() (string, string, error) {
	routes, err := loadDefaultRoutes()
	if err != nil {
		return "", "", fmt.Errorf("read physical default routes: %w; choose --interface explicitly", err)
	}
	chosen, err := choosePhysicalRoute(routes)
	if err != nil {
		return "", "", err
	}
	return chosen.Interface.Name, chosen.Gateway.String(), nil
}

func choosePhysicalRoute(routes []routewrapper.Route) (routewrapper.Route, error) {
	var candidates []routewrapper.Route
	for _, r := range routes {
		if !r.IsDefaultRoute() || r.Interface == nil || !physicalInterface(*r.Interface) {
			continue
		}
		if r.Gateway == nil || r.Gateway.IsUnspecified() || r.Gateway.IsLoopback() {
			continue
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return routewrapper.Route{}, fmt.Errorf("no active physical interface with a default gateway; use --list-interfaces and --interface (VPN/TUN interfaces require explicit selection)")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.Gateway.To4() != nil) != (b.Gateway.To4() != nil) {
			return a.Gateway.To4() != nil
		}
		if a.Metric != b.Metric {
			return a.Metric < b.Metric
		}
		return a.Interface.Index < b.Interface.Index
	})
	return candidates[0], nil
}
func physicalInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || len(iface.HardwareAddr) == 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	for _, prefix := range []string{"utun", "tun", "tap", "wg", "tailscale", "zt", "veth", "docker", "br-", "bridge", "virbr", "vmnet", "vboxnet", "awdl", "llw", "gif", "stf", "ipsec", "ppp", "vethernet"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}
