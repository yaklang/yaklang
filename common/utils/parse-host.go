package utils

import (
	"fmt"
	"github.com/pkg/errors"
	"net"
	"sort"
	"strings"
)

func ConcatPorts(ports []int) string {
	sort.Ints(ports)

	if len(ports) <= 0 {
		return ""
	}

	currentRangeStart := ports[0]
	currentRangeEnd := ports[0]
	blocks := []string{}

	isInRange := false
	for _, p := range ports[1:] {
		isInRange = (p - currentRangeEnd) == 1

		if !isInRange {
			// current port interrupt the isInRange state

			if currentRangeEnd != currentRangeStart {
				// add range
				blocks = append(blocks, fmt.Sprintf("%d-%d", currentRangeStart, currentRangeEnd))
			} else {
				// add port
				blocks = append(blocks, fmt.Sprintf("%d", currentRangeStart))
			}

			currentRangeStart = p
		}
		currentRangeEnd = p
	}

	if currentRangeStart == ports[len(ports)-1] {
		blocks = append(blocks, fmt.Sprintf("%d", currentRangeStart))
	} else {
		blocks = append(blocks, fmt.Sprintf("%d-%d", currentRangeStart, currentRangeEnd))
	}
	return strings.Join(blocks, ",")
}

type PortScanTarget struct {
	Targets []string
	TCPPort string
	UDPPort string
}

func (t *PortScanTarget) String() string {
	return fmt.Sprintf("Host:%v %v/tcp %v/udp", t.Targets, t.TCPPort, t.UDPPort)
}

func SplitHostsAndPorts(hosts, ports string, portGroupSize int, proto string) []PortScanTarget {
	hostList := ParseStringToHosts(hosts)
	portList := ParseStringToPorts(ports)

	rawTarget := []PortScanTarget{}
	for _, host := range hostList {

		if strings.TrimSpace(host) == "" {
			continue
		}

		ports := []int{}
		for _, port := range portList {
			if len(ports) < portGroupSize {
				ports = append(ports, port)
			} else {
				switch proto {
				case "udp":
					rawTarget = append(rawTarget, PortScanTarget{
						Targets: []string{host},
						UDPPort: ConcatPorts(ports),
					})
					break
				default:
					rawTarget = append(rawTarget, PortScanTarget{
						Targets: []string{host},
						TCPPort: ConcatPorts(ports),
					})
				}

				ports = []int{}
			}
		}

		if len(ports) > 0 {
			switch proto {
			case "udp":
				rawTarget = append(rawTarget, PortScanTarget{
					Targets: []string{host},
					UDPPort: ConcatPorts(ports),
				})
				break
			default:
				rawTarget = append(rawTarget, PortScanTarget{
					Targets: []string{host},
					TCPPort: ConcatPorts(ports),
				})
			}
		}
	}
	return rawTarget
}

func GetCClassByIPv4(s string) (string, error) {
	ip := net.ParseIP(FixForParseIP(s))
	if ip != nil && ip.To4() != nil {
		_, network, err := net.ParseCIDR(fmt.Sprintf("%v/24", s))
		if err != nil {
			return "", err
		}
		return network.String(), nil
	}
	return "", Errorf("invalid ipv4: %v", s)
}

func ParseIPNetToRange(n *net.IPNet) (int64, int64, error) {
	startIp := n.IP.To4()
	var (
		start, end uint32
	)
	start, err := IPv4ToUint32(startIp)
	if err != nil {
		return 0, 0, errors.Errorf("cannot convert ip: %s to int: %s", startIp, err)
	}

	current := start
	for {
		ip := Uint32ToIPv4(uint32(current))
		if ip == nil {
			return 0, 0, errors.Errorf("cannot convert: %d to ip: %s", current, ip)
		}

		if n.Contains(ip) {
			current += 1
			continue
		} else {
			end = current
			break
		}
	}

	if start > end {
		return 0, 0, errors.Errorf("unknown reason for parsing range")
	}

	return int64(start), int64(end), nil
}
