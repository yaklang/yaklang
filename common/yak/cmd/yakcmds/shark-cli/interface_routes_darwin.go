//go:build darwin

package sharkcli

import (
	"net"
	"syscall"

	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
	"golang.org/x/net/route"
)

// Read the kernel RIB instead of parsing netstat's version-dependent flags.
func loadDefaultRoutes() ([]routewrapper.Route, error) {
	rib, err := route.FetchRIB(syscall.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, err
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	byIndex := map[int]*net.Interface{}
	for i := range interfaces {
		byIndex[interfaces[i].Index] = &interfaces[i]
	}
	var routes []routewrapper.Route
	for _, message := range messages {
		r, ok := message.(*route.RouteMessage)
		if !ok || r.Flags&syscall.RTF_UP == 0 || r.Flags&syscall.RTF_GATEWAY == 0 || r.Flags&(syscall.RTF_HOST|syscall.RTF_REJECT|syscall.RTF_BLACKHOLE) != 0 {
			continue
		}
		if len(r.Addrs) <= syscall.RTAX_NETMASK {
			continue
		}
		destination := routeIP(r.Addrs[syscall.RTAX_DST])
		gateway := routeIP(r.Addrs[syscall.RTAX_GATEWAY])
		mask := routeIP(r.Addrs[syscall.RTAX_NETMASK])
		if destination == nil || !destination.IsUnspecified() || gateway == nil || (mask != nil && !mask.IsUnspecified()) {
			continue
		}
		bits := 128
		if destination.To4() != nil {
			bits = 32
		}
		metric := 0
		if r.Flags&syscall.RTF_IFSCOPE != 0 {
			// Prefer the global default when Wi-Fi and Ethernet also have
			// interface-scoped defaults installed at the same time.
			metric = 1
		}
		routes = append(routes, routewrapper.Route{Destination: net.IPNet{IP: destination, Mask: net.CIDRMask(0, bits)}, Gateway: gateway, Interface: byIndex[r.Index], Metric: metric})
	}
	return routes, nil
}
func routeIP(addr route.Addr) net.IP {
	switch a := addr.(type) {
	case *route.Inet4Addr:
		return net.IP(a.IP[:])
	case *route.Inet6Addr:
		return net.IP(a.IP[:])
	}
	return nil
}
