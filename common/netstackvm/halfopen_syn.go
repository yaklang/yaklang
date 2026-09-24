package netstackvm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils/arptable"
	"golang.org/x/time/rate"
)

var errHalfOpenClosed = errors.New("half-open SYN session is closed")

// ErrUnverifiedSYNTransport means the interface may terminate TCP locally and
// synthesize SYN-ACKs without consulting the destination (for example a TUN proxy).
var ErrUnverifiedSYNTransport = errors.New("half-open SYN requires a verified transport")

// HalfOpenSYNConfig is the host identity a half-open SYN session borrows.
// The session does not run DHCP and does not invent a gateway.
type HalfOpenSYNConfig struct {
	Context          context.Context // Used when OpenHalfOpenSYN receives a nil context.
	Retry            SYNRetryPolicy
	MaxInFlight      int // Default 256; held through response/retry completion.
	PacketsPerSecond int // Default 1000, burst 1; includes retransmissions.
	OnResult         func(target string, synAck TCPSegment, err error)
	Iface            *net.Interface
	SourceIP         net.IP
	Gateway          net.IP
	// AllowUnverifiedTransport opts into raw-IP/point-to-point interfaces.
	// A matching SYN-ACK on these interfaces is not evidence of an open remote
	// port: a proxy may synthesize it even when the destination is closed.
	AllowUnverifiedTransport bool
	// OnOpen is called once per successful probe on its
	// worker, outside the capture loop. It must return promptly.
	OnOpen func(ip net.IP, port int)
}

// HalfOpenSYN emits TCP SYNs from a writable netstackvm capture and reports
// SYN-ACKs without completing the handshake.
//
// Each scan uses TCPProbe with a real-interface transport and a SYN-only gate.
type HalfOpenSYN struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	wg       sync.WaitGroup
	closed   bool
	slots    chan struct{}
	limiter  *rate.Limiter
	retry    SYNRetryPolicy
	onResult func(string, TCPSegment, error)
	done     chan struct{}

	stack *stack.Stack
	main  *NetStackVirtualMachineEntry
	loop  *NetStackVirtualMachineEntry
	// mainIsLoopback is true when main itself is the host loopback device.
	mainIsLoopback           bool
	allowUnverifiedTransport bool
	gateway                  net.IP
	onOpen                   func(net.IP, int)

	flights map[flightKey]*synFlight
}

type flightKey struct {
	nic        tcpip.NICID
	local      [4]byte
	port       uint16
	remote     [4]byte
	remotePort uint16
}

type probeReply struct {
	segment TCPSegment
	raw     []byte
}
type synFrame struct {
	segment  TCPSegment
	data     []byte
	linkType gopacket.LayerType
}
type synFlight struct {
	key       flightKey
	ctx       context.Context
	generated chan *synFrame
	replies   chan probeReply
	frame     *synFrame
	permit    bool
	sending   bool
	sent      bool
}

// OpenHalfOpenSYN opens the host interface (and loopback, when that is a
// different device) as one writable gVisor stack.
func OpenHalfOpenSYN(ctx context.Context, cfg HalfOpenSYNConfig) (*HalfOpenSYN, error) {
	if cfg.Iface == nil {
		return nil, fmt.Errorf("half-open syn: interface is nil")
	}
	src := ipv4Only(cfg.SourceIP)
	if src == nil {
		return nil, fmt.Errorf("half-open syn: source %v is not ipv4", cfg.SourceIP)
	}
	if ctx == nil {
		ctx = cfg.Context
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Retry == (SYNRetryPolicy{}) {
		cfg.Retry = DefaultSYNRetryPolicy()
	}
	if err := cfg.Retry.validate(); err != nil {
		return nil, err
	}
	if cfg.MaxInFlight == 0 {
		cfg.MaxInFlight = 256
	}
	if cfg.PacketsPerSecond == 0 {
		cfg.PacketsPerSecond = 1000
	}
	if cfg.MaxInFlight < 1 || cfg.PacketsPerSecond < 1 {
		return nil, fmt.Errorf("half-open SYN limits must be positive")
	}

	ctx, cancel := context.WithCancel(ctx)

	stackCfg := NewDefaultConfig()
	stackCfg.HandleLocal = false
	stackCfg.ctx, stackCfg.cancel = context.WithCancel(ctx)
	stackIns, err := NewNetStackFromConfig(stackCfg)
	if err != nil {
		cancel()
		return nil, err
	}
	synRetries := tcpip.TCPSynRetriesOption(6)
	_ = stackIns.SetTransportProtocolOption(tcp.ProtocolNumber, &synRetries)

	h := &HalfOpenSYN{
		ctx:     ctx,
		cancel:  cancel,
		slots:   make(chan struct{}, cfg.MaxInFlight),
		limiter: rate.NewLimiter(rate.Limit(cfg.PacketsPerSecond), 1),
		retry:   cfg.Retry, onResult: cfg.OnResult, done: make(chan struct{}),
		stack:                    stackIns,
		gateway:                  ipv4Only(cfg.Gateway),
		onOpen:                   cfg.OnOpen,
		flights:                  make(map[flightKey]*synFlight),
		mainIsLoopback:           cfg.Iface.Flags&net.FlagLoopback != 0,
		allowUnverifiedTransport: cfg.AllowUnverifiedTransport,
	}
	h.main, err = h.openEntry(cfg.Iface, src, maskFor(cfg.Iface, src), h.gateway, ifaceNeedsResolution(cfg.Iface))
	if err != nil {
		h.Close()
		return nil, err
	}
	if !h.mainIsLoopback {
		if lo, loErr := pcaputil.GetLoopBackNetInterface(); loErr != nil {
			log.Debugf("half-open syn: loopback device unavailable: %v", loErr)
		} else if lo != nil && lo.Name != cfg.Iface.Name {
			loopIP := net.IPv4(127, 0, 0, 1).To4()
			h.loop, err = h.openEntry(lo, loopIP, net.CIDRMask(8, 32), nil, false)
			if err != nil {
				log.Warnf("half-open syn: loopback %s not opened: %v", lo.Name, err)
				h.loop = nil
			}
		}
	}
	gw := "<on-link>"
	if h.gateway != nil {
		gw = h.gateway.String()
	}
	log.Infof("half-open SYN via netstackvm on %s src %s gateway %s", cfg.Iface.Name, src, gw)
	return h, nil
}

func (h *HalfOpenSYN) openEntry(iface *net.Interface, ip net.IP, mask net.IPMask, gateway net.IP, resolve bool) (*NetStackVirtualMachineEntry, error) {
	var caps stack.LinkEndpointCapabilities
	if resolve {
		caps = stack.CapabilityResolutionRequired
	}
	opts := []Option{
		WithContext(h.ctx),
		WithNetStack(h.stack),
		WithPcapDevice(iface.Name),
		WithPCAPReadOnly(true),
		WithDisableForwarding(true),
		WithPcapCapabilities(caps),
	}
	if len(iface.HardwareAddr) == 6 {
		opts = append(opts, WithMainNICLinkAddress(iface.HardwareAddr.String()))
	}
	vm, err := NewNetStackVirtualMachineEntry(opts...)
	if err != nil {
		return nil, err
	}
	if err := validateHalfOpenTransport(iface, vm.driver.adaptor.linkType, h.allowUnverifiedTransport); err != nil {
		vm.Close()
		return nil, err
	}
	nic := vm.MainNICID()
	vm.driver.SetPCAPOutboundFilter(func(packet gopacket.Packet) bool { return h.allowOutbound(nic, packet) })
	vm.driver.SetPCAPInboundFilter(func(packet gopacket.Packet) bool { return h.observeInbound(nic, packet) })
	vm.driver.filterMutex.Lock()
	vm.driver.stackFrame = func(data []byte, lt gopacket.LayerType) error { return h.captureStackFrame(vm, data, lt) }
	vm.driver.filterMutex.Unlock()
	// Install the immutable session policy before enabling any injection.
	vm.driver.readOnly.Store(false)

	if err := vm.SetMainNICv4(ip, &net.IPNet{IP: ip.Mask(mask), Mask: mask}, ip); err != nil {
		vm.Close()
		return nil, err
	}
	if err := addConnectedRoute(vm, ip, mask); err != nil {
		vm.Close()
		return nil, err
	}
	if gateway != nil && !gateway.Equal(ip) {
		vm.stack.AddRoute(tcpip.Route{
			Destination: header.IPv4EmptySubnet,
			Gateway:     tcpip.AddrFrom4([4]byte(gateway)),
			NIC:         vm.MainNICID(),
			MTU:         uint32(vm.mtu),
		})
		h.seedGateway(vm, gateway)
	} else if !ip.IsLoopback() {
		vm.stack.AddRoute(tcpip.Route{
			Destination: header.IPv4EmptySubnet,
			NIC:         vm.MainNICID(),
			MTU:         uint32(vm.mtu),
		})
	}
	return vm, nil
}

func validateHalfOpenTransport(iface *net.Interface, link layers.LinkType, allowUnverified bool) error {
	if iface.Flags&net.FlagLoopback != 0 || allowUnverified {
		return nil
	}
	if link != layers.LinkTypeEthernet || iface.Flags&net.FlagPointToPoint != 0 {
		return fmt.Errorf("%w: interface %q link type %d may synthesize SYN-ACKs; select a physical interface or explicitly set AllowUnverifiedTransport after verifying the path", ErrUnverifiedSYNTransport, iface.Name, link)
	}
	return nil
}

func (h *HalfOpenSYN) seedGateway(vm *NetStackVirtualMachineEntry, gateway net.IP) {
	mac, err := arptable.SearchHardware(gateway.String())
	if err != nil || len(mac) != 6 {
		return
	}
	mac = append(net.HardwareAddr(nil), mac...)
	vm.driver.SetGatewayHardwareAddr(mac)
	tcpErr := vm.stack.AddStaticNeighbor(
		vm.MainNICID(),
		header.IPv4ProtocolNumber,
		tcpip.AddrFrom4([4]byte(gateway)),
		tcpip.LinkAddress(mac),
	)
	if tcpErr != nil {
		log.Debugf("half-open syn: static neighbor %s: %v", gateway, tcpErr)
	}
}

// StartTCPProbe retains admission, the endpoint and response registration until
// Close/cancellation. A live tuple registration is never overwritten.
// It does not send until ProbeSYN[Context]. This backend cannot ProbeACK.
func (h *HalfOpenSYN) StartTCPProbe(ctx context.Context, target string, options ...TCPProbeOption) (*TCPProbe, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	dst := net.ParseIP(host).To4()
	if err != nil || dst == nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid IPv4 TCP target %q", target)
	}
	policy := h.retry
	for _, option := range options {
		option(&policy)
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, errHalfOpenClosed
	}
	h.wg.Add(1)
	h.mu.Unlock()
	success := false
	defer func() {
		if !success {
			h.wg.Done()
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-h.ctx.Done():
		return nil, h.ctx.Err()
	case h.slots <- struct{}{}:
	}
	admitted := false
	defer func() {
		if !admitted {
			<-h.slots
		}
	}()
	vm := h.vmFor(dst)
	if vm == nil {
		return nil, fmt.Errorf("no interface for %s", host)
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &TCPProbe{ctx: ctx, cancel: cancel, halfOpen: true, retry: policy, probeStep: make(chan struct{}, 1)}
	ep, tcpErr := h.stack.NewEndpoint(tcp.ProtocolNumber, header.IPv4ProtocolNumber, &p.wq)
	if tcpErr != nil {
		cancel()
		return nil, fmt.Errorf("new TCP endpoint: %s", tcpErr)
	}
	src := vm.mainNICIPv4Address.To4()
	if tcpErr = ep.Bind(tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4(ip4key(src))}); tcpErr != nil {
		ep.Close()
		cancel()
		return nil, fmt.Errorf("bind TCP endpoint: %s", tcpErr)
	}
	bound, tcpErr := ep.GetLocalAddress()
	if tcpErr != nil || bound.Port == 0 {
		ep.Close()
		cancel()
		return nil, fmt.Errorf("source port unavailable: %v", tcpErr)
	}
	key := flightKey{nic: vm.MainNICID(), local: ip4key(src), port: bound.Port, remote: ip4key(dst), remotePort: uint16(port)}
	f := &synFlight{key: key, ctx: ctx, generated: make(chan *synFrame, 4), replies: make(chan probeReply, 1)}
	h.mu.Lock()
	if h.closed || h.flights[key] != nil {
		h.mu.Unlock()
		ep.Close()
		cancel()
		return nil, fmt.Errorf("probe session closed or tuple still reserved")
	}
	h.flights[key] = f
	h.mu.Unlock()
	p.ep = ep
	p.remote = tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4(ip4key(dst)), Port: uint16(port)}
	p.transport = &pcapProbeTransport{session: h, vm: vm, probe: p, flight: f}
	success, admitted = true, true
	go func() {
		select {
		case <-ctx.Done():
		case <-h.ctx.Done():
		}
		p.Close()
	}()
	return p, nil
}

// Emit runs the same public stepped probe in a bounded worker. Admission
// blocks under load. OnResult distinguishes cancellation, local send failure,
// reset and inconclusive silence; only a validated SYN-ACK calls OnOpen.
func (h *HalfOpenSYN) Emit(ctx context.Context, host string, port int) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errHalfOpenClosed
	}
	h.wg.Add(1)
	h.mu.Unlock()
	target := net.JoinHostPort(host, strconv.Itoa(port))
	probe, err := h.StartTCPProbe(ctx, target)
	if err != nil {
		h.wg.Done()
		return err
	}
	go func() {
		defer h.wg.Done()
		defer probe.Close()
		var ack TCPSegment
		_, err := probe.ProbeSYNContext(ctx)
		if err == nil {
			ack, err = probe.ReceiveSYNACKContext(ctx)
		}
		if err == nil && h.onOpen != nil {
			h.onOpen(ack.RemoteIP, int(ack.RemotePort))
		}
		if h.onResult != nil {
			h.onResult(target, ack, err)
		}
	}()
	return nil
}

// Wait drains probes and result callbacks. Call after the last Emit; no new
// StartTCPProbe/Emit may run concurrently with Wait. Cancellation does not
// implicitly cancel the session; Close does.
func (h *HalfOpenSYN) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (h *HalfOpenSYN) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	if h.closed {
		done := h.done
		h.mu.Unlock()
		<-done
		return nil
	}
	h.closed = true
	h.cancel()
	h.mu.Unlock()
	h.wg.Wait()
	if h.loop != nil {
		h.loop.Close()
	}
	if h.main != nil {
		h.main.Close()
	}
	if h.stack != nil {
		h.stack.Close()
		h.stack.Wait()
	}
	close(h.done)
	return nil
}

func (h *HalfOpenSYN) vmFor(dst net.IP) *NetStackVirtualMachineEntry {
	if dst.IsLoopback() {
		if h.loop != nil {
			return h.loop
		}
		if h.mainIsLoopback {
			return h.main
		}
		return nil
	}
	return h.main
}

func packetFlightKey(nic tcpip.NICID, ip *layers.IPv4, tcp *layers.TCP, inbound bool) flightKey {
	if inbound {
		return flightKey{nic, ip4key(ip.DstIP), uint16(tcp.DstPort), ip4key(ip.SrcIP), uint16(tcp.SrcPort)}
	}
	return flightKey{nic, ip4key(ip.SrcIP), uint16(tcp.SrcPort), ip4key(ip.DstIP), uint16(tcp.DstPort)}
}

// captureStackFrame never writes TCP. Automatic retransmits and abort RSTs
// stay here; only the probe retry controller can authorize a SYN write.
func (h *HalfOpenSYN) captureStackFrame(vm *NetStackVirtualMachineEntry, data []byte, lt gopacket.LayerType) error {
	packet := gopacket.NewPacket(data, lt, gopacket.Default)
	if packet.Layer(layers.LayerTypeARP) != nil {
		return vm.driver.writeFrame(data, lt)
	}
	ip, tcp, ok := ipv4TCP(packet)
	if !ok || !tcp.SYN || tcp.ACK || tcp.RST || tcp.FIN {
		return nil
	}
	seg, err := parseTCPSegment(append(append([]byte(nil), ip.Contents...), ip.Payload...), false)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.flights[packetFlightKey(vm.MainNICID(), ip, tcp, false)]
	if f == nil || f.ctx.Err() != nil || f.frame != nil {
		return nil
	}
	frame := &synFrame{seg, append([]byte(nil), data...), lt}
	select {
	case f.generated <- frame:
	default:
	}
	return nil
}

// allowOutbound is the final injection gate, including manually generated RSTs.
func (h *HalfOpenSYN) allowOutbound(nic tcpip.NICID, packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}
	if packet.Layer(layers.LayerTypeARP) != nil {
		return h.ctx.Err() == nil
	}
	ip, tcp, ok := ipv4TCP(packet)
	if !ok || !tcp.SYN || tcp.ACK || tcp.RST || tcp.FIN || tcp.PSH || tcp.URG || len(tcp.Payload) != 0 {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.flights[packetFlightKey(nic, ip, tcp, false)]
	if f == nil || f.ctx.Err() != nil || h.ctx.Err() != nil || f.frame == nil || !f.permit || tcp.Seq != f.frame.segment.Seq {
		return false
	}
	f.permit = false
	return true
}

func (h *HalfOpenSYN) observeInbound(nic tcpip.NICID, packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}
	if packet.Layer(layers.LayerTypeARP) != nil {
		return h.ctx.Err() == nil
	}
	ip, tcp, ok := ipv4TCP(packet)
	if !ok || !tcp.ACK || (!tcp.SYN && !tcp.RST) || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0 || packet.Metadata().Truncated {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.flights[packetFlightKey(nic, ip, tcp, true)]
	if f == nil || f.ctx.Err() != nil || h.ctx.Err() != nil || f.frame == nil || (!f.sent && !f.sending) {
		return false
	}
	raw := append(append([]byte(nil), ip.Contents...), ip.Payload...)
	seg, err := parseTCPSegment(raw, true)
	if err != nil || !validProbeReply(f.frame.segment, seg) {
		return false
	}
	// Host loopback captures may contain offload placeholders, not wire
	// checksums (e.g. macOS lo0). Only a known loopback NIC gets this exception.
	loopback := (h.loop != nil && nic == h.loop.MainNICID()) || (h.mainIsLoopback && h.main != nil && nic == h.main.MainNICID())
	if !loopback {
		ipv4hdr := header.IPv4(raw)
		tcphdr := header.TCP(ipv4hdr.Payload())
		if !ipv4hdr.IsChecksumValid() || !tcphdr.IsChecksumValid(ipv4hdr.SourceAddress(), ipv4hdr.DestinationAddress(), 0, 0) {
			return false
		}
	}
	select {
	case f.replies <- probeReply{seg, raw}:
	default:
	}
	// No TCP response ever enters gVisor, including RSTs.
	return false
}

func ipv4TCP(packet gopacket.Packet) (*layers.IPv4, *layers.TCP, bool) {
	ip, ipOK := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp, tcpOK := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
	return ip, tcp, ipOK && tcpOK && ip != nil && tcp != nil
}

func addConnectedRoute(vm *NetStackVirtualMachineEntry, ip net.IP, mask net.IPMask) error {
	network := ip.Mask(mask).To4()
	if network == nil || len(mask) != net.IPv4len {
		return fmt.Errorf("half-open syn: bad ipv4 mask")
	}
	var network4 [4]byte
	copy(network4[:], network)
	subnet, err := tcpip.NewSubnet(tcpip.AddrFrom4(network4), tcpip.MaskFromBytes([]byte(mask)))
	if err != nil {
		return err
	}
	vm.stack.AddRoute(tcpip.Route{
		Destination: subnet,
		NIC:         vm.MainNICID(),
		MTU:         uint32(vm.mtu),
	})
	return nil
}

func ifaceNeedsResolution(iface *net.Interface) bool {
	if iface == nil || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
		return false
	}
	return len(iface.HardwareAddr) > 0
}

func maskFor(iface *net.Interface, ip net.IP) net.IPMask {
	if iface != nil {
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok || ipNet == nil || ipNet.IP.To4() == nil {
					continue
				}
				if ipNet.IP.To4().Equal(ip) && len(ipNet.Mask) == net.IPv4len {
					return ipNet.Mask
				}
			}
		}
	}
	if ip.IsLoopback() {
		return net.CIDRMask(8, 32)
	}
	return net.CIDRMask(24, 32)
}

func ipv4Only(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	v4 := ip.To4()
	if v4 == nil || v4.IsUnspecified() {
		return nil
	}
	return append(net.IP(nil), v4...)
}

func ip4key(ip net.IP) [4]byte {
	var out [4]byte
	if v4 := ip.To4(); v4 != nil {
		copy(out[:], v4)
	}
	return out
}

func sameIPv4(a, b net.IP) bool {
	aa, bb := a.To4(), b.To4()
	return aa != nil && bb != nil && aa.Equal(bb)
}
