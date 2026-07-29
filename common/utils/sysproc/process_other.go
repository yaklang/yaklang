//go:build !darwin && !linux && !windows && (!freebsd || !amd64)

package sysproc

import "net/netip"

func findProcessName(network string, ip netip.Addr, srcPort int) (uint32, string, error) {
	return 0, "", ErrPlatformNotSupport
}

func findProcessNameByEndpoints(
	network string,
	srcIP netip.Addr,
	srcPort int,
	dstIP netip.Addr,
	dstPort int,
) (uint32, string, error) {
	return findProcessName(network, srcIP, srcPort)
}

func resolveSocketByNetlink(network string, ip netip.Addr, srcPort int) (uint32, uint32, error) {
	return 0, 0, ErrPlatformNotSupport
}
