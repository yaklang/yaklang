// Copyright 2012 Google, Inc. All rights reserved.
//
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file in the root of the source
// tree.

//go:build darwin || dragonfly || freebsd || netbsd || openbsd
// +build darwin dragonfly freebsd netbsd openbsd

// This is a BSD import for the routing structure initially found in
// https://github.com/gopacket/gopacket/blob/master/routing/routing.go
// RIB parsing follows the BSD route format described in
// https://github.com/freebsd/freebsd/blob/master/sys/net/route.h
package netroute

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"net"
	"sort"
	"syscall"

	"github.com/gopacket/gopacket/routing"
	"golang.org/x/net/route"
)

func toIPAddr(a syscall.Sockaddr) (net.IP, error) {
	switch t := a.(type) {
	case *syscall.SockaddrInet4:
		ip := net.IPv4(t.Addr[0], t.Addr[1], t.Addr[2], t.Addr[3])
		return ip, nil
	case *syscall.SockaddrInet6:
		ip := make(net.IP, net.IPv6len)
		copy(ip, t.Addr[:])
		return ip, nil
	default:
		return net.IP{}, fmt.Errorf("unknown family: %v", t)
	}
}

// selected BSD Route flags.
const (
	RTF_UP        = 0x1
	RTF_GATEWAY   = 0x2
	RTF_HOST      = 0x4
	RTF_REJECT    = 0x8
	RTF_DYNAMIC   = 0x10
	RTF_MODIFIED  = 0x20
	RTF_STATIC    = 0x800
	RTF_BLACKHOLE = 0x1000
	RTF_LOCAL     = 0x200000
	RTF_BROADCAST = 0x400000
	RTF_MULTICAST = 0x800000
	RTF_IFSCOPE   = 0x1000000
)

func New() (routing.Router, error) {
	rtr := &router{}
	rtr.ifaces = make(map[int]net.Interface)
	rtr.addrs = make(map[int]ipAddrs)
	tab, err := route.FetchRIB(syscall.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	msgs, err := syscall.ParseRoutingMessage(tab)
	if err != nil {
		return nil, err
	}
	var ipn *net.IPNet
	for _, msg := range msgs {
		m := msg.(*syscall.RouteMessage)
		// We ignore the error (m.Err) here. It's not clear what this error actually means,
		// and it makes us miss routes that _should_ be included.
		routeInfo := new(rtInfo)
		if int(m.Header.Version) < 3 || m.Header.Version > 5 {
			return nil, fmt.Errorf("unexpected RIB message version: %d", m.Header.Type)
		}
		if m.Header.Type != syscall.RTM_ADD && m.Header.Type != syscall.RTM_GET { // 修正为检查 RTM_ADD 和 RTM_GET
			log.Debugf("Unexpected RIB message type: %d, skipping.", m.Header.Type)
			continue
		}
		if m.Header.Flags&RTF_UP == 0 ||
			m.Header.Flags&(RTF_REJECT|RTF_BLACKHOLE) != 0 {
			continue
		}
		sockaddrs, err := syscall.ParseRoutingSockaddr(m)
		if err != nil {
			log.Debugf("Failed to parse Sockaddrs from RouteMessage data: %v, skipping. Message: %+v", err, m)
			continue
		}
		routeInfo.Priority = m.Header.Rmx.Hopcount
		dst, err := toIPAddr(sockaddrs[0])
		if err == nil {
			mask, _ := toIPAddr(sockaddrs[2])
			if mask == nil {
				mask = net.IP(net.CIDRMask(0, 8*len(dst)))
			}
			ipn = &net.IPNet{IP: dst, Mask: net.IPMask(mask)}
			if m.Header.Flags&RTF_HOST != 0 {
				ipn.Mask = net.CIDRMask(8*len(ipn.IP), 8*len(ipn.IP))
			}
			if m.Header.Flags&RTF_IFSCOPE != 0 {
				routeInfo.IsScoped = true
			}
			routeInfo.Dst = ipn
		} else {
			return nil, fmt.Errorf("unexpected RIB destination: %v", err)
		}
		if m.Header.Flags&RTF_GATEWAY != 0 {
			if gw, err := toIPAddr(sockaddrs[1]); err == nil {
				routeInfo.Gateway = gw
			}
		}
		if src, err := toIPAddr(sockaddrs[5]); err == nil {
			ipn = &net.IPNet{IP: src, Mask: net.CIDRMask(8*len(src), 8*len(src))}
			routeInfo.Src = ipn
			routeInfo.PrefSrc = src
			if m.Header.Flags&0x2 != 0 /* RTF_GATEWAY */ {
				routeInfo.Src.Mask = net.CIDRMask(0, 8*len(routeInfo.Src.IP))
			}
		}
		routeInfo.OutputIface = uint32(m.Header.Index)
		switch sockaddrs[0].(type) {
		case *syscall.SockaddrInet4:
			if routeInfo.Dst.IP.Equal(net.ParseIP("0.0.0.0")) {
				rtr.defaultRouteV4 = append(rtr.defaultRouteV4, routeInfo)
				continue
			}
			rtr.v4 = append(rtr.v4, routeInfo)
		case *syscall.SockaddrInet6:
			if routeInfo.Dst.IP.Equal(net.ParseIP("::")) {
				rtr.defaultRouteV6 = append(rtr.defaultRouteV6, routeInfo)
				continue
			}
			rtr.v6 = append(rtr.v6, routeInfo)
		}
	}
	sort.Sort(rtr.v4)
	sort.Sort(rtr.v6)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		rtr.ifaces[iface.Index] = iface
		var addrs ipAddrs
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range ifaceAddrs {
			if inet, ok := addr.(*net.IPNet); ok {
				// Go has a nasty habit of giving you IPv4s as ::ffff:1.2.3.4 instead of 1.2.3.4.
				// We want to use mapped v4 addresses as v4 preferred addresses, never as v6
				// preferred addresses.
				if v4 := inet.IP.To4(); v4 != nil {
					if addrs.v4 == nil {
						addrs.v4 = v4
					}
				} else if addrs.v6 == nil {
					addrs.v6 = inet.IP
				}
			}
		}
		rtr.addrs[iface.Index] = addrs
	}
	return rtr, nil
}
