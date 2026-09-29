package netstackvm

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/sys/unix"
)

// libpcap's packet API omits PACKET_AUXDATA checksum status. On Linux Ethernet
// devices use one bound packet socket for capture and injection instead; cooked
// "any", raw-IP and other link types retain libpcap's framing and strict checks.
func captureWithChecksumMetadata(device string, snaplen int, promisc bool, fallback *pcap.Handle) packetCaptureHandle {
	if fallback.LinkType() != layers.LinkTypeEthernet {
		return fallback
	}
	iface, err := net.InterfaceByName(device)
	if err != nil {
		return fallback
	}
	h, err := newLinuxPacketCapture(iface, snaplen, promisc)
	if err != nil {
		log.Warnf("netstackvm: capture checksum metadata unavailable on %s, retaining strict libpcap validation: %v", device, err)
		return fallback
	}
	fallback.Close()
	return h
}

type linuxPacketCapture struct {
	fd       int
	iface    int
	loopback bool
	data     []byte
	oob      []byte
}

func newLinuxPacketCapture(iface *net.Interface, snaplen int, promisc bool) (_ *linuxPacketCapture, err error) {
	if snaplen < 14 || snaplen > 1<<20 {
		return nil, fmt.Errorf("invalid capture snapshot length %d", snaplen)
	}
	// Protocol zero avoids receiving other interfaces before bind completes.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = unix.Close(fd)
		}
	}()
	if err = unix.SetsockoptInt(fd, unix.SOL_PACKET, unix.PACKET_AUXDATA, 1); err != nil {
		return nil, err
	}
	if err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); err != nil {
		return nil, err
	}
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, 2<<20)
	if promisc {
		err = unix.SetsockoptPacketMreq(fd, unix.SOL_PACKET, unix.PACKET_ADD_MEMBERSHIP, &unix.PacketMreq{Ifindex: int32(iface.Index), Type: unix.PACKET_MR_PROMISC})
		if err != nil {
			return nil, err
		}
	}
	protocol := binary.NativeEndian.Uint16([]byte{0, unix.ETH_P_ALL})
	if err = unix.Bind(fd, &unix.SockaddrLinklayer{Ifindex: iface.Index, Protocol: protocol}); err != nil {
		return nil, err
	}
	return &linuxPacketCapture{fd: fd, iface: iface.Index, loopback: iface.Flags&net.FlagLoopback != 0, data: make([]byte, snaplen), oob: make([]byte, 128)}, nil
}

func (h *linuxPacketCapture) LinkType() layers.LinkType { return layers.LinkTypeEthernet }
func (h *linuxPacketCapture) Close()                    { _ = unix.Close(h.fd) }
func (h *linuxPacketCapture) WritePacketData(data []byte) error {
	n, err := unix.Write(h.fd, data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

// The fanout owns the sole reader and waits for it before closing the descriptor.
// Poll is bounded so an idle link, discarded packet or interrupted syscall
// always returns control to the fanout's cancellation check.
func (h *linuxPacketCapture) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	fds := []unix.PollFd{{Fd: int32(h.fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 100)
	if n == 0 || err == unix.EINTR {
		return nil, gopacket.CaptureInfo{}, pcap.NextErrorTimeoutExpired
	}
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	if fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
		return nil, gopacket.CaptureInfo{}, fmt.Errorf("packet capture descriptor unavailable: poll events %#x", fds[0].Revents)
	}
	n, oobn, flags, from, err := unix.Recvmsg(h.fd, h.data, h.oob, unix.MSG_TRUNC)
	if err == unix.EAGAIN || err == unix.EINTR {
		return nil, gopacket.CaptureInfo{}, pcap.NextErrorTimeoutExpired
	}
	if err != nil {
		return nil, gopacket.CaptureInfo{}, err
	}
	sa, ok := from.(*unix.SockaddrLinklayer)
	if !ok || sa.Ifindex != h.iface || (h.loopback && sa.Pkttype == unix.PACKET_OUTGOING) {
		return nil, gopacket.CaptureInfo{}, pcap.NextErrorTimeoutExpired
	}
	captured := min(n, len(h.data))
	data := append([]byte(nil), h.data[:captured]...)
	ci := gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: captured, Length: n, InterfaceIndex: h.iface}
	return linuxCaptureMetadata(data, ci, h.oob[:oobn], flags, sa.Pkttype)
}

func linuxCaptureMetadata(data []byte, ci gopacket.CaptureInfo, oob []byte, flags int, packetType uint8) ([]byte, gopacket.CaptureInfo, error) {
	// Missing/malformed control data cannot authorize a checksum exception.
	ci.AncillaryData = nil
	messages, err := unix.ParseSocketControlMessage(oob)
	if err != nil || flags&unix.MSG_CTRUNC != 0 {
		return data, ci, nil
	}
	var aux []byte
	for _, message := range messages {
		if message.Header.Level == unix.SOL_PACKET && message.Header.Type == unix.PACKET_AUXDATA {
			if aux != nil || len(message.Data) < 20 {
				return data, ci, nil
			}
			aux = message.Data
		}
		if message.Header.Level == unix.SOL_SOCKET && message.Header.Type == unix.SO_TIMESTAMPNS {
			if stamp, ok := linuxCaptureTimestamp(message.Data); ok {
				ci.Timestamp = stamp
			}
		}
	}
	if aux == nil {
		return data, ci, nil
	}
	status := binary.NativeEndian.Uint32(aux[:4])
	wirelen, snaplen := binary.NativeEndian.Uint32(aux[4:8]), binary.NativeEndian.Uint32(aux[8:12])
	complete := flags&unix.MSG_TRUNC == 0 && ci.CaptureLength == ci.Length && ci.Length == int(wirelen) && wirelen == snaplen
	// Restore the outer VLAN tag removed by hardware. Preserve an inner tag
	// already present in the data (QinQ), including VLAN ID zero.
	if status&unix.TP_STATUS_VLAN_VALID != 0 && len(data) >= 14 {
		tag := make([]byte, 4)
		tpid := uint16(layers.EthernetTypeDot1Q)
		if status&unix.TP_STATUS_VLAN_TPID_VALID != 0 {
			tpid = binary.NativeEndian.Uint16(aux[18:20])
		}
		binary.BigEndian.PutUint16(tag[:2], tpid)
		binary.BigEndian.PutUint16(tag[2:], binary.NativeEndian.Uint16(aux[16:18]))
		frame := make([]byte, len(data)+4)
		copy(frame, data[:12])
		copy(frame[12:], tag)
		copy(frame[16:], data[12:])
		data = frame
		ci.CaptureLength += 4
		ci.Length += 4
	}
	if complete && packetType != unix.PACKET_OUTGOING {
		partial := status&unix.TP_STATUS_CSUMNOTREADY != 0
		validated := status&unix.TP_STATUS_CSUM_VALID != 0
		if partial != validated {
			ci.AncillaryData = []interface{}{captureChecksumEvidence{interfaceIndex: ci.InterfaceIndex, partial: partial, validated: validated}}
		}
	}
	return data, ci, nil
}

func linuxCaptureTimestamp(data []byte) (time.Time, bool) {
	var sec, nsec int64
	switch len(data) {
	case 16:
		sec, nsec = int64(binary.NativeEndian.Uint64(data[:8])), int64(binary.NativeEndian.Uint64(data[8:]))
	case 8:
		sec, nsec = int64(int32(binary.NativeEndian.Uint32(data[:4]))), int64(int32(binary.NativeEndian.Uint32(data[4:])))
	default:
		return time.Time{}, false
	}
	return time.Unix(sec, nsec), sec >= 0 && nsec >= 0 && nsec < int64(time.Second)
}
