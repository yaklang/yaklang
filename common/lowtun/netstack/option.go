package netstack

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/arp"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv6"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"golang.org/x/time/rate"
)

func NewDefaultStack(ipAddr string, macAddress string, networkIfaceGateway string, endpoint stack.LinkEndpoint, opts ...Option) (*stack.Stack, error) {
	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
			arp.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
			icmp.NewProtocol6,
		},
		HandleLocal: false,
	})
	err := WithDefault()(s)
	if err != nil {
		return nil, err
	}

	nic := s.NextNICID()
	opts = append(opts,
		WithCreatingNIC(nic, endpoint),

		// In the past we did s.AddAddressRange to assign 0.0.0.0/0
		// onto the interface. We need that to be able to terminate
		// all the incoming connections - to any ip. AddressRange API
		// has been removed and the suggested workaround is to use
		// Promiscuous mode. https://github.com/google/gvisor/issues/3876
		//
		// Ref: https://github.com/cloudflare/slirpnetstack/blob/master/stack.go
		WithPromiscuousMode(nic, true),

		// Enable spoofing if a stack may send packets from unowned
		// addresses. This change required changes to some netgophers
		// since previously, promiscuous mode was enough to let the
		// netstack respond to all incoming packets regardless of the
		// packet's destination address. Now that a stack.Route is not
		// held for each incoming packet, finding a route may fail with
		// local addresses we don't own but accepted packets for while
		// in promiscuous mode. Since we also want to be able to send
		// from any address (in response the received promiscuous mode
		// packets), we need to enable spoofing.
		//
		// Ref: https://github.com/google/gvisor/commit/8c0701462a84ff77e602f1626aec49479c308127
		WithSpoofing(nic, false),

		// basic all-subnet route table
		WithRouteTable(nic),
		WithMulticastGroups(nic, nil),
	)

	if ipAddr != "" {
		opts = append(opts, WithMainNICIP(nic, tcpip.AddrFromSlice(netip.MustParseAddr(ipAddr).AsSlice()), net.HardwareAddr{}))
	}

	if networkIfaceGateway != "" {
		opts = append(opts, WithIPv4RouteTableDefaultGateway(nic, netip.MustParseAddr(networkIfaceGateway)))
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

const (
	// defaultTimeToLive specifies the default TTL used by stack.
	defaultTimeToLive uint8 = 64

	// ipForwardingEnabled is the value used by stack to enable packet
	// forwarding between NICs.
	ipForwardingEnabled = true

	// icmpBurst is the default number of ICMP messages that can be sent in
	// a single burst.
	icmpBurst = 50

	// icmpLimit is the default maximum number of ICMP messages permitted
	// by this rate limiter.
	icmpLimit rate.Limit = 1000

	// tcpCongestionControl is the congestion control algorithm used by
	// stack. ccReno is the default option in gVisor stack.
	tcpCongestionControlAlgorithm = "reno" // "reno" or "cubic"

	// tcpDelayEnabled is the value used by stack to enable or disable
	// tcp delay option. Disable Nagle's algorithm here by default.
	tcpDelayEnabled = false

	// tcpModerateReceiveBufferEnabled is the value used by stack to
	// enable or disable tcp receive buffer auto-tuning option.
	tcpModerateReceiveBufferEnabled = false

	// tcpSACKEnabled is the value used by stack to enable or disable
	// tcp selective ACK.
	tcpSACKEnabled = true

	// tcpRecovery is the loss detection algorithm used by TCP.
	tcpRecovery = tcpip.TCPRACKLossDetection

	// tcpMinBufferSize is the smallest size of a send/recv buffer.
	tcpMinBufferSize = tcp.MinBufferSize

	// tcpMaxBufferSize is the maximum permitted size of a send/recv buffer.
	tcpMaxBufferSize = tcp.MaxBufferSize

	// tcpDefaultBufferSize is the default size of the send buffer for
	// a transport endpoint.
	tcpDefaultSendBufferSize = tcp.DefaultSendBufferSize

	// tcpDefaultReceiveBufferSize is the default size of the receive buffer
	// for a transport endpoint.
	tcpDefaultReceiveBufferSize = tcp.DefaultReceiveBufferSize
)

type Option func(*stack.Stack) error

// WithDefault sets all default values for stack.
func WithDefault() Option {
	return func(s *stack.Stack) error {
		opts := []Option{
			WithDefaultTTL(defaultTimeToLive),
			WithForwarding(ipForwardingEnabled),

			// Config default stack ICMP settings.
			WithICMPBurst(icmpBurst), WithICMPLimit(icmpLimit),

			// We expect no packet loss, therefore we can bump buffers.
			// Too large buffers thrash cache, so there is little point
			// in too large buffers.
			//
			// Ref: https://github.com/cloudflare/slirpnetstack/blob/master/stack.go
			WithTCPSendBufferSizeRange(tcpMinBufferSize, tcpDefaultSendBufferSize, tcpMaxBufferSize),
			WithTCPReceiveBufferSizeRange(tcpMinBufferSize, tcpDefaultReceiveBufferSize, tcpMaxBufferSize),

			WithTCPCongestionControl(tcpCongestionControlAlgorithm),
			WithTCPDelay(tcpDelayEnabled),

			// Receive Buffer Auto-Tuning Option, see:
			// https://github.com/google/gvisor/issues/1666
			WithTCPModerateReceiveBuffer(tcpModerateReceiveBufferEnabled),

			// TCP selective ACK Option, see:
			// https://tools.ietf.org/html/rfc2018
			WithTCPSACKEnabled(tcpSACKEnabled),

			// TCPRACKLossDetection: indicates RACK is used for loss detection and
			// recovery.
			//
			// TCPRACKStaticReoWnd: indicates the reordering window should not be
			// adjusted when DSACK is received.
			//
			// TCPRACKNoDupTh: indicates RACK should not consider the classic three
			// duplicate acknowledgements rule to mark the segments as lost. This
			// is used when reordering is not detected.
			WithTCPRecovery(tcpRecovery),
		}

		for _, opt := range opts {
			if err := opt(s); err != nil {
				return err
			}
		}

		return nil
	}
}

// WithDefaultTTL sets the default TTL used by stack.
func WithDefaultTTL(ttl uint8) Option {
	return func(s *stack.Stack) error {
		opt := tcpip.DefaultTTLOption(ttl)
		if err := s.SetNetworkProtocolOption(ipv4.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set ipv4 default TTL: %s", err)
		}
		if err := s.SetNetworkProtocolOption(ipv6.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set ipv6 default TTL: %s", err)
		}
		return nil
	}
}

// WithForwarding sets packet forwarding between NICs for IPv4 & IPv6.
func WithForwarding(v bool) Option {
	return func(s *stack.Stack) error {
		if err := s.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, v); err != nil {
			return fmt.Errorf("set ipv4 forwarding: %s", err)
		}
		if err := s.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, v); err != nil {
			return fmt.Errorf("set ipv6 forwarding: %s", err)
		}
		return nil
	}
}

// WithICMPBurst sets the number of ICMP messages that can be sent
// in a single burst.
func WithICMPBurst(burst int) Option {
	return func(s *stack.Stack) error {
		s.SetICMPBurst(burst)
		return nil
	}
}

// WithICMPLimit sets the maximum number of ICMP messages permitted
// by rate limiter.
func WithICMPLimit(limit rate.Limit) Option {
	return func(s *stack.Stack) error {
		s.SetICMPLimit(limit)
		return nil
	}
}

// WithTCPSendBufferSize sets default the send buffer size for TCP.
func WithTCPSendBufferSize(size int) Option {
	return func(s *stack.Stack) error {
		sndOpt := tcpip.TCPSendBufferSizeRangeOption{Min: tcpMinBufferSize, Default: size, Max: tcpMaxBufferSize}
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sndOpt); err != nil {
			return fmt.Errorf("set TCP send buffer size range: %s", err)
		}
		return nil
	}
}

// WithTCPSendBufferSizeRange sets the send buffer size range for TCP.
func WithTCPSendBufferSizeRange(a, b, c int) Option {
	return func(s *stack.Stack) error {
		sndOpt := tcpip.TCPSendBufferSizeRangeOption{Min: a, Default: b, Max: c}
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &sndOpt); err != nil {
			return fmt.Errorf("set TCP send buffer size range: %s", err)
		}
		return nil
	}
}

// WithTCPReceiveBufferSize sets the default receive buffer size for TCP.
func WithTCPReceiveBufferSize(size int) Option {
	return func(s *stack.Stack) error {
		rcvOpt := tcpip.TCPReceiveBufferSizeRangeOption{Min: tcpMinBufferSize, Default: size, Max: tcpMaxBufferSize}
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvOpt); err != nil {
			return fmt.Errorf("set TCP receive buffer size range: %s", err)
		}
		return nil
	}
}

// WithTCPReceiveBufferSizeRange sets the receive buffer size range for TCP.
func WithTCPReceiveBufferSizeRange(a, b, c int) Option {
	return func(s *stack.Stack) error {
		rcvOpt := tcpip.TCPReceiveBufferSizeRangeOption{Min: a, Default: b, Max: c}
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &rcvOpt); err != nil {
			return fmt.Errorf("set TCP receive buffer size range: %s", err)
		}
		return nil
	}
}

// WithTCPCongestionControl sets the current congestion control algorithm.
func WithTCPCongestionControl(cc string) Option {
	return func(s *stack.Stack) error {
		opt := tcpip.CongestionControlOption(cc)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set TCP congestion control algorithm: %s", err)
		}
		return nil
	}
}

// WithTCPDelay enables or disables Nagle's algorithm in TCP.
func WithTCPDelay(v bool) Option {
	return func(s *stack.Stack) error {
		opt := tcpip.TCPDelayEnabled(v)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set TCP delay: %s", err)
		}
		return nil
	}
}

// WithTCPModerateReceiveBuffer sets receive buffer moderation for TCP.
func WithTCPModerateReceiveBuffer(v bool) Option {
	return func(s *stack.Stack) error {
		opt := tcpip.TCPModerateReceiveBufferOption(v)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set TCP moderate receive buffer: %s", err)
		}
		return nil
	}
}

// WithTCPSACKEnabled sets the SACK option for TCP.
func WithTCPSACKEnabled(v bool) Option {
	return func(s *stack.Stack) error {
		opt := tcpip.TCPSACKEnabled(v)
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &opt); err != nil {
			return fmt.Errorf("set TCP SACK: %s", err)
		}
		return nil
	}
}

// WithTCPRecovery sets the recovery option for TCP.
func WithTCPRecovery(v tcpip.TCPRecovery) Option {
	return func(s *stack.Stack) error {
		if err := s.SetTransportProtocolOption(tcp.ProtocolNumber, &v); err != nil {
			return fmt.Errorf("set TCP Recovery: %s", err)
		}
		return nil
	}
}

func WithCreatingNIC(nicId tcpip.NICID, ep stack.LinkEndpoint) Option {
	return func(s *stack.Stack) error {
		if err := s.CreateNICWithOptions(nicId, ep,
			stack.NICOptions{
				Disabled: false,
				// If no queueing discipline was specified
				// provide a stub implementation that just
				// delegates to the lower link endpoint.
				QDisc: nil,
			}); err != nil {
			return fmt.Errorf("create NIC: %s", err)
		}
		return nil
	}
}

// WithPromiscuousMode sets promiscuous mode in the given NICs.
func WithPromiscuousMode(nicID tcpip.NICID, v bool) Option {
	return func(s *stack.Stack) error {
		if err := s.SetPromiscuousMode(nicID, v); err != nil {
			return fmt.Errorf("set promiscuous mode: %s", err)
		}
		return nil
	}
}

// WithSpoofing sets address spoofing in the given NICs, allowing
// endpoints to bind to any address in the NIC.
func WithSpoofing(nicID tcpip.NICID, v bool) Option {
	return func(s *stack.Stack) error {
		if err := s.SetSpoofing(nicID, v); err != nil {
			return fmt.Errorf("set spoofing: %s", err)
		}
		return nil
	}
}

// withMulticastGroups adds a NIC to the given multicast groups.
func WithMulticastGroups(nicID tcpip.NICID, multicastGroups []netip.Addr) Option {
	return func(s *stack.Stack) error {
		if len(multicastGroups) == 0 {
			return nil
		}
		// The default NIC of tun2socks is working on Spoofing mode. When the UDP Endpoint
		// tries to use a non-local address to connect, the network stack will
		// generate a temporary addressState to build the route, which can be primary
		// but is ephemeral. Nevertheless, when the UDP Endpoint tries to use a
		// multicast address to connect, the network stack will select an available
		// primary addressState to build the route. However, when tun2socks is in the
		// just-initialized or idle state, there will be no available primary addressState,
		// and the connect operation will fail. Therefore, we need to add permanent addresses,
		// e.g. 10.0.0.1/8 and fd00:1/8, to the default NIC, which are only used to build
		// routes for multicast response and do not affect other connections.
		//
		// In fact, for multicast, the sender normally does not expect a response.
		// So, the ep.net.Connect is unnecessary. If we implement a custom UDP Forwarder
		// and ForwarderRequest in the future, we can remove these code.
		s.AddProtocolAddress(
			nicID,
			tcpip.ProtocolAddress{
				Protocol: ipv4.ProtocolNumber,
				AddressWithPrefix: tcpip.AddressWithPrefix{
					Address:   tcpip.AddrFrom4([4]byte{0x0a, 0, 0, 0x01}),
					PrefixLen: 8,
				},
			},
			stack.AddressProperties{PEB: stack.CanBePrimaryEndpoint},
		)
		s.AddProtocolAddress(
			nicID,
			tcpip.ProtocolAddress{
				Protocol: ipv6.ProtocolNumber,
				AddressWithPrefix: tcpip.AddressWithPrefix{
					Address:   tcpip.AddrFrom16([16]byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}),
					PrefixLen: 8,
				},
			},
			stack.AddressProperties{PEB: stack.CanBePrimaryEndpoint},
		)
		for _, multicastGroup := range multicastGroups {
			var err tcpip.Error
			switch {
			case multicastGroup.Is4():
				err = s.JoinGroup(ipv4.ProtocolNumber, nicID, tcpip.AddrFrom4(multicastGroup.As4()))
			case multicastGroup.Is6():
				err = s.JoinGroup(ipv6.ProtocolNumber, nicID, tcpip.AddrFrom16(multicastGroup.As16()))
			}
			if err != nil {
				return fmt.Errorf("join multicast group: %s", err)
			}
		}
		return nil
	}
}

func WithRouteTable(nicID tcpip.NICID) Option {
	return func(s *stack.Stack) error {
		s.SetRouteTable([]tcpip.Route{
			{
				Destination: header.IPv4EmptySubnet,
				NIC:         nicID,
			},
			{
				Destination: header.IPv6EmptySubnet,
				NIC:         nicID,
			},
		})
		return nil
	}
}

func WithMainNICIP(nicID tcpip.NICID, ip tcpip.Address, mac net.HardwareAddr) Option {
	return func(s *stack.Stack) error {
		s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
			Protocol: header.IPv4ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFromSlice(ip.AsSlice()),
				PrefixLen: 24,
			},
		}, stack.AddressProperties{})
		tcpErr := s.SetNICAddress(nicID, tcpip.LinkAddress(mac))
		if tcpErr != nil {
			log.Errorf("set nic address failed: %v", tcpErr)
		}
		return nil
	}
}

func WithIPv4RouteTableDefaultGateway(nicID tcpip.NICID, gateway netip.Addr) Option {
	return func(s *stack.Stack) error {
		s.SetRouteTable([]tcpip.Route{
			{
				Destination: header.IPv4EmptySubnet,
				Gateway:     tcpip.AddrFrom4(gateway.As4()),
				NIC:         nicID,
			},
		})
		return nil
	}
}
