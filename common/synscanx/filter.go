package synscanx

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
)

func (s *Scannerx) excludedHost(host string) bool {
	if s.config.excludeHosts != nil {
		if s.config.excludeHosts.Contains(host) {
			return true
		}
	}

	return false
}

func (s *Scannerx) excludedPort(port int) bool {
	if s.config == nil || s.config.excludePorts == nil {
		return false
	}
	// The exclude filter stores the service port. UDP 1-15 is not port<<12.
	_, port = utils.ParsePortToProtoPort(port)
	return s.config.excludePorts.Contains(port)
}

func (s *Scannerx) GetNonExcludedPorts(ports string) []int {
	var nonExcludedPorts []int
	var builder strings.Builder
	for _, port := range utils.ParseStringToPorts(ports) {
		if s.excludedPort(port) {
			continue
		}
		builder.WriteString(strconv.Itoa(port))
		builder.WriteString(",")
		nonExcludedPorts = append(nonExcludedPorts, port)
	}
	s.ports.Add(builder.String())
	if s.config.shuffle {
		utils.ShuffleInt(nonExcludedPorts)
	}
	return nonExcludedPorts
}

func (s *Scannerx) GetNonExcludedHosts(targets string) []string {
	parsed := utils.ParseStringToHosts(targets)
	resolved := resolveScanHosts(parsed)
	var nonExcludedHosts []string
	for _, host := range resolved {
		if s.excludedHost(host) {
			continue
		}
		if utils.IsIPv4(host) {
			nonExcludedHosts = append(nonExcludedHosts, host)
		}
	}
	s.hosts = hostsparser.NewHostsParser(s.ctx, strings.Join(nonExcludedHosts, ","))
	if s.config.shuffle {
		utils.ShuffleString(nonExcludedHosts)
	}
	return nonExcludedHosts
}

// resolveScanHosts keeps input order. Names are resolved together, and only
// IPv4 answers are kept. IPv6 addresses are not probed.
func resolveScanHosts(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	type pending struct {
		index int
		name  string
	}
	var names []pending
	out := make([][]string, len(hosts))
	var skippedV6 int
	for i, host := range hosts {
		switch {
		case utils.IsIPv4(host):
			out[i] = []string{host}
		case utils.IsIPv6(host):
			skippedV6++
		default:
			names = append(names, pending{index: i, name: host})
		}
	}
	if skippedV6 > 0 {
		log.Infof("synscanx: skipped %d ipv6 targets; probes are ipv4-only", skippedV6)
	}
	if len(names) == 1 {
		out[names[0].index] = lookupIPv4(names[0].name)
	} else if len(names) > 1 {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for _, item := range names {
			wg.Add(1)
			go func(item pending) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				out[item.index] = lookupIPv4(item.name)
			}(item)
		}
		wg.Wait()
	}
	var flat []string
	seen := make(map[string]struct{}, len(hosts))
	for _, ips := range out {
		for _, ip := range ips {
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			flat = append(flat, ip)
		}
	}
	return flat
}

func lookupIPv4(host string) []string {
	var out []string
	for _, ip := range netx.LookupAll(host, netx.WithTimeout(3*time.Second)) {
		if utils.IsIPv4(ip) {
			out = append(out, ip)
		}
	}
	if len(out) == 0 {
		log.Debugf("synscanx: no ipv4 address for %s", host)
	}
	return out
}

// hostResultFilter matches result IPs against the caller's host text. A domain
// is kept as a string and also expanded to its A records, and a CIDR stays one
// network comparison instead of one closure per address.
func hostResultFilter(raw string) *utils.HostsFilter {
	filter := utils.NewHostsFilter()
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(part); err == nil {
			filter.AddCIDR(part)
			continue
		}
		if net.ParseIP(part) == nil && strings.Count(part, "-") == 0 {
			filter.Add(part)
			for _, ip := range lookupIPv4(part) {
				filter.Add(ip)
			}
			continue
		}
		filter.Add(part)
	}
	return filter
}
