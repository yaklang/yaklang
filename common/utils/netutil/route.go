package netutil

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/gopacket/gopacket/routing"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil/netroute"
	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

func FindInterfaceByIP(ip string) (net.Interface, error) {
	ipOriginIns := net.ParseIP(ip)
	ifs, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}

	for _, i := range ifs {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipIns, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			if ipIns.String() == ipOriginIns.String() {
				return i, nil
			}
		}
	}
	return net.Interface{}, utils.Errorf("cannot fetch net.Interface{} by: %v", ip)
}

func IsPrivateIPString(target string) bool {
	return utils.IsPrivateIP(net.ParseIP(utils.FixForParseIP(target)))
}

var (
	DarwinGetawayExtractorRe   = regexp2.MustCompile(`gateway: ([\[\]0-9a-fA-TaskFunc:\.]+)`, regexp2.IgnoreCase|regexp2.Multiline)
	DarwinInterfaceExtractorRe = regexp2.MustCompile(`interface: ([^\s]+)`, regexp2.IgnoreCase|regexp2.Multiline)
)

var (
	notifyOnce = new(sync.Once)
)

//func GetLoopbackDevName() (string, error) {
//	devs, err := pcap.FindAllDevs()
//	if err != nil {
//		return "", utils.Errorf("cannot find pcap ifaceDevs: %v", err)
//	}
//	for _, d := range devs { // 尝试获取本地回环网卡
//		for _, addr := range d.Addresses {
//			if addr.IP.IsLoopback() {
//				return d.Name, nil
//			}
//		}
//		if strings.Contains(strings.ToLower(d.Description), "adapter for loopback traffic capture") {
//			return d.Name, nil
//		}
//		if net.Flags(uint(d.Flags))&net.FlagLoopback == 1 {
//			return d.Name, nil
//		}
//	}
//	return "", utils.Errorf("cannot find loopback device")
//}

func GetPublicRouteIfaceName() (string, error) {
	route, _, _, err := GetPublicRoute()
	if err != nil {
		return "", err
	}
	return route.Name, nil
}

func GetPublicRoute() (*net.Interface, net.IP, net.IP, error) {
	iface, gw, ip, err := Route(5*time.Second, "8.8.8.8")
	if err != nil {
		return nil, nil, nil, err
	}
	notifyOnce.Do(func() {
		log.Infof("public interface network: %v gw: %v local: %v", iface.Name, gw.String(), ip.String())
	})
	return iface, gw, ip, nil
}

func GetPublicHost() (net.IP, error) {
	iface, gw, ip, err := Route(5*time.Second, "8.8.8.8")
	if err != nil {
		return nil, err
	}
	notifyOnce.Do(func() {
		log.Infof("public interface network: %v gw: %v local: %v", iface.Name, gw.String(), ip.String())
	})
	return ip, nil
}

func Route(timeout time.Duration, target string) (iface *net.Interface, gateway, preferredSrc net.IP, err error) {
	var addr = target
	if !utils.IsIPv4(target) && !utils.IsIPv6(target) {
		host, _, _ := utils.ParseStringToHostPort(target)
		if host != "" {
			target = host
		}
		// 针对域名，先去解析一下
		log.Infof("fetching %v 's address for %s", target, timeout.String())
		addr = netx.LookupFirst(target, netx.WithTimeout(timeout))
		if addr == "" {
			err = errors.Errorf("cannot found domain[%s]'s ip address", target)
			return nil, nil, nil, err
		}
	}

	if strings.HasSuffix(addr, ".0") {
		addr = addr[:len(addr)-2] + ".1"
	}
	ip := net.ParseIP(utils.FixForParseIP(addr))
	if ip == nil {
		err = errors.Errorf("ip: %v is invalid", ip)
		return nil, nil, nil, err
	}

	nativeRoute := func() (*net.Interface, net.IP, net.IP, error) {
		log.Debugf("start to call nativeCall netroute for %v", runtime.GOOS)
		route, err := netroute.New()
		if err != nil {
			return nil, nil, nil, err
		}
		log.Debugf("start to find route for %s in %v", ip, runtime.GOOS)
		interfaceIns, gateway, srcIP, err := route.Route(ip)
		if err != nil {
			return nil, nil, nil, err
		}
		if interfaceIns == nil {
			_ret, err := FindInterfaceByIP(srcIP.String())
			if err != nil {
				return nil, nil, nil, err
			}
			interfaceIns = &_ret
		}
		log.Debugf("finished for finding gateway: %s, iface: %v srcIP: %v", gateway, interfaceIns.Name, srcIP.String())
		return interfaceIns, gateway, srcIP, nil
	}
	ifIns, ip1, ip2, err := nativeRoute()
	if err == nil {
		return ifIns, ip1, ip2, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch runtime.GOOS {
	case "linux":
		fallback := func() (*net.Interface, net.IP, net.IP, error) {
			log.Infof("using gopacket finding route to: %s", ip)
			router, err := routing.New()
			if err != nil {
				err = errors.Errorf("get route failed: %s", err)
				return nil, nil, nil, err
			}

			return router.Route(ip)
		}

		log.Infof("start to find iproute2 utils...")
		//ipUtil, err := exec.LookPath("ip")
		//if err != nil {
		//	log.Infof("start to find iproute2 utils... failed: %s", err)
		//	return fallback()
		//}

		cmd := exec.CommandContext(ctx, "ip", "route", "get", target)
		raw, err := cmd.CombinedOutput()
		if err != nil {
			log.Infof("exec iproute2 utils... failed: %s", err)
			return fallback()
		}

		result := Grok(string(raw), `(local +)?(%{IPORHOST:target} +)?( +via +)?%{IPORHOST:gateway} +dev +%{WORD:iface} +src +%{IP:ifaceIp}`)
		routeTarget := result.Get("target")
		_ = routeTarget
		//if routeTarget != target {
		//
		//	return fallback()
		//}

		gatewayIp := result.Get("gateway")
		ifaceName := result.Get("iface")
		ifaceIp := result.Get("ifaceIp")

		log.Infof("iproute2 found iface: %v ifaceIp: %s gIp: %s", ifaceName, ifaceIp, gatewayIp)
		iface, err = net.InterfaceByName(ifaceName)
		if err != nil {
			log.Infof("open net.InterfaceByName: %v failed: %v", iface, err)
			log.Infof("iproute failed: %s", string(raw))
			return fallback()
		}

		iface, gIp, sIp := iface, net.ParseIP(gatewayIp), net.ParseIP(ifaceIp)
		if gIp == nil || sIp == nil {
			return fallback()
		}
		return iface, gIp, sIp, nil
	case "openbsd", "darwin":
		log.Infof("cannot call native route calling, use /sbin/route -n get " + ip.String())
		cmd := exec.CommandContext(ctx, "/sbin/route", "-n", "get", ip.String())
		result, err := cmd.CombinedOutput()
		if err != nil {
			err = errors.Errorf("[route -n get %v] failed: %s", ip.String(), err)
			return nil, nil, nil, err
		}

		resultStr := string(result)
		match, err := DarwinGetawayExtractorRe.FindStringMatch(resultStr)
		if err != nil {
			return nil, nil, nil, errors.Errorf("find match failed: %s", err)
		}

		var (
			targetGateway net.IP
			iface         *net.Interface
			srcIp         net.IP
		)
		if match != nil {
			if getawayIp := match.GroupByNumber(1); getawayIp != nil {
				targetGateway = net.ParseIP(utils.FixForParseIP(getawayIp.String()))
			}
		}

		if targetGateway == nil {
			targetGateway = net.ParseIP(utils.FixForParseIP(target))
		}

		if targetGateway == nil {
			return nil, nil, nil, utils.Error("getaway is invalid/empty")
		}

		match, err = DarwinInterfaceExtractorRe.FindStringMatch(resultStr)
		if err != nil {
			return nil, nil, nil, errors.Errorf("find interface failed: %s", err)
		}
		if match == nil {
			return nil, nil, nil, errors.New("no match found for interface")
		}

		if ifaceName := match.GroupByNumber(1); ifaceName != nil {
			iface, err = net.InterfaceByName(ifaceName.String())
			if err != nil {
				return nil, nil, nil, errors.Errorf("get iface failed: %s", err)
			}

			addrs, err := iface.Addrs()
			if err != nil {
				return nil, nil, nil, errors.Errorf("iface: %v cannot get address: %s", iface.Name, err)
			}
			for _, addr := range addrs {
				raw := utils.FixForParseIP(addr.String())
				srcIpAddress, _, err := net.ParseCIDR(raw)
				if err != nil {
					continue
				}
				if utils.IsIPv6(srcIpAddress.String()) == utils.IsIPv6(targetGateway.String()) {
					srcIp = srcIpAddress
				}
			}
		} else {
			return nil, nil, nil, errors.New("cannot found interface ip")
		}

		return iface, targetGateway, srcIp, err
	default:
		var handleRoute = func(rs []routewrapper.Route) (*net.Interface, net.IP, net.IP, error) {
			// 找到目标IP的最具体的路由
			targetIP := net.ParseIP(utils.FixForParseIP(target))
			if targetIP == nil {
				return nil, nil, nil, utils.Errorf("invalid target IP: %s", target)
			}

			log.Debugf("Selecting route for target IP: %s from %d routes", targetIP.String(), len(rs))
			for i, route := range rs {
				if route.Destination.IP == nil || route.Destination.Mask == nil {
					continue
				}

				ones, _ := route.Destination.Mask.Size()
				gateway := "none"
				if route.Gateway != nil {
					gateway = route.Gateway.String()
				}
				log.Debugf("Route[%d]: interface=%s, network=%s/%d, gateway=%s",
					i, route.Interface.Name, route.Destination.IP.String(), ones, gateway)
			}

			// 跟踪两个最佳匹配的路由：一个是总体最佳匹配的路由，另一个是最佳直接连接的网络
			// 直连的优先
			var bestRoute *routewrapper.Route
			var bestDirectRoute *routewrapper.Route
			var bestMaskBits int = -1
			var bestDirectMaskBits int = -1

			// 第一次遍历：找到包含目标IP的路由，优先考虑具体性
			for i, route := range rs {
				if route.Destination.IP == nil || route.Destination.Mask == nil {
					continue
				}

				// 检查此路由的网络是否包含我们的目标IP
				if route.Destination.Contains(targetIP) {
					ones, _ := route.Destination.Mask.Size()

					// 首先检查此路由是否是直接连接的网络
					isDirect := false
					for _, iface := range rs {
						if iface.Interface.Name == route.Interface.Name {
							interfaceAddrs, _ := iface.Interface.Addrs()
							for _, addr := range interfaceAddrs {
								ipNet, ok := addr.(*net.IPNet)
								if !ok {
									continue
								}

								// 如果接口有一个IP在同一子网中
								if ipNet.IP.Mask(ipNet.Mask).Equal(route.Destination.IP.Mask(route.Destination.Mask)) {
									isDirect = true
									break
								}
							}
						}
						if isDirect {
							break
						}
					}

					if isDirect {
						// 如果这个路由更具体，更新最佳直接路由
						if ones > bestDirectMaskBits {
							bestDirectMaskBits = ones
							bestDirectRoute = &rs[i]
							log.Debugf("New best direct route: interface=%s, network=%s/%d",
								route.Interface.Name, route.Destination.IP.String(), ones)
						}
					} else {
						// 如果这个路由更具体，更新最佳网关路由
						if ones > bestMaskBits {
							bestMaskBits = ones
							bestRoute = &rs[i]
							log.Debugf("New best gateway route: interface=%s, network=%s/%d, gateway=%s",
								route.Interface.Name, route.Destination.IP.String(), ones, route.Gateway)
						}
					}
				}
			}

			// 优先选择直接连接的网络而不是网关路由
			var selectedRoute *routewrapper.Route
			if bestDirectRoute != nil {
				selectedRoute = bestDirectRoute
				log.Debugf("Selected direct route: interface=%s, network=%s/%d",
					selectedRoute.Interface.Name, selectedRoute.Destination.IP.String(), bestDirectMaskBits)
			} else if bestRoute != nil {
				selectedRoute = bestRoute
				log.Debugf("Selected gateway route: interface=%s, network=%s/%d",
					selectedRoute.Interface.Name, selectedRoute.Destination.IP.String(), bestMaskBits)
			} else {
				// 如果找不到匹配的路由，尝试默认路由（0.0.0.0/0）
				for i, route := range rs {
					if route.Destination.IP == nil || route.Destination.Mask == nil {
						continue
					}

					ones, bits := route.Destination.Mask.Size()
					if ones == 0 && bits > 0 {
						selectedRoute = &rs[i]
						log.Debugf("Selected default route: interface=%s, gateway=%s",
							selectedRoute.Interface.Name, selectedRoute.Gateway)
						break
					}
				}
			}

			if selectedRoute == nil {
				return nil, nil, nil, utils.Errorf("no suitable route found for: %s", target)
			}

			// 从接口获取源IP
			var srcIP net.IP
			addrs, err := selectedRoute.Interface.Addrs()
			if err != nil {
				return nil, nil, nil, errors.Errorf("interface %v cannot get addresses: %s",
					selectedRoute.Interface.Name, err)
			}

			// 在接口上找到一个合适的源IP
			for _, addr := range addrs {
				ipAddr, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					continue
				}

				// 匹配IP版本（IPv4或IPv6）
				isIPv6Target := utils.IsIPv6(targetIP.String())
				isIPv6Addr := utils.IsIPv6(ipAddr.String())

				if isIPv6Target == isIPv6Addr {
					srcIP = ipAddr
					break
				}
			}

			if srcIP == nil && len(addrs) > 0 {
				// 如果找不到匹配的IP版本，使用第一个地址
				ipAddr, _, err := net.ParseCIDR(addrs[0].String())
				if err == nil {
					srcIP = ipAddr
				}
			}

			log.Debugf("Final route selected: interface=%s, gateway=%s, srcIP=%s for target=%s",
				selectedRoute.Interface.Name, selectedRoute.Gateway, srcIP, target)

			return selectedRoute.Interface, selectedRoute.Gateway, srcIP, nil
		}

		r, err := routewrapper.NewRouteWrapper()
		if err != nil {
			return nil, nil, nil, utils.Errorf("windows err: %v", err)
		}
		routes, err := r.Routes()
		if err != nil {
			return nil, nil, nil, utils.Errorf("fetch routes failed: %s", err)
		}

		ifaceIns, gatewayIns, ipLocal, err := handleRoute(routes)
		if err != nil {
			log.Infof("No specific route found for %s, trying default routes", target)
			routes, err = r.DefaultRoutes()
			if err != nil {
				return nil, nil, nil, utils.Errorf("get default routes failed: %s", err)
			}
			return handleRoute(routes)
		}
		return ifaceIns, gatewayIns, ipLocal, nil
	}
}
