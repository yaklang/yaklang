// Copyright 2012 Google, Inc. All rights reserved.
//
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE file in the root of the source
// tree.

// Originally found in
// https://github.com/google/gopacket/blob/master/routing/routing.go
//   - Route selection modified to choose most selective route
//     to break ties when route priority is insufficient.
package netroute

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	bits2 "math/bits"
	"net"
	"strings"
)

// rtInfo contains information on a single route.
type rtInfo struct {
	Src, Dst         *net.IPNet
	Gateway, PrefSrc net.IP
	// We currently ignore the InputIface.
	InputIface, OutputIface uint32
	Priority                uint32
}

// routeSlice implements sort.Interface to sort routes by Priority.
type routeSlice []*rtInfo

func (r routeSlice) Len() int {
	return len(r)
}
func (r routeSlice) Less(i, j int) bool {
	return r[i].Priority < r[j].Priority
}
func (r routeSlice) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

type router struct {
	ifaces map[int]net.Interface
	addrs  map[int]ipAddrs
	v4, v6 routeSlice
}

func (r *router) String() string {
	strs := []string{"ROUTER", "--- V4 ---"}
	for _, route := range r.v4 {
		strs = append(strs, fmt.Sprintf("%+v", *route))
	}
	strs = append(strs, "--- V6 ---")
	for _, route := range r.v6 {
		strs = append(strs, fmt.Sprintf("%+v", *route))
	}
	return strings.Join(strs, "\n")
}

type ipAddrs struct {
	v4, v6 net.IP
}

func (r *router) Route(dst net.IP) (iface *net.Interface, gateway, preferredSrc net.IP, err error) {
	return r.RouteWithSrc(nil, nil, dst)
}

func calcMaskSize(mask net.IPMask) (int, int) {
	ones, bits := mask.Size()
	if bits <= 0 {
		if len(mask) == 4 {
			bits = 32
			maskInt := uint32(mask[0])<<24 | uint32(mask[1])<<16 | uint32(mask[2])<<8 | uint32(mask[3])
			for i := 0; i < 32; i++ {
				if maskInt&1 == 1 {
					ones++
				} else {
					break
				}
				maskInt >>= 1
			}
			return ones, bits
		} else if len(mask) == 16 {
			bits = 128
			highMaskInt := uint64(mask[0])<<56 | uint64(mask[1])<<48 | uint64(mask[2])<<40 | uint64(mask[3])<<32 |
				uint64(mask[4])<<24 | uint64(mask[5])<<16 | uint64(mask[6])<<8 | uint64(mask[7])
			lowMaskInt := uint64(mask[8])<<56 | uint64(mask[9])<<48 | uint64(mask[10])<<40 | uint64(mask[11])<<32 |
				uint64(mask[12])<<24 | uint64(mask[13])<<16 | uint64(mask[14])<<8 | uint64(mask[15])
			for i := 0; i < 64; i++ {
				if highMaskInt&1 == 1 {
					ones++
				} else {
					break
				}
				highMaskInt >>= 1
			}
			if ones == 64 {
				for i := 0; i < 64; i++ {
					if lowMaskInt&1 == 1 {
						ones++
					} else {
						break
					}
					lowMaskInt >>= 1
				}
			}
			return ones, bits
		}
	}
	return ones, bits
}

func (r *router) RouteWithSrc(input net.HardwareAddr, src, dst net.IP) (iface *net.Interface, gateway, preferredSrc net.IP, err error) {
	var ifaceIndex int
	switch {
	case dst.To4() != nil:
		ifaceIndex, gateway, preferredSrc, err = r.route(false, r.v4, input, src.To4(), dst.To4())
	case dst.To16() != nil:
		ifaceIndex, gateway, preferredSrc, err = r.route(true, r.v6, input, src.To16(), dst.To16())
	default:
		err = errors.New("IP is not valid as IPv4 or IPv6")
		return
	}
	if err != nil {
		return
	}

	// Interfaces are 1-indexed, but we store them in a 0-indexed array.
	correspondingIface, ok := r.ifaces[ifaceIndex]
	if !ok {
		err = errors.New("Route refereced unknown interface")
	}
	iface = &correspondingIface

	if preferredSrc == nil {
		switch {
		case dst.To4() != nil:
			preferredSrc = r.addrs[ifaceIndex].v4
		case dst.To16() != nil:
			preferredSrc = r.addrs[ifaceIndex].v6
		}
	}
	return
}

func (r *router) route(isV6 bool, routes routeSlice, input net.HardwareAddr, src, dst net.IP) (iface int, gateway, preferredSrc net.IP, err error) {
	var inputIndex uint32
	if input != nil {
		for i, iface := range r.ifaces {
			if bytes.Equal(input, iface.HardwareAddr) {
				// Convert from zero- to one-indexed.
				inputIndex = uint32(i + 1)
				break
			}
		}
	}
	var mostSpecificRt *rtInfo

	for _, rt := range routes {
		if rt.InputIface != 0 && rt.InputIface != inputIndex {
			continue
		}
		if src != nil && rt.Src != nil && !rt.Src.Contains(src) {
			continue
		}
		if rt.Dst != nil && !rt.Dst.Contains(dst) {
			continue
		}
		if mostSpecificRt != nil {
			var candSpec, curSpec int
			if rt.Dst != nil {
				if !isV6 && len(rt.Dst.Mask) > 4 {
					ret := funk.Reverse(utils.CopyBytes(rt.Dst.Mask)).([]byte)
					maskInt, _ := utils.IPv4ToUint32(net.IPv4(ret[3], ret[2], ret[1], ret[0]).To4())
					candSpec = bits2.OnesCount32(maskInt)
				} else {
					candSpec, _ = rt.Dst.Mask.Size()
				}
			}
			if mostSpecificRt.Dst != nil {
				if !isV6 && len(mostSpecificRt.Dst.Mask) > 4 {
					ret := funk.Reverse(utils.CopyBytes(mostSpecificRt.Dst.Mask)).([]byte)
					maskInt, _ := utils.IPv4ToUint32(net.IPv4(ret[3], ret[2], ret[1], ret[0]).To4())
					curSpec = bits2.OnesCount32(maskInt)
				} else {
					curSpec, _ = mostSpecificRt.Dst.Mask.Size()
				}
			}

			if candSpec < curSpec {
				continue
			}
			log.Infof("found new route to %v, mask size: %v(>=%v), use out-iface: %v(%v->%v)", dst.String(), candSpec, curSpec, rt.OutputIface, rt.Src, rt.Dst)
		}
		mostSpecificRt = rt
	}
	if mostSpecificRt != nil {
		return int(mostSpecificRt.OutputIface), mostSpecificRt.Gateway, mostSpecificRt.PrefSrc, nil
	}

	err = fmt.Errorf("no route found for %v", dst)
	return
}
