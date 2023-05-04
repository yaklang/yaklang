package routewrapper

import (
	"net"
	"regexp"
)

type Route struct {
	Destination net.IPNet
	Gateway     net.IP
	Interface   *net.Interface
	Flags       map[string]string
	Expire      int
	Metric      int
}

func (route *Route) IsDefaultRoute() bool {
	ones, _ := route.Destination.Mask.Size()
	return (route.Destination.IP.Equal(net.IPv4zero) || route.Destination.IP.Equal(net.IPv6zero)) && ones == 0
}

func (route *Route) DestinationIsNetwork() bool {
	if route.Destination.Mask == nil {
		return false
	}
	ones, bits := route.Destination.Mask.Size()
	if route.Destination.IP.To4() != nil {
		if ones == 32 && bits == 32 {
			return false
		}
	} else {
		if ones == 128 && bits == 128 {
			return false
		}
	}
	return true
}

type Routing interface {
	Routes() ([]Route, error)
	DefaultRoutes() ([]Route, error)
	AddRoute(Route) error
	GetInterface(name string) (*net.Interface, error)
}

var delimitedByWhitespaces = regexp.MustCompile("\\s+")
