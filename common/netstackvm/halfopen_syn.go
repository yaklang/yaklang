package netstackvm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/waiter"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils/arptable"
)

const halfOpenSYNRetire = 3 * time.Second

var errHalfOpenClosed = errors.New("half-open SYN session is closed")

// HalfOpenSYNConfig is the host identity a half-open SYN session borrows.
// The session does not run DHCP and does not invent a gateway.
type HalfOpenSYNConfig struct {
	Context  context.Context
	Iface    *net.Interface
	SourceIP net.IP
	Gateway  net.IP
	// OnOpen is called for a matching SYN-ACK. It runs on the capture loop
	// and must not block for long.
	OnOpen func(ip net.IP, port int)
}

// HalfOpenSYN emits TCP SYNs from a writable netstackvm capture and reports
// SYN-ACKs without completing the handshake.
//
// TCPProbe is not this path: it finishes a full handshake between two
// channel-backed stacks. A SYN scan has to leave the ACK unsent, so the
// SYN-ACK is observed and then dropped before gVisor can answer it.
type HalfOpenSYN struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
	slots  chan struct{}

	stack *stack.Stack
	main  *NetStackVirtualMachineEntry
	loop  *NetStackVirtualMachineEntry
	// mainIsLoopback is true when main itself is the host loopback device.
	mainIsLoopback bool
	gateway        net.IP
	onOpen         func(net.IP, int)

	flights map[flightKey]*synFlight
}

type flightKey struct {
	local [4]byte
	port  uint16
}

type synFlight struct {
	dstIP    net.IP
	dstPort  uint16
	isn      uint32
	haveISN  bool
	signaled bool
	reported bool
	sent     chan struct{}
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
		ctx = context.Background()
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
	synRetries := tcpip.TCPSynRetriesOption(1)
	_ = stackIns.SetTransportProtocolOption(tcp.ProtocolNumber, &synRetries)

	h := &HalfOpenSYN{
		ctx:            ctx,
		cancel:         cancel,
		slots:          make(chan struct{}, 256),
		stack:          stackIns,
		gateway:        ipv4Only(cfg.Gateway),
		onOpen:         cfg.OnOpen,
		flights:        make(map[flightKey]*synFlight),
		mainIsLoopback: cfg.Iface.Flags&net.FlagLoopback != 0,
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
		WithPCAPReadOnly(false),
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
	vm.driver.SetPCAPOutboundFilter(h.observeOutbound)
	vm.driver.SetPCAPInboundFilter(h.observeInbound)

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

// Emit starts one active open. The endpoint is closed after the SYN is on
// the wire (or after a short wait) so gVisor does not keep retransmitting.
// Closing the endpoint may generate a RST; the outbound filter drops it.
func (h *HalfOpenSYN) Emit(ctx context.Context, host string, port int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	dst := net.ParseIP(host).To4()
	if dst == nil || port <= 0 || port > 65535 {
		return fmt.Errorf("half-open syn: invalid target %s:%d", host, port)
	}
	vm := h.vmFor(dst)
	if vm == nil {
		return fmt.Errorf("half-open syn: no device for %s", host)
	}
	src := vm.mainNICIPv4Address.To4()
	if src == nil {
		return fmt.Errorf("half-open syn: device has no ipv4")
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errHalfOpenClosed
	}
	h.wg.Add(1)
	h.mu.Unlock()
	handed := false
	defer func() {
		if !handed {
			h.wg.Done()
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.ctx.Done():
		return h.ctx.Err()
	case h.slots <- struct{}{}:
	}

	ep, flight, err := h.connect(vm, src, dst, uint16(port))
	if err != nil {
		<-h.slots
		return err
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		ep.Close()
		<-h.slots
		return errHalfOpenClosed
	}
	h.mu.Unlock()
	handed = true
	go h.retire(ctx, ep, flight)
	return nil
}

func (h *HalfOpenSYN) connect(vm *NetStackVirtualMachineEntry, src, dst net.IP, port uint16) (tcpip.Endpoint, *synFlight, error) {
	var wq waiter.Queue
	ep, tcpErr := vm.stack.NewEndpoint(tcp.ProtocolNumber, header.IPv4ProtocolNumber, &wq)
	if tcpErr != nil {
		return nil, nil, fmt.Errorf("half-open syn: new endpoint: %s", tcpErr)
	}
	ok := false
	defer func() {
		if !ok {
			ep.Close()
		}
	}()
	local := tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4([4]byte(src))}
	if tcpErr = ep.Bind(local); tcpErr != nil {
		return nil, nil, fmt.Errorf("half-open syn: bind: %s", tcpErr)
	}
	bound, tcpErr := ep.GetLocalAddress()
	if tcpErr != nil || bound.Port == 0 {
		return nil, nil, fmt.Errorf("half-open syn: local port unavailable: %v", tcpErr)
	}
	flight := h.register(src, bound.Port, dst, port)
	remote := tcpip.FullAddress{NIC: vm.MainNICID(), Addr: tcpip.AddrFrom4([4]byte(dst)), Port: port}
	tcpErr = ep.Connect(remote)
	if _, started := tcpErr.(*tcpip.ErrConnectStarted); !started {
		if tcpErr == nil {
			return nil, nil, fmt.Errorf("half-open syn: connect finished before the SYN was sent")
		}
		return nil, nil, fmt.Errorf("half-open syn: connect: %s", tcpErr)
	}
	ok = true
	return ep, flight, nil
}

func (h *HalfOpenSYN) retire(ctx context.Context, ep tcpip.Endpoint, flight *synFlight) {
	defer h.wg.Done()
	defer func() { <-h.slots }()
	timer := time.NewTimer(halfOpenSYNRetire)
	defer timer.Stop()
	select {
	case <-flight.sent:
	case <-timer.C:
	case <-h.ctx.Done():
	case <-ctx.Done():
	}
	ep.Close()
}

// Close stops new SYNs, aborts endpoints still in flight, and releases the capture.
func (h *HalfOpenSYN) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	h.wg.Wait()
	if h.loop != nil {
		_ = h.loop.Close()
	}
	if h.main != nil {
		_ = h.main.Close()
	}
	if h.stack != nil {
		h.stack.Close()
		h.stack.Wait()
	}
	return nil
}

func (h *HalfOpenSYN) vmFor(dst net.IP) *NetStackVirtualMachineEntry {
	if dst != nil && dst.IsLoopback() {
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

func (h *HalfOpenSYN) register(local net.IP, localPort uint16, dst net.IP, dstPort uint16) *synFlight {
	key := flightKey{local: ip4key(local), port: localPort}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := &synFlight{
		dstIP:   append(net.IP(nil), dst.To4()...),
		dstPort: dstPort,
		sent:    make(chan struct{}),
	}
	h.flights[key] = f
	return f
}

func (h *HalfOpenSYN) observeOutbound(packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}
	if packet.Layer(layers.LayerTypeARP) != nil {
		return true
	}
	ip, tcpLayer, ok := ipv4TCP(packet)
	if !ok || !tcpLayer.SYN || tcpLayer.ACK || tcpLayer.FIN || tcpLayer.RST {
		return false
	}
	h.noteSYN(ip.SrcIP, uint16(tcpLayer.SrcPort), ip.DstIP, uint16(tcpLayer.DstPort), tcpLayer.Seq)
	return true
}

func (h *HalfOpenSYN) observeInbound(packet gopacket.Packet) bool {
	if packet == nil {
		return false
	}
	if packet.Layer(layers.LayerTypeARP) != nil {
		return true
	}
	ip, tcpLayer, ok := ipv4TCP(packet)
	if ok && tcpLayer.SYN && tcpLayer.ACK && !tcpLayer.RST {
		h.noteSYNACK(ip.DstIP, uint16(tcpLayer.DstPort), ip.SrcIP, uint16(tcpLayer.SrcPort), tcpLayer.Ack)
	}
	// SYN-ACK stays out of gVisor. Delivering it would make the stack ACK
	// and turn the probe into a full handshake.
	return false
}

func (h *HalfOpenSYN) noteSYN(localIP net.IP, localPort uint16, dstIP net.IP, dstPort uint16, seq uint32) {
	key := flightKey{local: ip4key(localIP), port: localPort}
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.flights[key]
	if f == nil || f.dstPort != dstPort || !sameIPv4(f.dstIP, dstIP) {
		return
	}
	f.isn = seq
	f.haveISN = true
	if !f.signaled {
		f.signaled = true
		close(f.sent)
	}
}

func (h *HalfOpenSYN) noteSYNACK(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, ack uint32) {
	key := flightKey{local: ip4key(localIP), port: localPort}
	h.mu.Lock()
	f := h.flights[key]
	if f == nil || !f.haveISN || ack != f.isn+1 || f.dstPort != remotePort || !sameIPv4(f.dstIP, remoteIP) || f.reported {
		h.mu.Unlock()
		return
	}
	f.reported = true
	cb := h.onOpen
	h.mu.Unlock()
	if cb != nil {
		cb(append(net.IP(nil), remoteIP.To4()...), int(remotePort))
	}
}

func ipv4TCP(packet gopacket.Packet) (*layers.IPv4, *layers.TCP, bool) {
	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	tcpLayer := packet.Layer(layers.LayerTypeTCP)
	ip, ipOK := ipLayer.(*layers.IPv4)
	tcpHdr, tcpOK := tcpLayer.(*layers.TCP)
	if !ipOK || !tcpOK || ip == nil || tcpHdr == nil {
		return nil, nil, false
	}
	return ip, tcpHdr, true
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
