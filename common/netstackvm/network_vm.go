package netstackvm

import (
	"context"
	"errors"
	"fmt"
	"github.com/gopacket/gopacket"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

// NetworkMode separates network ownership from the packet transport.
type NetworkMode string

const (
	// NetworkIsolated has no host uplink. Connect its packet endpoint to a virtual network.
	NetworkIsolated NetworkMode = "isolated"
	// NetworkBridged uses a caller-owned L2 uplink (for example TAP), with its own MAC/IP.
	NetworkBridged NetworkMode = "bridged"
	// NetworkShared uses a private IPv4 network and a userspace TCP/UDP host gateway.
	// It requires no host routes, privileges, or packet injection. Raw IP protocols
	// remain available inside the VM but are not forwarded through host sockets.
	NetworkShared NetworkMode = "shared"
	// NetworkPCAP observes a host device. All writes, including TCP RSTs, are suppressed.
	NetworkPCAP NetworkMode = "pcap"
)

var ErrPassiveNetwork = errors.New("pcap observation mode does not permit active networking")

// NetworkVMConfig describes one independent stack. A supplied Link is owned by
// the VM once validation succeeds (including setup failures), and must not
// be attached to another stack.
// Bridging requires an Ethernet-capable link with a unique unicast MAC. PCAP is
// deliberately a separate, passive mode; it is not a bridge uplink.
type NetworkVMConfig struct {
	Mode    NetworkMode
	Address netip.Prefix
	Gateway netip.Addr
	Link    stack.LinkEndpoint
	Device  string
	DHCP    bool
	DNS     []netip.Addr
	// OnPacket observes pcap traffic synchronously; it must not block.
	OnPacket func(gopacket.Packet)
	// HostDialContext can enforce egress policy in shared mode. Nil uses net.Dialer.
	// Custom dialers must honor context cancellation, including during Close.
	HostDialContext func(context.Context, string, string) (net.Conn, error)
}

type NetworkVM struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mode      NetworkMode
	stack     *stack.Stack
	nic       tcpip.NICID
	link      stack.LinkEndpoint
	packets   *channel.Endpoint
	mu        sync.RWMutex
	address   netip.Prefix
	gateway   netip.Addr
	dns       []netip.Addr
	ready     chan struct{}
	readyOnce sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup
	shared    *sharedGateway
}

func NewNetworkVM(ctx context.Context, cfg NetworkVMConfig) (*NetworkVM, error) {
	return newNetworkVM(ctx, cfg, NewPCAPEndpoint)
}

func newNetworkVM(ctx context.Context, cfg NetworkVMConfig, openPCAP func(context.Context, *stack.Stack, string, net.HardwareAddr, bool) (*PCAPEndpoint, error)) (*NetworkVM, error) {
	if ctx == nil {
		return nil, errors.New("nil network VM context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Mode == "" {
		cfg.Mode = NetworkIsolated
	}
	switch cfg.Mode {
	case NetworkIsolated, NetworkBridged, NetworkShared, NetworkPCAP:
	default:
		return nil, fmt.Errorf("unknown network mode %q", cfg.Mode)
	}
	if cfg.Mode == NetworkPCAP {
		if cfg.Device == "" || cfg.Link != nil || cfg.DHCP || cfg.Address.IsValid() || cfg.Gateway.IsValid() {
			return nil, errors.New("pcap mode requires only a device; addresses, DHCP and active links are forbidden")
		}
	} else {
		if cfg.Device != "" {
			return nil, errors.New("a pcap device is only valid in passive mode")
		}
		if cfg.DHCP && (cfg.Mode != NetworkBridged || cfg.Address.IsValid() || cfg.Gateway.IsValid()) {
			return nil, errors.New("DHCP requires bridged mode without a static address or gateway")
		}
		if cfg.Mode == NetworkBridged && cfg.Link == nil {
			return nil, errors.New("bridged mode requires an L2 link")
		}
		if cfg.Mode == NetworkShared && cfg.Link != nil {
			return nil, errors.New("shared mode owns its private link")
		}
		if !cfg.DHCP {
			if !cfg.Address.IsValid() || !cfg.Address.Addr().Is4() || !cfg.Address.Addr().IsGlobalUnicast() {
				return nil, errors.New("a unicast IPv4 prefix is required")
			}
			if cfg.Gateway.IsValid() && (!cfg.Gateway.Is4() || !cfg.Address.Contains(cfg.Gateway) || cfg.Gateway == cfg.Address.Addr() || !cfg.Gateway.IsGlobalUnicast()) {
				return nil, errors.New("gateway must be a different unicast IPv4 address in the VM subnet")
			}
		}
	}
	for _, dns := range cfg.DNS {
		if !dns.Is4() || !dns.IsGlobalUnicast() {
			return nil, errors.New("DNS servers must be unicast IPv4 addresses")
		}
	}
	if len(cfg.DNS) > 0 && (cfg.DHCP || cfg.Mode == NetworkPCAP) {
		return nil, errors.New("static DNS cannot be combined with DHCP or pcap")
	}
	if cfg.OnPacket != nil && cfg.Mode != NetworkPCAP {
		return nil, errors.New("OnPacket requires pcap mode")
	}
	if cfg.Mode == NetworkShared && cfg.Gateway.IsValid() {
		return nil, errors.New("shared mode configures its own gateway")
	}
	if cfg.HostDialContext != nil && cfg.Mode != NetworkShared {
		return nil, errors.New("host dial policy requires shared mode")
	}
	if cfg.Mode == NetworkBridged {
		mac := []byte(cfg.Link.LinkAddress())
		if len(mac) != 6 || mac[0]&1 != 0 || string(mac) == "\x00\x00\x00\x00\x00\x00" {
			return nil, errors.New("bridge link requires a unique unicast MAC")
		}
	}
	c := NewDefaultConfig()
	c.HandleLocal = cfg.Mode != NetworkPCAP
	s, err := NewNetStackFromConfig(c)
	if err != nil {
		return nil, err
	}
	base, cancel := context.WithCancel(ctx)
	vm := &NetworkVM{ctx: base, cancel: cancel, mode: cfg.Mode, stack: s, nic: s.NextNICID(), ready: make(chan struct{})}
	success := false
	defer func() {
		if !success {
			vm.Close()
		}
	}()
	vm.dns = append([]netip.Addr(nil), cfg.DNS...)
	vm.link = cfg.Link
	if cfg.Mode == NetworkPCAP {
		ep, err := openPCAP(base, s, cfg.Device, net.HardwareAddr{2, 0, 0, 0, 0, 1}, true)
		if err != nil {
			return nil, err
		}
		ep.readOnly.Store(true)
		ep.SetCapabilities(stack.CapabilityRXChecksumOffload)
		if cfg.OnPacket != nil {
			ep.SetPCAPInboundFilter(func(p gopacket.Packet) bool { cfg.OnPacket(p); return true })
		}
		vm.link = ep
	} else if vm.link == nil {
		vm.packets = channel.New(1024, 1500, "")
		vm.link = vm.packets
	}
	if err := s.CreateNIC(vm.nic, vm.link); err != nil {
		return nil, fmt.Errorf("create network VM NIC: %s", err)
	}
	if cfg.Mode == NetworkPCAP {
		s.SetPromiscuousMode(vm.nic, true)
		vm.readyOnce.Do(func() { close(vm.ready) })
	} else if cfg.DHCP {
		client := dhcp.NewClient(s, vm.nic, 5*time.Second, time.Second, time.Second, vm.applyLease)
		vm.wg.Add(1)
		go func() { defer vm.wg.Done(); client.Run(base) }()
	} else {
		if err := vm.setAddress(cfg.Address, cfg.Gateway); err != nil {
			return nil, err
		}
		vm.readyOnce.Do(func() { close(vm.ready) })
		if cfg.Mode == NetworkShared {
			if err := vm.startShared(cfg.HostDialContext); err != nil {
				return nil, err
			}
		}
	}
	success = true
	go func() { <-base.Done(); vm.Close() }()
	return vm, nil
}

// Stack exposes the independent protocol stack for raw IP and custom protocols.
// In pcap mode even writes through this stack pass the immutable passive gate.
func (v *NetworkVM) Stack() *stack.Stack { return v.stack }
func (v *NetworkVM) Mode() NetworkMode   { return v.mode }

// PacketEndpoint is available in isolated mode for connecting virtual networks.
// Shared mode consumes its own queues, so it does not expose them.
func (v *NetworkVM) PacketEndpoint() *channel.Endpoint {
	if v.mode == NetworkIsolated {
		return v.packets
	}
	return nil
}
func (v *NetworkVM) Address() netip.Prefix { v.mu.RLock(); defer v.mu.RUnlock(); return v.address }
func (v *NetworkVM) Gateway() netip.Addr   { v.mu.RLock(); defer v.mu.RUnlock(); return v.gateway }
func (v *NetworkVM) DNSServers() []netip.Addr {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return append([]netip.Addr(nil), v.dns...)
}

func (v *NetworkVM) setAddress(addr netip.Prefix, gateway netip.Addr) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.address == addr && v.gateway == gateway {
		return nil
	}
	if v.address.IsValid() {
		v.stack.RemoveAddress(v.nic, tcpip.AddrFrom4(v.address.Addr().As4()))
	}
	v.address = netip.Prefix{}
	v.gateway = netip.Addr{}
	v.stack.SetRouteTable(nil)
	if !addr.IsValid() {
		return nil
	}
	a := tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4(addr.Addr().As4()), PrefixLen: addr.Bits()}
	if err := v.stack.AddProtocolAddress(v.nic, tcpip.ProtocolAddress{Protocol: header.IPv4ProtocolNumber, AddressWithPrefix: a}, stack.AddressProperties{}); err != nil {
		return fmt.Errorf("configure VM address: %s", err)
	}
	routes := []tcpip.Route{{Destination: a.Subnet(), NIC: v.nic}}
	if gateway.IsValid() {
		routes = append(routes, tcpip.Route{Destination: header.IPv4EmptySubnet, Gateway: tcpip.AddrFrom4(gateway.As4()), NIC: v.nic})
	}
	if v.mode == NetworkShared {
		routes = append(routes, tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: v.nic})
	}
	v.stack.SetRouteTable(routes)
	v.address, v.gateway = addr, gateway
	return nil
}

func (v *NetworkVM) applyLease(ctx context.Context, lost, acquired tcpip.AddressWithPrefix, cfg dhcp.Config) {
	if ctx.Err() != nil {
		return
	}
	var address netip.Prefix
	var gateway netip.Addr
	if acquired.Address.Len() == 4 && !acquired.Address.Unspecified() {
		address = netip.PrefixFrom(netip.AddrFrom4(acquired.Address.As4()), acquired.PrefixLen)
	}
	if address.IsValid() && len(cfg.Router) > 0 && cfg.Router[0].Len() == 4 {
		gateway = netip.AddrFrom4(cfg.Router[0].As4())
	}
	// The DHCP server identifier is not a router. Install only option 3 as gateway.
	if err := v.setAddress(address, gateway); err != nil {
		return
	}
	v.mu.Lock()
	v.dns = nil
	if address.IsValid() {
		for _, a := range cfg.DNS {
			if ip, ok := netip.AddrFromSlice(a.AsSlice()); ok {
				v.dns = append(v.dns, ip)
			}
		}
	}
	v.mu.Unlock()
	if address.IsValid() {
		v.readyOnce.Do(func() { close(v.ready) })
	}
}

func (v *NetworkVM) WaitReady(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil readiness context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-v.ctx.Done():
		return net.ErrClosed
	case <-v.ready:
	}
	if v.ctx.Err() != nil {
		return net.ErrClosed
	}
	if v.mode != NetworkPCAP && !v.Address().IsValid() {
		return errors.New("VM has no current address lease")
	}
	return nil
}

func (v *NetworkVM) Close() error {
	v.closeOnce.Do(func() {
		v.cancel()
		if v.shared != nil {
			v.shared.close()
		}
		if v.link != nil {
			v.link.Close()
		}
		v.wg.Wait()
		v.stack.Close()
		v.stack.Wait()
	})
	return nil
}

// DialContext supports TCP and connected UDP. Hostnames are resolved using the
// VM's DHCP DNS servers; without those, callers must supply an IP literal. This
// prevents silent DNS or connection fallback to the host network.
func (v *NetworkVM) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if v.mode == NetworkPCAP {
		return nil, ErrPassiveNetwork
	}
	if network != "tcp" && network != "tcp4" && network != "udp" && network != "udp4" {
		return nil, fmt.Errorf("unsupported VM network %q", network)
	}
	if err := v.WaitReady(ctx); err != nil {
		return nil, err
	}
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil || port == 0 {
		return nil, fmt.Errorf("invalid destination port %q", portString)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		servers := v.DNSServers()
		if len(servers) == 0 {
			return nil, fmt.Errorf("VM DNS is not configured for %q", host)
		}
		resolver := &net.Resolver{PreferGo: true, Dial: func(c context.Context, n, a string) (net.Conn, error) {
			return v.DialContext(c, n, net.JoinHostPort(servers[0].String(), "53"))
		}}
		ips, e := resolver.LookupNetIP(ctx, "ip4", host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("VM DNS returned no IPv4 addresses")
		}
		ip = ips[0]
	}
	if !ip.Is4() {
		return nil, errors.New("VM dial currently requires IPv4")
	}
	remote := tcpip.FullAddress{NIC: v.nic, Addr: tcpip.AddrFrom4(ip.As4()), Port: uint16(port)}
	if network == "udp" || network == "udp4" {
		return gonet.DialUDP(v.stack, nil, &remote, header.IPv4ProtocolNumber)
	}
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(v.ctx, cancel)
	defer stop()
	return gonet.DialContextTCP(dialCtx, v.stack, remote, header.IPv4ProtocolNumber)
}

func (v *NetworkVM) ListenTCP(address string) (net.Listener, error) {
	if v.mode == NetworkPCAP {
		return nil, ErrPassiveNetwork
	}
	if err := v.WaitReady(v.ctx); err != nil {
		return nil, err
	}
	a, err := netip.ParseAddrPort(address)
	if err != nil || !a.Addr().Is4() {
		return nil, fmt.Errorf("listen requires an IPv4 address:port: %q", address)
	}
	return gonet.ListenTCP(v.stack, tcpip.FullAddress{NIC: v.nic, Addr: tcpip.AddrFrom4(a.Addr().As4()), Port: a.Port()}, header.IPv4ProtocolNumber)
}

// ListenUDP binds a VM-owned datagram socket. It never binds a host socket.
func (v *NetworkVM) ListenUDP(address string) (net.PacketConn, error) {
	if v.mode == NetworkPCAP {
		return nil, ErrPassiveNetwork
	}
	if err := v.WaitReady(v.ctx); err != nil {
		return nil, err
	}
	a, err := netip.ParseAddrPort(address)
	if err != nil || !a.Addr().Is4() {
		return nil, fmt.Errorf("listen requires an IPv4 address:port: %q", address)
	}
	local := tcpip.FullAddress{NIC: v.nic, Addr: tcpip.AddrFrom4(a.Addr().As4()), Port: a.Port()}
	return gonet.DialUDP(v.stack, &local, nil, header.IPv4ProtocolNumber)
}
