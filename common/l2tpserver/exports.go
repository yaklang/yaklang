package l2tpserver

import (
	"net"

	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

// ServerOption is a functional option for configuring the server
type ServerOption func(*Config) error

// WithListenAddr sets the listen address
func WithListenAddr(addr string) ServerOption {
	return func(c *Config) error {
		c.ListenAddr = addr
		return nil
	}
}

// WithHostname sets the hostname
func WithHostname(hostname string) ServerOption {
	return func(c *Config) error {
		c.Hostname = hostname
		return nil
	}
}

// WithVendorName sets the vendor name
func WithVendorName(vendor string) ServerOption {
	return func(c *Config) error {
		c.VendorName = vendor
		return nil
	}
}

// WithAuthFunc sets the authentication function
func WithAuthFunc(authFunc func(username, password string) bool) ServerOption {
	return func(c *Config) error {
		c.AuthFunc = authFunc
		return nil
	}
}

// WithNetStack sets the network stack for packet injection
func WithNetStack(s *stack.Stack, nicID tcpip.NICID) ServerOption {
	return func(c *Config) error {
		c.NetStack = s
		c.NICID = nicID
		return nil
	}
}

// WithIPPool sets the IP address pool range
func WithIPPool(start, end net.IP) ServerOption {
	return func(c *Config) error {
		c.IPPoolStart = start
		c.IPPoolEnd = end
		return nil
	}
}

// NewL2TPServer creates a new L2TP server with functional options
func NewL2TPServer(opts ...ServerOption) (*Server, error) {
	config := &Config{}

	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	return NewServer(config)
}

// StartL2TPServer creates and starts an L2TP server
func StartL2TPServer(opts ...ServerOption) (*Server, error) {
	server, err := NewL2TPServer(opts...)
	if err != nil {
		return nil, err
	}

	if err := server.Start(); err != nil {
		return nil, err
	}

	return server, nil
}
