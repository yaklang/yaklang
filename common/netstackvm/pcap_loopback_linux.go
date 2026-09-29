package netstackvm

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/gopacket/gopacket/layers"
	"golang.org/x/sys/unix"
)

// Linux AF_PACKET injection on lo appears in captures but is not delivered to
// local IPv4 sockets. Use IP_HDRINCL after the endpoint's normal write policy.
// The fanout write gate serializes both lazy socket creation and Close.
func newPCAPLoopbackWriter(device string, link layers.LinkType, fallback func([]byte) error) (func([]byte) error, func()) {
	iface, err := net.InterfaceByName(device)
	if err != nil || iface.Flags&net.FlagLoopback == 0 || link != layers.LinkTypeEthernet {
		return fallback, func() {}
	}
	fd := -1
	closeSocket := func() {
		if fd >= 0 {
			_ = unix.Close(fd)
			fd = -1
		}
	}
	return func(frame []byte) error {
		if len(frame) < 14 || binary.BigEndian.Uint16(frame[12:14]) != uint16(layers.EthernetTypeIPv4) {
			return fallback(frame)
		}
		ip := frame[14:]
		if len(ip) < 20 || ip[0]>>4 != 4 {
			return fmt.Errorf("invalid loopback IPv4 packet")
		}
		hlen, size := int(ip[0]&15)*4, int(binary.BigEndian.Uint16(ip[2:4]))
		if hlen < 20 || size < hlen || size > len(ip) || !net.IP(ip[12:16]).IsLoopback() || !net.IP(ip[16:20]).IsLoopback() {
			return fmt.Errorf("invalid loopback IPv4 length or address")
		}
		if fd < 0 {
			fd, err = unix.Socket(unix.AF_INET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.IPPROTO_RAW)
			if err != nil {
				return err
			}
			if err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1); err != nil {
				closeSocket()
				return err
			}
			if err = unix.BindToDevice(fd, device); err != nil {
				closeSocket()
				return err
			}
		}
		dst := &unix.SockaddrInet4{}
		copy(dst.Addr[:], ip[16:20])
		return unix.Sendto(fd, ip[:size], 0, dst)
	}, closeSocket
}
