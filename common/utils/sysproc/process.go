package sysproc

import (
	"errors"
	"net"
	"net/netip"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	ErrInvalidNetwork     = errors.New("invalid network")
	ErrPlatformNotSupport = errors.New("not support on this platform")
	ErrNotFound           = errors.New("process not found")
)

const (
	TCP = "tcp"
	UDP = "udp"
)

func FindProcessNameByConn(conn net.Conn) (uint32, string, error) {
	remoteAddr := conn.RemoteAddr()
	network := remoteAddr.Network()
	srcIP, srcPort, err := utils.ParseStringToHostPort(remoteAddr.String())
	if err != nil {
		return 0, "", err
	}
	srcAddr := IpToAddr(net.ParseIP(srcIP))
	localAddr := conn.LocalAddr()
	if localAddr == nil {
		return FindProcessName(network, srcAddr, int(srcPort))
	}
	dstIP, dstPort, err := utils.ParseStringToHostPort(localAddr.String())
	if err != nil {
		return FindProcessName(network, srcAddr, int(srcPort))
	}
	return findProcessNameByEndpoints(
		network,
		srcAddr,
		int(srcPort),
		IpToAddr(net.ParseIP(dstIP)),
		int(dstPort),
	)
}

func FindProcessName(network string, srcIP netip.Addr, srcPort int) (uint32, string, error) {
	return findProcessName(network, srcIP, srcPort)
}

// IpToAddr converts the net.IP to netip.Addr.
// If slice's length is not 4 or 16, IpToAddr returns netip.Addr{}
func IpToAddr(slice net.IP) netip.Addr {
	ip := slice
	if len(ip) != 4 {
		if ip = slice.To4(); ip == nil {
			ip = slice
		}
	}

	if addr, ok := netip.AddrFromSlice(ip); ok {
		return addr
	}
	return netip.Addr{}
}
