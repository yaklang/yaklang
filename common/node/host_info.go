package node

import (
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
)

type systemHostInfoProvider struct{}

func (systemHostInfoProvider) Snapshot() HostInfo {
	info := HostInfo{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
	}
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}
	info.IPAddresses = collectHostIPAddresses()
	if len(info.IPAddresses) > 0 {
		info.PrimaryIP = info.IPAddresses[0]
	}
	return normalizeHostInfo(info)
}

func normalizeHostInfo(input HostInfo) HostInfo {
	normalized := HostInfo{
		Hostname:        strings.TrimSpace(input.Hostname),
		PrimaryIP:       strings.TrimSpace(input.PrimaryIP),
		OperatingSystem: strings.TrimSpace(input.OperatingSystem),
		Architecture:    strings.TrimSpace(input.Architecture),
	}

	seen := make(map[string]struct{}, len(input.IPAddresses)+1)
	appendIP := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		normalized.IPAddresses = append(normalized.IPAddresses, trimmed)
	}

	appendIP(normalized.PrimaryIP)
	for _, value := range input.IPAddresses {
		appendIP(value)
	}
	if normalized.PrimaryIP == "" && len(normalized.IPAddresses) > 0 {
		normalized.PrimaryIP = normalized.IPAddresses[0]
	}
	return normalized
}

func collectHostIPAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}

	type candidate struct {
		iface string
		ip    string
	}

	candidates := make([]candidate, 0, 8)
	for _, iface := range interfaces {
		if !isHostInterfaceEligible(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := hostIPAddress(addr)
			if ip == "" {
				continue
			}
			candidates = append(candidates, candidate{
				iface: iface.Name,
				ip:    ip,
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := compareHostIPCandidate(candidates[i])
		right := compareHostIPCandidate(candidates[j])
		if left != right {
			return left < right
		}
		if candidates[i].iface != candidates[j].iface {
			return candidates[i].iface < candidates[j].iface
		}
		return candidates[i].ip < candidates[j].ip
	})

	addresses := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, exists := seen[candidate.ip]; exists {
			continue
		}
		seen[candidate.ip] = struct{}{}
		addresses = append(addresses, candidate.ip)
	}
	return addresses
}

func isHostInterfaceEligible(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}

	name := strings.ToLower(strings.TrimSpace(iface.Name))
	for _, prefix := range []string{
		"docker",
		"veth",
		"br-",
		"cni",
		"flannel",
		"virbr",
		"tun",
		"tap",
		"zt",
	} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func hostIPAddress(addr net.Addr) string {
	switch value := addr.(type) {
	case *net.IPNet:
		return normalizeHostIPAddress(value.IP)
	case *net.IPAddr:
		return normalizeHostIPAddress(value.IP)
	default:
		return ""
	}
}

func normalizeHostIPAddress(ip net.IP) string {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		if !ipv4.IsGlobalUnicast() {
			return ""
		}
		return ipv4.String()
	}
	if !ip.IsGlobalUnicast() {
		return ""
	}
	return ip.String()
}

func compareHostIPCandidate(candidate struct {
	iface string
	ip    string
}) int {
	ip := net.ParseIP(candidate.ip)
	if ip == nil {
		return 1000
	}

	score := 0
	if ipv4 := ip.To4(); ipv4 != nil {
		score += 0
		if !ipv4.IsPrivate() {
			score += 10
		}
	} else {
		score += 20
	}

	name := strings.ToLower(candidate.iface)
	if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") || strings.HasPrefix(name, "wlan") {
		score -= 2
	}
	return score
}
