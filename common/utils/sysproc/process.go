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
	network := conn.RemoteAddr().Network()
	srcIP, srcPort, err := utils.ParseStringToHostPort(conn.RemoteAddr().String())
	if err != nil {
		return 0, "", err
	}
	return FindProcessName(network, IpToAddr(net.ParseIP(srcIP)), int(srcPort))
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
