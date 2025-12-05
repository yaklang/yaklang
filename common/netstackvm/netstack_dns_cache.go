package netstackvm

import (
	"context"
	"github.com/miekg/dns"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type NetStackReserveDNSCache struct {
	ctx context.Context

	aCache     *omap.OrderedMap[string, []string] //  A OR AAA 记录 cache ip -> domain
	cNameCache *omap.OrderedMap[string, []string] // CNAME 记录 cache domain -> domain

}

func (ns *NetStackReserveDNSCache) ReserveResolve(ip string) []string {
	aDomain, ok := ns.aCache.Get(ip)
	if !ok {
		return nil
	}
	cName := ns.findAllCNAMEsForDomains(aDomain)
	allDomain := append(cName, aDomain...)
	return lo.Uniq(allDomain)
}

func (ns *NetStackReserveDNSCache) findAllCNAMEsForDomains(domains []string) []string {
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
func (ns *NetStackReserveDNSCache) findAllRelatedCNAMEsRecursive(currentDomain string, results *[]string, visited map[string]struct{}) {
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

func (ns *NetStackReserveDNSCache) AddACache(ip string, domain string) {
	if _, ok := ns.aCache.Get(ip); !ok {
		ns.aCache.Set(ip, []string{})
	}

	domains, _ := ns.aCache.Get(ip)
	if !lo.Contains(domains, domain) {
		domains = append(domains, domain)
		ns.aCache.Set(ip, domains)
	}
}

func (ns *NetStackReserveDNSCache) AddCNameCache(domain string, cname string) {
	if _, ok := ns.cNameCache.Get(domain); !ok {
		ns.cNameCache.Set(domain, []string{})
	}

	cnames, _ := ns.cNameCache.Get(domain)
	if !lo.Contains(cnames, cname) {
		cnames = append(cnames, cname)
		ns.cNameCache.Set(domain, cnames)
	}
}

func StartNetReserveStackDNSCache(ctx context.Context) (*NetStackReserveDNSCache, error) {
	vm, err := NewSystemNetStackVM(WithPcapCapabilities(stack.CapabilityRXChecksumOffload), WithForceSystemNetStack(true))
	if err != nil {
		return nil, err
	}
	sniffer := NewNetstackSniffer(vm)
	m := &NetStackReserveDNSCache{
		aCache:     omap.NewOrderedMap[string, []string](map[string][]string{}),
		cNameCache: omap.NewOrderedMap[string, []string](map[string][]string{}),
		ctx:        ctx,
	}

	localIP := make([]string, 0)
	for _, entry := range vm.entries {
		localIP = append(localIP, entry.mainNICIPv4Address.String())
	}

	checkLocalIP := func(ip string) bool {
		for _, local := range localIP {
			if ip == local {
				return true
			}
		}
		return false
	}

	sniffer.RegisterSniffHandle(header.TCPProtocolNumber, func(buffer *stack.PacketBuffer) {
		ipHeader := header.IPv4(buffer.NetworkHeader().Slice())
		if !ipHeader.IsValid(buffer.Size()) {
			return
		}
		srcIP := ipHeader.SourceAddress().String()
		dstIP := ipHeader.DestinationAddress().String()
		targetIP := dstIP
		if checkLocalIP(dstIP) {
			if checkLocalIP(srcIP) {
				return
			} else {
				targetIP = srcIP
			}
		}
		tcpPayload := GetTransportPayload(buffer)
		if serverName := ReadServerName(tcpPayload); serverName != "" {
			m.AddACache(targetIP, serverName)
		}
	})

	sniffer.RegisterSniffHandle(header.UDPProtocolNumber, func(buffer *stack.PacketBuffer) {
		ipHeader := header.IPv4(buffer.NetworkHeader().Slice())
		if !ipHeader.IsValid(buffer.Size()) {
			return
		}

		tcpHeader := header.UDP(buffer.TransportHeader().Slice())
		srcPort := tcpHeader.SourcePort()
		dstPort := tcpHeader.DestinationPort()

		if srcPort == 53 || dstPort == 53 {
			msg := new(dns.Msg)
			err := msg.Unpack(GetTransportPayload(buffer))
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

	})
	return m, nil
}
