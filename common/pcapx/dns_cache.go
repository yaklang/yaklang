package pcapx

import (
	"context"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
)

type PcapReserveDNSCache struct {
	ctx context.Context

	aCache     *omap.OrderedMap[string, []string] //  A OR AAA 记录 cache ip -> domain
	cNameCache *omap.OrderedMap[string, []string] // CNAME 记录 cache domain -> domain

}

func (ns *PcapReserveDNSCache) ReserveResolve(ip string) []string {
	aDomain, ok := ns.aCache.Get(ip)
	if !ok {
		return nil
	}
	cName := ns.findAllCNAMEsForDomains(aDomain)
	allDomain := append(cName, aDomain...)
	return lo.Uniq(allDomain)
}

func (ns *PcapReserveDNSCache) findAllCNAMEsForDomains(domains []string) []string {
	allResults := make([]string, len(domains))
	allResults = append(allResults, domains...)
	for _, domain := range domains {
		var singleDomainResults []string
		visited := make(map[string]struct{})
		ns.findAllRelatedCNAMEsRecursive(domain, &singleDomainResults, visited)
		allResults = append(allResults, singleDomainResults...)
	}
	return allResults
}

// findAllRelatedCNAMEsRecursive 私有递归辅助函数保持不变。
func (ns *PcapReserveDNSCache) findAllRelatedCNAMEsRecursive(currentDomain string, results *[]string, visited map[string]struct{}) {
	if _, ok := visited[currentDomain]; ok {
		return
	}
	visited[currentDomain] = struct{}{}
	canonicalDomains, ok := ns.cNameCache.Get(currentDomain)
	if !ok || len(canonicalDomains) == 0 {
		return
	}
	nextDomain := canonicalDomains[0]
	*results = append(*results, nextDomain)
	ns.findAllRelatedCNAMEsRecursive(nextDomain, results, visited)
}

func (ns *PcapReserveDNSCache) AddACache(ip string, domain string) {
	if _, ok := ns.aCache.Get(ip); !ok {
		ns.aCache.Set(ip, []string{})
	}

	domains, _ := ns.aCache.Get(ip)
	if !lo.Contains(domains, domain) {
		domains = append(domains, domain)
		ns.aCache.Set(ip, domains)
	}
}

func (ns *PcapReserveDNSCache) AddCNameCache(domain string, cname string) {
	if _, ok := ns.cNameCache.Get(domain); !ok {
		ns.cNameCache.Set(domain, []string{})
	}

	cnames, _ := ns.cNameCache.Get(domain)
	if !lo.Contains(cnames, cname) {
		cnames = append(cnames, cname)
		ns.cNameCache.Set(domain, cnames)
	}
}

func GetSystemLocalIP(interf net.Interface) (net.IP, net.IP, net.IPMask) {
	addrs, err := interf.Addrs()
	if err != nil {
		return nil, nil, nil
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipv4 := ipnet.IP.To4(); ipv4 != nil {
			// 计算网关地址 - 使用网段的第一个地址作为网关
			gateway := make(net.IP, len(ipv4))
			copy(gateway, ipv4)

			// 通过掩码计算网段第一个地址作为网关
			for i := range gateway {
				gateway[i] = ipv4[i] & ipnet.Mask[i]
			}
			gateway[3]++

			return ipv4, gateway, ipnet.Mask
		}
	}
	return nil, nil, nil
}

func StartReserveDNSCache(ctx context.Context) (*PcapReserveDNSCache, error) {
	startDeviceList := make([]string, 0)
	selectDevice, _ := netutil.GetPublicRouteIfaceName()
	allNic, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	m := &PcapReserveDNSCache{
		aCache:     omap.NewOrderedMap[string, []string](map[string][]string{}),
		cNameCache: omap.NewOrderedMap[string, []string](map[string][]string{}),
		ctx:        ctx,
	}

	localIP := make([]string, 0)
	for _, nic := range allNic {
		if nic.Flags&net.FlagRunning == 0 { // just get the running interface
			continue
		}

		if nic.Flags&net.FlagLoopback == 0 && nic.Name != selectDevice { // if not loopback and not public interface, skip it
			continue
		}
		startDeviceList = append(startDeviceList, nic.Name)
		currentLocalIp, _, _ := GetSystemLocalIP(nic)
		if currentLocalIp != nil {
			localIP = append(localIP, currentLocalIp.String())
		}
	}

	checkLocalIP := func(ip string) bool {
		for _, local := range localIP {
			if ip == local {
				return true
			}
		}
		return false
	}

	var start = make(chan struct{})
	go func() {
		err = pcaputil.Start(
			pcaputil.WithContext(ctx),
			pcaputil.WithCaptureStartedCallback(func() {
				close(start)
			}),
			pcaputil.WithDevice(startDeviceList...),
			pcaputil.WithTLSClientHello(func(flow *pcaputil.TrafficFlow, hello *tlsutils.HandshakeClientHello) {
				srcIP := flow.ClientConn.LocalAddr().String()
				dstIP := flow.ClientConn.RemoteAddr().String()
				targetIP := dstIP
				if checkLocalIP(dstIP) {
					if checkLocalIP(srcIP) {
						return
					} else {
						targetIP = srcIP
					}
				}
				if serverName := hello.SNI(); serverName != "" {
					m.AddACache(targetIP, serverName)
				}
			}),
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				ret, isOk := packet.TransportLayer().(*layers.UDP)
				if !isOk || ret == nil {
					return
				}

				if ret.DstPort == 53 || ret.SrcPort == 53 {
					msg := new(dns.Msg)
					err := msg.Unpack(ret.Payload)
					if err != nil {
						return
					}

					for _, answer := range msg.Answer {
						switch t := answer.(type) {
						case *dns.A:
							m.AddACache(t.A.String(), t.Hdr.Name)
						case *dns.CNAME:
							m.AddCNameCache(t.Target, t.Hdr.Name)
						case *dns.AAAA:
							m.AddACache(t.AAAA.String(), t.Hdr.Name)
						default:
						}
					}
				}
			}),
		)
		log.Errorf("pcap reserve dns cache exited: %s", err)
	}()
	select {
	case <-start:
	case <-ctx.Done():
		return nil, utils.Errorf("pcap reserve dns context done before start: %s", ctx.Err())
	}
	return m, nil
}
