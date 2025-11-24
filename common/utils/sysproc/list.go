package sysproc

import (
	"net"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

type ProcessInfo struct {
	*process.Process
}

func (p *ProcessInfo) GetRemoteNonLocalIPAddresses() ([]string, error) {
	conn, err := p.Connections()
	if err != nil {
		return nil, err
	}

	localIPs, err := collectLocalIPAddressSet()
	if err != nil {
		return nil, err
	}

	var (
		ips       []string
		uniqueMap = make(map[string]struct{})
	)
	for _, c := range conn {
		ip := strings.TrimSpace(c.Raddr.IP)
		if ip == "" || strings.EqualFold(ip, "localhost") {
			continue
		}

		normalized := normalizeIP(ip)
		if normalized == "" {
			continue
		}

		if _, ok := localIPs[normalized]; ok {
			continue
		}

		if _, ok := uniqueMap[normalized]; ok {
			continue
		}
		uniqueMap[normalized] = struct{}{}
		ips = append(ips, normalized)
	}
	return ips, nil
}

func collectLocalIPAddressSet() (map[string]struct{}, error) {
	localSet := map[string]struct{}{
		"127.0.0.1": {},
		"::1":       {},
		"0.0.0.0":   {},
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}

			if ip.IsLoopback() {
				localSet[ip.String()] = struct{}{}
			}

			if v4 := ip.To4(); v4 != nil {
				localSet[v4.String()] = struct{}{}
			} else {
				localSet[ip.String()] = struct{}{}
			}
		}
	}

	return localSet, nil
}

func normalizeIP(raw string) string {
	parsed := net.ParseIP(raw)
	if parsed == nil {
		return strings.TrimSpace(raw)
	}

	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.String()
}

func List() ([]*ProcessInfo, error) {
	pcs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	var results = make([]*ProcessInfo, len(pcs))
	for i, p := range pcs {
		results[i] = &ProcessInfo{Process: p}
	}
	return results, nil
}
