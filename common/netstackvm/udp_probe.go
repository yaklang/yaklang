package netstackvm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/checksum"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
)

var (
	// ErrUDPNoResponse is inconclusive: loss or filtering is not a closed port.
	ErrUDPNoResponse = errors.New("UDP probe received no response")
	// ErrUDPUnreachable is an ICMP port-unreachable for this probe's datagram.
	ErrUDPUnreachable = errors.New("UDP probe received port unreachable")
	// ErrUDPSend means the stack datagram was not injected.
	ErrUDPSend = errors.New("UDP probe could not send")
)

// UDPSegment is one UDP datagram observed on the probe's own stack.
// Local is this endpoint and remote is the peer.
type UDPSegment struct {
	LocalIP    net.IP
	LocalPort  uint16
	RemoteIP   net.IP
	RemotePort uint16
	Payload    []byte
}

type udpFrame struct {
	segment  UDPSegment
	data     []byte
	linkType gopacket.LayerType
}

type udpResult struct {
	segment     UDPSegment
	unreachable bool
}

type udpFlight struct {
	key         flightKey
	ctx         context.Context
	generated   chan *udpFrame
	results     chan udpResult
	frame       *udpFrame
	permit      bool
	sending     bool
	sent        bool
	badChecksum bool
}

// ProbeUDP sends one payload as a UDP datagram through the gVisor stack and
// waits for a matching reply. The context bounds admission, neighbor
// resolution, injection and the response wait. ICMP port-unreachable ends the
// probe without reporting the port open. Silence is ErrUDPNoResponse.
func (h *HalfOpenSYN) ProbeUDP(ctx context.Context, target string, payload []byte) (UDPSegment, error) {
	if h == nil {
		return UDPSegment{}, fmt.Errorf("udp probe: nil session")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(payload) > header.UDPMaximumSize-header.UDPMinimumSize {
		return UDPSegment{}, fmt.Errorf("udp probe payload too large")
	}
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return UDPSegment{}, err
	}
	port, err := parsePort(portText)
	dst := net.ParseIP(host).To4()
	if err != nil || !usableProbeIPv4(dst) || port < 1 || port > 65535 {
		return UDPSegment{}, fmt.Errorf("invalid IPv4 UDP target %q", target)
	}
	release, err := h.acquireProbe(ctx)
	if err != nil {
		return UDPSegment{}, err
	}
	defer release()

	vm := h.vmFor(dst)
	if vm == nil {
		return UDPSegment{}, fmt.Errorf("no interface for %s", host)
	}
	if subnet := vm.mainNICIPv4Netmask; subnet != nil {
		ones, bits := subnet.Mask.Size()
		if bits == 32 && ones < 31 && (dst.Equal(subnet.IP.Mask(subnet.Mask)) || isIPv4Broadcast(dst, subnet)) {
			return UDPSegment{}, fmt.Errorf("invalid UDP target %s: selected subnet boundary", host)
		}
	}
	var wq waiter.Queue
	ep, tcpErr := h.stack.NewEndpoint(udp.ProtocolNumber, header.IPv4ProtocolNumber, &wq)
	if tcpErr != nil {
		return UDPSegment{}, fmt.Errorf("new UDP endpoint: %s", tcpErr)
	}
	defer ep.Close()
	src := vm.mainNICIPv4Address.To4()
	if tcpErr = ep.Bind(tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4(ip4key(src))}); tcpErr != nil {
		return UDPSegment{}, fmt.Errorf("bind UDP endpoint: %s", tcpErr)
	}
	bound, tcpErr := ep.GetLocalAddress()
	if tcpErr != nil || bound.Port == 0 {
		return UDPSegment{}, fmt.Errorf("source port unavailable: %v", tcpErr)
	}
	key := flightKey{nic: vm.MainNICID(), local: ip4key(src), port: bound.Port, remote: ip4key(dst), remotePort: uint16(port)}
	f := &udpFlight{
		key:       key,
		ctx:       ctx,
		generated: make(chan *udpFrame, 4),
		results:   make(chan udpResult, 1),
	}
	h.mu.Lock()
	if h.udpFlights == nil {
		h.udpFlights = make(map[flightKey]*udpFlight)
	}
	if h.closed || h.udpFlights[key] != nil {
		h.mu.Unlock()
		return UDPSegment{}, fmt.Errorf("udp probe session closed or tuple still reserved")
	}
	h.udpFlights[key] = f
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.udpFlights[key] == f {
			delete(h.udpFlights, key)
		}
		h.mu.Unlock()
	}()

	remote := tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4(ip4key(dst)), Port: uint16(port)}
	if _, tcpErr = ep.Write(bytes.NewReader(payload), tcpip.WriteOptions{To: &remote}); tcpErr != nil {
		return UDPSegment{}, fmt.Errorf("%w: %s", ErrUDPSend, tcpErr)
	}
	sendTimeout := h.retry.SendTimeout
	if sendTimeout <= 0 {
		sendTimeout = 3 * time.Second
	}
	sendCtx, sendCancel := context.WithTimeout(ctx, sendTimeout)
	var frame *udpFrame
	select {
	case <-sendCtx.Done():
		sendCancel()
		return UDPSegment{}, fmt.Errorf("%w: %w", ErrUDPSend, sendCtx.Err())
	case <-h.ctx.Done():
		sendCancel()
		return UDPSegment{}, h.ctx.Err()
	case frame = <-f.generated:
	}
	sendCancel()
	h.mu.Lock()
	f.frame = frame
	h.mu.Unlock()
	if err := h.waitLimiter(ctx); err != nil {
		return UDPSegment{}, err
	}
	if err := h.injectUDP(ctx, vm, f); err != nil {
		return UDPSegment{}, err
	}
	select {
	case <-ctx.Done():
		h.mu.Lock()
		bad := f.badChecksum
		h.mu.Unlock()
		if bad {
			return UDPSegment{}, fmt.Errorf("%w: %w", ErrUDPNoResponse, ErrProbeChecksum)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return UDPSegment{}, ctx.Err()
		}
		return UDPSegment{}, ErrUDPNoResponse
	case <-h.ctx.Done():
		return UDPSegment{}, h.ctx.Err()
	case res := <-f.results:
		if res.unreachable {
			return UDPSegment{}, ErrUDPUnreachable
		}
		return res.segment, nil
	}
}

func (h *HalfOpenSYN) acquireProbe(ctx context.Context) (func(), error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, errHalfOpenClosed
	}
	h.wg.Add(1)
	h.mu.Unlock()
	select {
	case <-ctx.Done():
		h.wg.Done()
		return nil, ctx.Err()
	case <-h.ctx.Done():
		h.wg.Done()
		return nil, h.ctx.Err()
	case h.slots <- struct{}{}:
	}
	// Close can win the select race. Do not start a probe on a session that
	// has already begun shutdown.
	h.mu.Lock()
	closed := h.closed || h.ctx.Err() != nil
	h.mu.Unlock()
	if closed {
		<-h.slots
		h.wg.Done()
		if err := h.ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errHalfOpenClosed
	}
	return func() {
		<-h.slots
		h.wg.Done()
	}, nil
}

func (h *HalfOpenSYN) injectUDP(ctx context.Context, vm *NetStackVirtualMachineEntry, f *udpFlight) error {
	h.mu.Lock()
	if h.closed || f.ctx.Err() != nil || h.udpFlights[f.key] != f || f.frame == nil {
		h.mu.Unlock()
		return context.Canceled
	}
	f.permit = true
	f.sending = true
	h.mu.Unlock()
	var err error
	if vm.driver.readOnly.Load() {
		err = ErrPassiveNetwork
	} else {
		err = vm.driver.writeFrameContext(ctx, f.frame.data, f.frame.linkType)
	}
	h.mu.Lock()
	if f.permit && err == nil {
		err = fmt.Errorf("%w: injection was declined", ErrUDPSend)
	}
	if err != nil && !errors.Is(err, ErrUDPSend) {
		err = fmt.Errorf("%w: %w", ErrUDPSend, err)
	}
	f.permit = false
	f.sending = false
	if err == nil {
		f.sent = true
	}
	h.mu.Unlock()
	return err
}

func (h *HalfOpenSYN) captureUDPFrame(vm *NetStackVirtualMachineEntry, packet gopacket.Packet, data []byte, lt gopacket.LayerType) bool {
	ip, udpLayer, ok := ipv4UDP(packet)
	if !ok {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.udpFlights[udpPacketKey(vm.MainNICID(), ip, udpLayer, false)]
	if f == nil || f.ctx.Err() != nil || f.frame != nil {
		return true
	}
	frame := &udpFrame{
		segment: UDPSegment{
			LocalIP:    append(net.IP(nil), ip.SrcIP.To4()...),
			LocalPort:  uint16(udpLayer.SrcPort),
			RemoteIP:   append(net.IP(nil), ip.DstIP.To4()...),
			RemotePort: uint16(udpLayer.DstPort),
			Payload:    append([]byte(nil), udpLayer.Payload...),
		},
		data:     append([]byte(nil), data...),
		linkType: lt,
	}
	select {
	case f.generated <- frame:
	default:
	}
	return true
}

func (h *HalfOpenSYN) allowUDP(nic tcpip.NICID, ip *layers.IPv4, udpLayer *layers.UDP) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.udpFlights[udpPacketKey(nic, ip, udpLayer, false)]
	if f == nil || f.ctx.Err() != nil || h.ctx.Err() != nil || f.frame == nil || !f.permit {
		return false
	}
	if !bytes.Equal(udpLayer.Payload, f.frame.segment.Payload) {
		return false
	}
	f.permit = false
	return true
}

func (h *HalfOpenSYN) observeUDP(nic tcpip.NICID, packet gopacket.Packet, ip *layers.IPv4, udpLayer *layers.UDP) {
	if ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 || packet.Metadata().Truncated {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.udpFlights[udpPacketKey(nic, ip, udpLayer, true)]
	if f == nil || f.ctx.Err() != nil || h.ctx.Err() != nil || f.frame == nil || (!f.sent && !f.sending) {
		return
	}
	raw := append(append([]byte(nil), ip.Contents...), ip.Payload...)
	loopback := (h.loop != nil && nic == h.loop.MainNICID()) || (h.mainIsLoopback && h.main != nil && nic == h.main.MainNICID())
	if !loopback {
		ipv4hdr := header.IPv4(raw)
		if !ipv4hdr.IsChecksumValid() || !udpChecksumValid(ipv4hdr) {
			f.badChecksum = true
			return
		}
	}
	seg := UDPSegment{
		LocalIP:    append(net.IP(nil), ip.DstIP.To4()...),
		LocalPort:  uint16(udpLayer.DstPort),
		RemoteIP:   append(net.IP(nil), ip.SrcIP.To4()...),
		RemotePort: uint16(udpLayer.SrcPort),
		Payload:    append([]byte(nil), udpLayer.Payload...),
	}
	offerUDPResult(f.results, udpResult{segment: seg})
}

func (h *HalfOpenSYN) observeUDPUnreachable(nic tcpip.NICID, packet gopacket.Packet) {
	key, ok := unreachableUDPKey(nic, packet)
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.udpFlights[key]
	if f == nil || f.ctx.Err() != nil || h.ctx.Err() != nil || f.frame == nil || (!f.sent && !f.sending) {
		return
	}
	offerUDPResult(f.results, udpResult{unreachable: true})
}

// unreachableUDPKey accepts a port-unreachable only when the outer IPv4 and
// ICMP checksums are valid. The quoted datagram may be the RFC 792 minimum:
// the IPv4 header plus the first 8 bytes of UDP.
func unreachableUDPKey(nic tcpip.NICID, packet gopacket.Packet) (flightKey, bool) {
	return unreachableTransportKey(nic, packet, header.UDPProtocolNumber, func(code header.ICMPv4Code) bool {
		return code == header.ICMPv4PortUnreachable
	})
}

func unreachableTCPKey(nic tcpip.NICID, packet gopacket.Packet) (flightKey, bool) {
	return unreachableTransportKey(nic, packet, header.TCPProtocolNumber, func(code header.ICMPv4Code) bool {
		switch code {
		case header.ICMPv4NetUnreachable, header.ICMPv4HostUnreachable, header.ICMPv4ProtoUnreachable, header.ICMPv4PortUnreachable,
			header.ICMPv4NetProhibited, header.ICMPv4HostProhibited, header.ICMPv4AdminProhibited:
			return true
		default:
			return false
		}
	})
}

func unreachableTransportKey(nic tcpip.NICID, packet gopacket.Packet, proto tcpip.TransportProtocolNumber, accept func(header.ICMPv4Code) bool) (flightKey, bool) {
	if packet == nil || packet.Metadata().Truncated {
		return flightKey{}, false
	}
	ipLayer, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	if !ok || ipLayer == nil || ipLayer.FragOffset != 0 || ipLayer.Flags&layers.IPv4MoreFragments != 0 {
		return flightKey{}, false
	}
	raw := append(append([]byte(nil), ipLayer.Contents...), ipLayer.Payload...)
	if len(raw) < header.IPv4MinimumSize {
		return flightKey{}, false
	}
	iphdr := header.IPv4(raw)
	// IsValid before any header-length slice. A declared IHL or total length
	// past the captured bytes must not be treated as a matching unreachable.
	if !iphdr.IsValid(len(raw)) || !iphdr.IsChecksumValid() {
		return flightKey{}, false
	}
	icmpLen := int(iphdr.PayloadLength())
	icmpBytes := iphdr.Payload()
	if icmpLen < header.ICMPv4MinimumSize || icmpLen > len(icmpBytes) {
		return flightKey{}, false
	}
	icmpBytes = icmpBytes[:icmpLen]
	if checksum.Checksum(icmpBytes, 0) != 0xffff {
		return flightKey{}, false
	}
	icmpHdr := header.ICMPv4(icmpBytes)
	if icmpHdr.Type() != header.ICMPv4DstUnreachable || !accept(icmpHdr.Code()) {
		return flightKey{}, false
	}
	return quotedTransportKey(nic, icmpHdr.Payload(), proto)
}

func quotedUDPKey(nic tcpip.NICID, quote []byte) (flightKey, bool) {
	return quotedTransportKey(nic, quote, header.UDPProtocolNumber)
}

func quotedTransportKey(nic tcpip.NICID, quote []byte, proto tcpip.TransportProtocolNumber) (flightKey, bool) {
	if len(quote) < header.IPv4MinimumSize+header.ICMPv4MinimumErrorPayloadSize {
		return flightKey{}, false
	}
	ip := header.IPv4(quote)
	hlen := int(ip.HeaderLength())
	if hlen < header.IPv4MinimumSize || hlen > header.IPv4MaximumHeaderSize || len(quote) < hlen+header.ICMPv4MinimumErrorPayloadSize {
		return flightKey{}, false
	}
	if quote[0]>>4 != header.IPv4Version || ip.TransportProtocol() != proto {
		return flightKey{}, false
	}
	if ip.More() || ip.FragmentOffset() != 0 {
		return flightKey{}, false
	}
	src := ip.SourceAddressSlice()
	dst := ip.DestinationAddressSlice()
	if len(src) != net.IPv4len || len(dst) != net.IPv4len {
		return flightKey{}, false
	}
	return flightKey{
		nic:        nic,
		local:      ip4key(src),
		port:       binary.BigEndian.Uint16(quote[hlen:]),
		remote:     ip4key(dst),
		remotePort: binary.BigEndian.Uint16(quote[hlen+2:]),
	}, true
}

func ipv4UDP(packet gopacket.Packet) (*layers.IPv4, *layers.UDP, bool) {
	if packet == nil {
		return nil, nil, false
	}
	ip, ipOK := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	udpLayer, udpOK := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	return ip, udpLayer, ipOK && udpOK && ip != nil && udpLayer != nil
}

func udpPacketKey(nic tcpip.NICID, ip *layers.IPv4, udpLayer *layers.UDP, inbound bool) flightKey {
	if inbound {
		return flightKey{nic, ip4key(ip.DstIP), uint16(udpLayer.DstPort), ip4key(ip.SrcIP), uint16(udpLayer.SrcPort)}
	}
	return flightKey{nic, ip4key(ip.SrcIP), uint16(udpLayer.SrcPort), ip4key(ip.DstIP), uint16(udpLayer.DstPort)}
}

func udpChecksumValid(ip header.IPv4) bool {
	payload := ip.Payload()
	if len(payload) < header.UDPMinimumSize {
		return false
	}
	u := header.UDP(payload)
	n := int(u.Length())
	if n < header.UDPMinimumSize || n > len(payload) {
		return false
	}
	if u.Checksum() == 0 {
		return true
	}
	sum := header.PseudoHeaderChecksum(header.UDPProtocolNumber, ip.SourceAddress(), ip.DestinationAddress(), u.Length())
	return checksum.Checksum(u[:n], sum) == 0xffff
}

// offerUDPResult keeps a datagram when an ICMP unreachable is already queued,
// and lets a datagram replace that unreachable. A 1-slot channel otherwise
// drops whichever packet arrives second.
func offerUDPResult(ch chan udpResult, res udpResult) {
	select {
	case ch <- res:
		return
	default:
	}
	select {
	case old := <-ch:
		if !old.unreachable && res.unreachable {
			select {
			case ch <- old:
			default:
			}
			return
		}
	default:
	}
	select {
	case ch <- res:
	default:
	}
}

func parsePort(text string) (int, error) {
	port := 0
	if text == "" {
		return 0, fmt.Errorf("empty port")
	}
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid port %q", text)
		}
		port = port*10 + int(c-'0')
		if port > 65535 {
			return 0, fmt.Errorf("invalid port %q", text)
		}
	}
	return port, nil
}
