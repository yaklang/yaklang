package yakgrpc

import (
	"fmt"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"strings"
	"sync"
	"time"
)

func (s *Server) DiagnoseNetwork(req *ypb.DiagnoseNetworkRequest, server ypb.Yak_DiagnoseNetworkServer) error {
	var wg sync.WaitGroup
	timeout := utils.FloatSecondDuration(req.GetNetworkTimeout())
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	var addNetworkDiagnose = func(title string, diagnoseType string, desc string, level ...string) {
		// diagnose type:
		// 1. route
		// 2. network
		// 3. transport
		// 4. log
		var loglevel string
		if len(level) > 0 {
			loglevel = strings.ToLower(level[0])
		}
		server.Send(&ypb.DiagnoseNetworkResponse{
			Title:          title,
			DiagnoseType:   diagnoseType,
			DiagnoseResult: desc,
			LogLevel:       loglevel,
		})
	}
	var _addDiagnoseLog = func(level, message string) {
		addNetworkDiagnose("diagnose-log", "log", message, level)
	}
	var info = func(message string) {
		_addDiagnoseLog("info", message)
	}
	var warning = func(message string) {
		_addDiagnoseLog("warning", message)
	}

	var ipChannel = make(chan string, 1000)
	var ipPortChannel = make(chan string, 1000)
	var domainChannel = make(chan string, 1000)

	routeFilter := filter.NewFilter()
	route := func(ip string, verbose ...string) {
		if routeFilter.Exist(ip) {
			return
		}
		routeFilter.Insert(ip)

		var suffix string
		if len(verbose) > 0 {
			suffix = fmt.Sprintf("(%v)", strings.Join(verbose, ", "))
		}

		iface, gw, _, err := netutil.Route(timeout, ip)
		if err != nil {
			warning(fmt.Sprintf("Route[%v] failed: %v", ip, err.Error()))
			return
		}
		var lines []string
		if iface != nil {
			lines = append(lines, fmt.Sprintf("Interface[%d]: %v", iface.Index, iface.Name))
			lines = append(lines, "Mac(hardware): "+iface.HardwareAddr.String())
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				lines = append(lines, "Address: "+a.String())
			}
			lines = append(lines, "")
		}

		if gw.IsLoopback() {
			lines = append(lines, "Gateway: Loopback")
		} else {
			lines = append(lines, fmt.Sprintf("Gateway: %v", gw.String()))
		}
		if len(lines) > 0 {
			addNetworkDiagnose(
				fmt.Sprintf("route-diagnose[%#v%v]", ip, suffix),
				"route",
				strings.Join(lines, "\n"),
			)
		}
	}

	wg.Add(4)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		f := filter.NewFilter()
		for domain := range domainChannel {
			if f.Exist(domain) {
				continue
			}
			f.Insert(domain)

			var lines []string
			for _, dnsServer := range req.GetDNSServers() {
				ips := utils.GetIPsFromHostWithTimeout(timeout, domain, []string{dnsServer})
				for _, i := range ips {
					if utils.IsIPv4(i) {
						lines = append(lines, fmt.Sprintf("%v =>    [A]: %v", dnsServer, i))
					} else {
						lines = append(lines, fmt.Sprintf("%v => [AAAA]: %v", dnsServer, i))
					}
					route(i, fmt.Sprintf("%v @%v", domain, dnsServer))
				}
			}
			systemDNS, err := utils.GetSystemDnsServers()
			if err != nil {
				warning("Get system dns servers failed: " + err.Error())
				continue
			}
			for _, dnsServer := range systemDNS {
				ips := utils.GetIPsFromHostWithTimeout(timeout, domain, []string{dnsServer})
				for _, i := range ips {
					if utils.IsIPv4(i) {
						lines = append(lines, fmt.Sprintf("SYSTEM: %v =>    [A]: %v", dnsServer, i))
					} else {
						lines = append(lines, fmt.Sprintf("SYSTEM: %v => [AAAA]: %v", dnsServer, i))
					}
					route(i, fmt.Sprintf("%v @%v", domain, dnsServer))
				}
			}
			if len(lines) > 0 {
				addNetworkDiagnose(
					fmt.Sprintf("dns-diagnose[%#v]", domain),
					"dns",
					strings.Join(lines, "\n"),
				)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for ip := range ipChannel {
			route(ip)
		}
	}()

	proxy := req.GetProxy()
	if proxy != "" {
		info("Start to diagnose proxy ...")
		proxyUrl, err := url.Parse(proxy)
		if err != nil {
			warning("Parse proxy url failed: " + err.Error())
		} else {
			proxyUrl.User = url.UserPassword(req.GetProxyAuthUsername(), req.GetProxyAuthUsername())
			proxy = proxyUrl.String()
		}
	}
	go func() {
		defer wg.Done()
		if proxy == "" {
			return
		}

		if req.GetProxyToAddr() == "" {
			warning("ProxyToAddr is empty. (specific a host to test proxy)")
			return
		}
		conn, err := utils.GetForceProxyConn(req.GetProxyToAddr(), proxy, timeout)
		if err != nil {
			warning("Get proxy connection failed: " + err.Error())
			return
		}
		conn.Close()
		addNetworkDiagnose(
			"proxy-diagnose",
			"proxy",
			fmt.Sprintf("Proxy [%v] connection to [%v] success.", proxy, req.GetProxyToAddr()),
		)
	}()

	go func() {
		defer wg.Done()
		f := filter.NewFilter()
		for ipPort := range ipPortChannel {
			if f.Exist(ipPort) {
				continue
			}
			f.Insert(ipPort)
			host, port, _ := utils.ParseStringToHostPort(ipPort)
			if port <= 0 {
				continue
			}

			var lines []string
			conn, err := utils.GetAutoProxyConn(ipPort, proxy, timeout)
			if err != nil {
				warning(fmt.Sprintf("Get tcp connection to [%v:%v] failed: %v", host, port, err.Error()))
				continue
			}
			lines = append(lines, fmt.Sprintf("TCP [%v:%v] connection success.", host, port))

			tlsConn := utils.NewDefaultTLSClient(conn)
			err = tlsConn.HandshakeContext(utils.TimeoutContext(timeout))
			if err == nil {
				lines = append(lines, fmt.Sprintf("TLS [%v:%v] connection success.", host, port))
			}
			conn.Close()
		}
	}()

	info("Initialized Diagnose Network ...")
	for _, i := range utils.PrettifyListFromStringSplitEx(req.GetConnectTarget(), "\n", ",", "|") {
		hosts := utils.ParseStringToHosts(i)
		var h string
		if len(hosts) > 0 {
			h = hosts[0]
		} else {
			h = i
		}
		var params = make(map[string]any)
		params["origin"] = h
		host, port, _ := utils.ParseStringToHostPort(h)
		if port <= 0 {
			// only host
			if utils.IsIPv4(h) || utils.IsIPv6(h) {
				ipChannel <- h
			} else {
				domainChannel <- h
			}
		} else {
			ipPortChannel <- h
			if utils.IsIPv4(host) || utils.IsIPv6(h) {
				ipChannel <- host
			} else {
				domainChannel <- host
			}
		}
	}

	if req.GetDomain() != "" {
		domainChannel <- req.GetDomain()
	}
	defer close(ipPortChannel)
	defer close(domainChannel)
	defer close(ipChannel)
	return nil
}
