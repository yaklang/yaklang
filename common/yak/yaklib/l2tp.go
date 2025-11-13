package yaklib

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/l2tpserver"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
	"github.com/yaklang/yaklang/common/utils"
)

// L2TPServer represents a running L2TP server
type L2TPServer struct {
	server          *l2tpserver.Server
	ctx             context.Context
	cancel          context.CancelFunc
	packetCallbacks []func([]byte)
	endpoint        *channel.Endpoint
}

// L2TPClient represents an L2TP client connection
type L2TPClient struct {
	client *l2tpserver.Client
}

type l2tpServerConfig struct {
	ctx            context.Context
	host           string
	port           int
	hostname       string
	vendorName     string
	authFunc       func(string, string) bool
	ipPoolStart    net.IP
	ipPoolEnd      net.IP
	packetCallback func([]byte)
	enableNetStack bool
	gatewayIP      string
}

type l2tpClientConfig struct {
	ctx            context.Context
	username       string
	password       string
	timeout        time.Duration
	packetCallback func([]byte)
}

type L2TPServerOpt func(*l2tpServerConfig)
type L2TPClientOpt func(*l2tpClientConfig)

// Server options

func _l2tpServerHost(host string) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.host = host
	}
}

func _l2tpServerPort(port int) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.port = port
	}
}

func _l2tpServerContext(ctx context.Context) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.ctx = ctx
	}
}

func _l2tpServerHostname(hostname string) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.hostname = hostname
	}
}

func _l2tpServerVendorName(vendor string) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.vendorName = vendor
	}
}

func _l2tpServerAuth(authFunc func(string, string) bool) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.authFunc = authFunc
	}
}

func _l2tpServerIPPool(start, end string) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.ipPoolStart = net.ParseIP(start)
		c.ipPoolEnd = net.ParseIP(end)
	}
}

func _l2tpServerPacketCallback(callback func([]byte)) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.packetCallback = callback
	}
}

func _l2tpServerEnableNetStack(enable bool) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.enableNetStack = enable
	}
}

func _l2tpServerGatewayIP(ip string) L2TPServerOpt {
	return func(c *l2tpServerConfig) {
		c.gatewayIP = ip
	}
}

// Client options

func _l2tpClientContext(ctx context.Context) L2TPClientOpt {
	return func(c *l2tpClientConfig) {
		c.ctx = ctx
	}
}

func _l2tpClientAuth(username, password string) L2TPClientOpt {
	return func(c *l2tpClientConfig) {
		c.username = username
		c.password = password
	}
}

func _l2tpClientTimeout(timeout float64) L2TPClientOpt {
	return func(c *l2tpClientConfig) {
		c.timeout = time.Duration(float64(time.Second) * timeout)
	}
}

func _l2tpClientPacketCallback(callback func([]byte)) L2TPClientOpt {
	return func(c *l2tpClientConfig) {
		c.packetCallback = callback
	}
}

// Serve starts an L2TP server
func _l2tpServe(opts ...L2TPServerOpt) (*L2TPServer, error) {
	config := &l2tpServerConfig{
		ctx:            context.Background(),
		host:           "0.0.0.0",
		port:           1701, // Default L2TP port
		hostname:       "yaklang-l2tp",
		vendorName:     "Yaklang",
		authFunc:       func(u, p string) bool { return true }, // Default: accept all
		ipPoolStart:    net.IPv4(172, 16, 0, 10),
		ipPoolEnd:      net.IPv4(172, 16, 0, 100),
		enableNetStack: true,
		gatewayIP:      "172.16.0.1",
	}

	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(config.ctx)

	var serverOpts []l2tpserver.ServerOption

	// Set listen address
	addr := fmt.Sprintf("%s:%d", config.host, config.port)
	serverOpts = append(serverOpts, l2tpserver.WithListenAddr(addr))
	serverOpts = append(serverOpts, l2tpserver.WithHostname(config.hostname))
	serverOpts = append(serverOpts, l2tpserver.WithVendorName(config.vendorName))
	serverOpts = append(serverOpts, l2tpserver.WithAuthFunc(config.authFunc))
	serverOpts = append(serverOpts, l2tpserver.WithIPPool(config.ipPoolStart, config.ipPoolEnd))

	// Add packet callback if provided
	if config.packetCallback != nil {
		serverOpts = append(serverOpts, l2tpserver.WithServerOnPacket(config.packetCallback))
	}

	var endpoint *channel.Endpoint
	var packetCallbacks []func([]byte)

	// Setup network stack if enabled
	if config.enableNetStack {
		// Create network stack
		s := stack.New(stack.Options{
			NetworkProtocols: []stack.NetworkProtocolFactory{
				ipv4.NewProtocol,
			},
			TransportProtocols: []stack.TransportProtocolFactory{
				tcp.NewProtocol,
				udp.NewProtocol,
				icmp.NewProtocol4,
			},
		})

		// Create hijacked endpoint with callback
		endpoint = channel.New(512, 1500, "")

		// Setup packet callback if provided
		if config.packetCallback != nil {
			packetCallbacks = append(packetCallbacks, config.packetCallback)

			// Start goroutine to read packets from endpoint
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					default:
						pkt := endpoint.ReadContext(ctx)
						if pkt == nil {
							continue
						}

						// Extract packet data
						var data []byte
						for _, v := range pkt.AsSlices() {
							data = append(data, v...)
						}

						// Call all callbacks
						for _, cb := range packetCallbacks {
							cb(data)
						}

						pkt.DecRef()
					}
				}
			}()
		}

		nicID := tcpip.NICID(1)
		if err := s.CreateNIC(nicID, endpoint); err != nil {
			cancel()
			return nil, utils.Errorf("create NIC failed: %v", err)
		}

		// Configure gateway IP
		gatewayIP := net.ParseIP(config.gatewayIP)
		if gatewayIP == nil {
			gatewayIP = net.IPv4(172, 16, 0, 1)
		}

		addr := tcpip.AddrFrom4([4]byte{gatewayIP[12], gatewayIP[13], gatewayIP[14], gatewayIP[15]})
		protocolAddr := tcpip.ProtocolAddress{
			Protocol: header.IPv4ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   addr,
				PrefixLen: 24,
			},
		}

		if err := s.AddProtocolAddress(nicID, protocolAddr, stack.AddressProperties{}); err != nil {
			cancel()
			return nil, utils.Errorf("add protocol address failed: %v", err)
		}

		// Set route table
		s.SetRouteTable([]tcpip.Route{
			{
				Destination: header.IPv4EmptySubnet,
				NIC:         nicID,
			},
		})

		serverOpts = append(serverOpts, l2tpserver.WithNetStack(s, nicID))
	}

	// Start server
	server, err := l2tpserver.StartL2TPServer(serverOpts...)
	if err != nil {
		cancel()
		return nil, utils.Errorf("start L2TP server failed: %v", err)
	}

	l2tpServer := &L2TPServer{
		server:          server,
		ctx:             ctx,
		cancel:          cancel,
		packetCallbacks: packetCallbacks,
		endpoint:        endpoint,
	}

	log.Infof("L2TP server started on %s:%d", config.host, config.port)
	return l2tpServer, nil
}

// Stop stops the L2TP server
func (s *L2TPServer) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.server != nil {
		return s.server.Stop()
	}
	return nil
}

// OnPacket adds a packet callback
func (s *L2TPServer) OnPacket(callback func([]byte)) {
	s.packetCallbacks = append(s.packetCallbacks, callback)
}

// InjectPacket injects a packet into the network stack
func (s *L2TPServer) InjectPacket(data []byte) error {
	if s.endpoint == nil {
		return utils.Error("network stack not enabled")
	}

	// Create packet buffer
	buf := buffer.MakeWithData(data)
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buf,
	})
	defer pkt.DecRef()

	// Inject to endpoint
	s.endpoint.InjectInbound(header.IPv4ProtocolNumber, pkt)
	return nil
}

// Connect establishes an L2TP client connection
func _l2tpConnect(host string, port interface{}, opts ...L2TPClientOpt) (*L2TPClient, error) {
	config := &l2tpClientConfig{
		ctx:      context.Background(),
		username: "test",
		password: "test",
		timeout:  10 * time.Second,
	}

	for _, opt := range opts {
		opt(config)
	}

	serverAddr := utils.HostPort(host, port)

	// Build client options
	var clientOpts []l2tpserver.ClientOption
	if config.username != "" {
		clientOpts = append(clientOpts, l2tpserver.WithUsername(config.username))
	}
	if config.password != "" {
		clientOpts = append(clientOpts, l2tpserver.WithPassword(config.password))
	}
	if config.timeout > 0 {
		clientOpts = append(clientOpts, l2tpserver.WithClientTimeout(config.timeout))
	}
	if config.packetCallback != nil {
		clientOpts = append(clientOpts, l2tpserver.WithOnPacket(config.packetCallback))
	}

	// Create L2TP client
	client, err := l2tpserver.NewL2TPClient(serverAddr, clientOpts...)
	if err != nil {
		return nil, utils.Errorf("create L2TP client failed: %v", err)
	}

	l2tpClient := &L2TPClient{
		client: client,
	}

	log.Infof("L2TP client connected to %s", serverAddr)
	return l2tpClient, nil
}

// InjectPacket sends an IP packet through the L2TP tunnel
func (c *L2TPClient) InjectPacket(packet []byte) error {
	if c.client == nil {
		return utils.Error("client not initialized")
	}
	return c.client.InjectPacket(packet)
}

// Close closes the client connection
func (c *L2TPClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// GetTunnelID returns the assigned tunnel ID
func (c *L2TPClient) GetTunnelID() uint16 {
	if c.client != nil {
		return c.client.GetTunnelID()
	}
	return 0
}

// GetPeerTunnelID returns the peer's tunnel ID
func (c *L2TPClient) GetPeerTunnelID() uint16 {
	if c.client != nil {
		return c.client.GetPeerTunnelID()
	}
	return 0
}

// GetSessionID returns the assigned session ID
func (c *L2TPClient) GetSessionID() uint16 {
	if c.client != nil {
		return c.client.GetSessionID()
	}
	return 0
}

// GetPeerSessionID returns the peer's session ID
func (c *L2TPClient) GetPeerSessionID() uint16 {
	if c.client != nil {
		return c.client.GetPeerSessionID()
	}
	return 0
}

// GetEndpoint returns the channel endpoint for network stack integration
func (c *L2TPClient) GetEndpoint() *channel.Endpoint {
	if c.client != nil {
		return c.client.GetEndpoint()
	}
	return nil
}

var L2TPExports = map[string]interface{}{
	// Server functions
	"Serve": _l2tpServe,

	// Server options
	"host":           _l2tpServerHost,
	"port":           _l2tpServerPort,
	"context":        _l2tpServerContext,
	"hostname":       _l2tpServerHostname,
	"vendorName":     _l2tpServerVendorName,
	"auth":           _l2tpServerAuth,
	"ipPool":         _l2tpServerIPPool,
	"callback":       _l2tpServerPacketCallback,
	"enableNetStack": _l2tpServerEnableNetStack,
	"gatewayIP":      _l2tpServerGatewayIP,

	// Client functions
	"Connect": _l2tpConnect,

	// Client options
	"clientContext":  _l2tpClientContext,
	"clientAuth":     _l2tpClientAuth,
	"clientTimeout":  _l2tpClientTimeout,
	"clientCallback": _l2tpClientPacketCallback,
}
