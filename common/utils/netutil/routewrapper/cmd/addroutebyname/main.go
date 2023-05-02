package main

import (
	"fmt"
	"net"
	"os"
	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

var progname = os.Args[0]

func bail(msg string, status int) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", progname, msg)
	os.Exit(status)
}

func main() {
	if len(os.Args) < 3 {
		bail("too few arguments", 255)
	}
	ifName := os.Args[1]
	hosts := os.Args[2:]
	w, err := routewrapper.NewRouteWrapper()
	if err != nil {
		bail(err.Error(), 1)
	}
	if_, err := w.GetInterface(ifName)
	if err != nil {
		bail(err.Error(), 1)
	}
	addrs := make([]net.IP, 0, 16)
	for _, host := range hosts {
		ips, err := net.LookupIP(host)
		if err != nil {
			bail(err.Error(), 1)
		}
		addrs = append(addrs, ips...)
	}
	for _, addr := range addrs {
		err = w.AddRoute(routewrapper.Route{
			Destination: net.IPNet{addr, nil},
			Gateway:     nil,
			Interface:   if_,
		})
		if err != nil {
			bail(err.Error(), 2)
		}
	}
}
