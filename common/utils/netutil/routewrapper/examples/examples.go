package main

import (
	"fmt"
	"net"
	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

func main() {
	w, err := routewrapper.NewBSDRouteWrapper(
		"/usr/sbin/netstat",
		"/sbin/route",
	)
	if err != nil {
		panic(err.Error())
	}
	routes, err := w.Routes()
	if err != nil {
		panic(err.Error())
	}
	for i, route := range routes {
		ifName := "*"
		if route.Interface != nil {
			ifName = route.Interface.Name
		}
		fmt.Printf("%d: %s %s\n", i, route.Destination.String(), ifName)
	}
	if_, err := w.GetInterface("en0")
	if err != nil {
		panic(err.Error())
	}
	err = w.AddRoute(routewrapper.Route{
		Destination: net.IPNet{net.ParseIP("10.0.0.1"), nil},
		Gateway:     nil,
		Interface:   if_,
	})
	if err != nil {
		panic(err.Error())
	}
}
