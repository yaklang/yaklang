package netstackvm

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	gvisorDHCP "github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/dhcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"golang.org/x/time/rate"
)

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

type Option func(*Config) error

type Config struct {
	ctx    context.Context
	cancel context.CancelFunc

	pcapPromisc bool
	pcapDevice  string

	// stack options
	IPv4Disabled                bool
	IPv6Disabled                bool
	DHCPDisabled                bool
	ARPDisabled                 bool
	ICMPDisabled                bool
	HandleLocal                 bool
	TCPDisabled                 bool
	UDPDisabled                 bool
	DisallowPacketEndpointWrite bool
	EnableLinkLayer             bool
	OnTCPConnectionRequested    func(*tcpip.FullAddress, *tcpip.FullAddress)

	//dhcp config
	DHCPAcquireTimeout       time.Duration
	DHCPAcquireInterval      time.Duration
	DHCPAcquireRetryInterval time.Duration
	DHCPAcquireCallback      func(ctx context.Context, lost, acquired tcpip.AddressWithPrefix, cfg gvisorDHCP.Config)

	// nic options
	MainNICIPv4Address        string
	MainNICIPv4AddressNetmask string

	MainNICIPv6Address        string
	MainNICIPv6AddressNetmask string
	MainNICLinkAddress        net.HardwareAddr

	// tcp options
	// DefaultTTL specifies the default TTL used by stack
	DefaultTTL uint8
	// Forwarding enables packet forwarding between NICs
	Forwarding bool
	// ICMPBurst is the number of ICMP messages that can be sent in a single burst
	ICMPBurst int
	// ICMPLimit is the maximum number of ICMP messages permitted by rate limiter
	ICMPLimit rate.Limit
	// TCPSendBufferSizeMin is the smallest size of a send buffer
	TCPSendBufferSizeMin int
	// TCPSendBufferSizeMax is the maximum permitted size of a send buffer
	TCPSendBufferSizeMax int
	// TCPSendBufferSizeDefault is the default size of the send buffer
	TCPSendBufferSizeDefault int
	// TCPReceiveBufferSizeMin is the smallest size of a receive buffer
	TCPReceiveBufferSizeMin int
	// TCPReceiveBufferSizeMax is the maximum permitted size of a receive buffer
	TCPReceiveBufferSizeMax int
	// TCPReceiveBufferSizeDefault is the default size of the receive buffer
	TCPReceiveBufferSizeDefault int
	// TCPCongestionControl is the congestion control algorithm used by TCP (reno or cubic)
	TCPCongestionControl string
	// TCPDelayEnabled enables/disables Nagle's algorithm for TCP
	TCPDelayEnabled bool
	// TCPModerateReceiveBuffer enables/disables TCP receive buffer auto-tuning
	TCPModerateReceiveBuffer bool
	// TCPSACKEnabled enables/disables TCP selective acknowledgment
	TCPSACKEnabled bool
	// TCPRACKLossDetection specifies the TCP loss detection algorithm
	TCPRACKLossDetection tcpip.TCPRecovery
}

func NewDefaultConfig() *Config {
	return &Config{
		DefaultTTL:                  defaultTimeToLive,
		Forwarding:                  ipForwardingEnabled,
		ICMPBurst:                   icmpBurst,
		ICMPLimit:                   icmpLimit,
		TCPSendBufferSizeMin:        tcpMinBufferSize,
		TCPSendBufferSizeMax:        tcpMaxBufferSize,
		TCPSendBufferSizeDefault:    tcpDefaultSendBufferSize,
		TCPReceiveBufferSizeMin:     tcpMinBufferSize,
		TCPReceiveBufferSizeMax:     tcpMaxBufferSize,
		TCPReceiveBufferSizeDefault: tcpDefaultReceiveBufferSize,
		TCPCongestionControl:        tcpCongestionControlAlgorithm,
		TCPDelayEnabled:             tcpDelayEnabled,
		TCPModerateReceiveBuffer:    tcpModerateReceiveBufferEnabled,
		TCPSACKEnabled:              tcpSACKEnabled,
		TCPRACKLossDetection:        tcpRecovery,
		DHCPAcquireTimeout:          time.Second * 5,
		DHCPAcquireInterval:         time.Second * 2,
		DHCPAcquireRetryInterval:    time.Second * 2,
	}
}

func WithMainNICLinkAddress(linkAddress string) Option {
	return func(c *Config) error {
		mac, err := net.ParseMAC(linkAddress)
		if err != nil {
			return fmt.Errorf("invalid link address: %s", linkAddress)
		}
		c.MainNICLinkAddress = mac
		return nil
	}
}

func WithRandomMainNICLinkAddress() Option {
	return func(c *Config) error {
		mac := make([]byte, 6)
		rand.Read(mac)
		mac[0] = (mac[0] | 2) & 0xfe // Set local bit, ensure unicast
		c.MainNICLinkAddress = net.HardwareAddr(mac)
		return nil
	}
}

func WithMainNICIPAddress(ipAddress string) Option {
	return func(c *Config) error {
		ip := net.ParseIP(ipAddress)
		if ip == nil {
			return fmt.Errorf("invalid ip address: %s", ipAddress)
		}
		if ip.To4() != nil {
			c.MainNICIPv4Address = ipAddress
		} else {
			c.MainNICIPv6Address = ipAddress
		}
		return nil
	}
}

func WithHandleLocal(handleLocal bool) Option {
	return func(c *Config) error {
		c.HandleLocal = handleLocal
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Config) error {
		c.ctx, c.cancel = context.WithCancel(ctx)
		return nil
	}
}

func WithPcapPromisc(promisc bool) Option {
	return func(c *Config) error {
		c.pcapPromisc = promisc
		return nil
	}
}

func WithPcapDevice(device string) Option {
	return func(c *Config) error {
		c.pcapDevice = device
		return nil
	}
}

func WithEnableLinkLayer(enable bool) Option {
	return func(c *Config) error {
		c.EnableLinkLayer = enable
		return nil
	}
}

func WithDisallowPacketEndpointWrite(disallow bool) Option {
	return func(c *Config) error {
		c.DisallowPacketEndpointWrite = disallow
		return nil
	}
}

func WithDHCPDisabled(disabled bool) Option {
	return func(c *Config) error {
		c.DHCPDisabled = disabled
		return nil
	}
}

func WithARPDisabled(disabled bool) Option {
	return func(c *Config) error {
		c.ARPDisabled = disabled
		return nil
	}
}

func WithOnTCPConnectionRequested(fn func(*tcpip.FullAddress, *tcpip.FullAddress)) Option {
	return func(c *Config) error {
		c.OnTCPConnectionRequested = fn
		return nil
	}
}

func WithIPv4Disabled(disabled bool) Option {
	return func(c *Config) error {
		c.IPv4Disabled = disabled
		return nil
	}
}

func WithIPv6Disabled(disabled bool) Option {
	return func(c *Config) error {
		c.IPv6Disabled = disabled
		return nil
	}
}

func WithICMPDisabled(disabled bool) Option {
	return func(c *Config) error {
		c.ICMPDisabled = disabled
		return nil
	}
}

func WithTCPDisabled(disabled bool) Option {
	return func(c *Config) error {
		c.TCPDisabled = disabled
		return nil
	}
}

func WithUDPDisabled(disabled bool) Option {
	return func(c *Config) error {
		c.UDPDisabled = disabled
		return nil
	}
}
