package l2tpserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
)

// Global counter for assigning unique tunnel IDs to clients
var nextClientTunnelID uint32 = 0

// Client represents an L2TP client
type Client struct {
	mu sync.RWMutex

	// Network
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr

	// L2TP state
	tunnelID      uint16 // Our tunnel ID
	peerTunnelID  uint16 // Server's tunnel ID
	sessionID     uint16 // Our session ID
	peerSessionID uint16 // Server's session ID
	ns            uint16 // Next send sequence number
	nr            uint16 // Next receive sequence number

	// PPP
	pppAuth       *ppp.PPPAuth
	authenticated bool

	// Network stack integration
	endpoint *channel.Endpoint
	netStack *stack.Stack
	nicID    tcpip.NICID

	// Configuration
	username string
	password string
	timeout  time.Duration

	// Context
	ctx    context.Context
	cancel context.CancelFunc

	// Callbacks
	onPacket func([]byte)
}

// ClientOption is a functional option for configuring the client
type ClientOption func(*ClientConfig) error

// ClientConfig holds client configuration
type ClientConfig struct {
	Username string
	Password string
	Timeout  time.Duration
	NetStack *stack.Stack
	NICID    tcpip.NICID
	OnPacket func([]byte)
}

// WithUsername sets the username for PPP authentication
func WithUsername(username string) ClientOption {
	return func(c *ClientConfig) error {
		c.Username = username
		return nil
	}
}

// WithPassword sets the password for PPP authentication
func WithPassword(password string) ClientOption {
	return func(c *ClientConfig) error {
		c.Password = password
		return nil
	}
}

// WithClientTimeout sets the timeout for connection establishment
func WithClientTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) error {
		c.Timeout = timeout
		return nil
	}
}

// WithClientNetStack sets the network stack for the client
func WithClientNetStack(s *stack.Stack, nicID tcpip.NICID) ClientOption {
	return func(c *ClientConfig) error {
		c.NetStack = s
		c.NICID = nicID
		return nil
	}
}

// WithOnPacket sets the packet callback
func WithOnPacket(callback func([]byte)) ClientOption {
	return func(c *ClientConfig) error {
		c.OnPacket = callback
		return nil
	}
}

// NewL2TPClient creates and connects a new L2TP client
func NewL2TPClient(serverAddr string, opts ...ClientOption) (*Client, error) {
	config := &ClientConfig{
		Username: "user",
		Password: "pass",
		Timeout:  10 * time.Second,
	}

	for _, opt := range opts {
		if err := opt(config); err != nil {
			return nil, err
		}
	}

	// Resolve server address
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve server address: %w", err)
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Assign unique tunnel ID to this client
	tunnelID := uint16(atomic.AddUint32(&nextClientTunnelID, 1))
	if tunnelID == 0 {
		// Skip 0 as it's reserved, get next ID
		tunnelID = uint16(atomic.AddUint32(&nextClientTunnelID, 1))
	}

	client := &Client{
		conn:       conn,
		remoteAddr: udpAddr,
		tunnelID:   tunnelID, // Each client gets a unique tunnel ID
		sessionID:  100,
		username:   config.Username,
		password:   config.Password,
		timeout:    config.Timeout,
		ctx:        ctx,
		cancel:     cancel,
		netStack:   config.NetStack,
		nicID:      config.NICID,
		onPacket:   config.OnPacket,
	}

	// Setup PPP auth
	client.pppAuth = ppp.GetDefaultPPPAuth()
	// Note: PPPAuth credentials are set via the auth handler callback

	// Setup network stack if provided
	if config.NetStack != nil {
		// Create channel endpoint with reasonable buffer sizes
		client.endpoint = channel.New(256, 1500, "")

		// The endpoint will be attached by the caller after getting the client
		// This allows the caller to configure the NIC properly
	}

	// Perform L2TP handshake
	if err := client.connect(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to establish L2TP connection: %w", err)
	}

	// Start receiving packets
	go client.receiveLoop()

	// Start endpoint read loop if network stack is configured
	if client.endpoint != nil {
		go client.endpointReadLoop()
	}

	return client, nil
}

// connect performs the L2TP connection handshake
func (c *Client) connect() error {
	// Step 1: Send SCCRQ (Start-Control-Connection-Request)
	log.Info("L2TP Client: Sending SCCRQ")
	sccrq := c.buildSCCRQ()
	if _, err := c.conn.Write(sccrq); err != nil {
		return fmt.Errorf("failed to send SCCRQ: %w", err)
	}

	// Step 2: Receive SCCRP (Start-Control-Connection-Reply)
	buf := make([]byte, 4096)
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	n, err := c.conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to receive SCCRP: %w", err)
	}

	header, offset, err := ParseL2TPHeader(buf[:n])
	if err != nil {
		return fmt.Errorf("failed to parse SCCRP header: %w", err)
	}

	if !header.IsControl() {
		return fmt.Errorf("expected control message, got data message")
	}

	avps, err := ParseAVPs(buf[offset:n])
	if err != nil {
		return fmt.Errorf("failed to parse SCCRP AVPs: %w", err)
	}

	var messageType uint16
	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
		}
		if avp.Type == AVPAssignedTunnelID {
			c.peerTunnelID, _ = avp.GetUint16()
		}
	}

	if messageType != SCCRP {
		return fmt.Errorf("expected SCCRP (2), got message type %d", messageType)
	}

	log.Infof("L2TP Client: Received SCCRP, peer tunnel ID: %d", c.peerTunnelID)

	// Update sequence numbers
	c.nr = header.Ns + 1
	c.ns++

	// Step 3: Send SCCCN (Start-Control-Connection-Connected)
	log.Info("L2TP Client: Sending SCCCN")
	scccn := c.buildSCCCN()
	if _, err := c.conn.Write(scccn); err != nil {
		return fmt.Errorf("failed to send SCCCN: %w", err)
	}
	c.ns++

	time.Sleep(100 * time.Millisecond)

	// Step 4: Send ICRQ (Incoming-Call-Request)
	log.Info("L2TP Client: Sending ICRQ")
	icrq := c.buildICRQ()
	if _, err := c.conn.Write(icrq); err != nil {
		return fmt.Errorf("failed to send ICRQ: %w", err)
	}

	// Step 5: Receive ICRP (Incoming-Call-Reply)
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	n, err = c.conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to receive ICRP: %w", err)
	}

	header, offset, err = ParseL2TPHeader(buf[:n])
	if err != nil {
		return fmt.Errorf("failed to parse ICRP header: %w", err)
	}

	avps, err = ParseAVPs(buf[offset:n])
	if err != nil {
		return fmt.Errorf("failed to parse ICRP AVPs: %w", err)
	}

	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
		}
		if avp.Type == AVPAssignedSessionID {
			c.peerSessionID, _ = avp.GetUint16()
		}
	}

	if messageType != ICRP {
		return fmt.Errorf("expected ICRP (11), got message type %d", messageType)
	}

	log.Infof("L2TP Client: Received ICRP, peer session ID: %d", c.peerSessionID)

	// Update sequence numbers
	c.nr = header.Ns + 1
	c.ns++

	// Step 6: Send ICCN (Incoming-Call-Connected)
	log.Info("L2TP Client: Sending ICCN")
	iccn := c.buildICCN()
	if _, err := c.conn.Write(iccn); err != nil {
		return fmt.Errorf("failed to send ICCN: %w", err)
	}
	c.ns++

	log.Info("L2TP Client: Connection established successfully")

	// Send PAP authentication request if username and password are provided
	if c.username != "" && c.password != "" {
		log.Info("L2TP Client: Sending PAP authentication request")
		if err := c.sendPAPRequest(); err != nil {
			return fmt.Errorf("failed to send PAP request: %w", err)
		}
	}

	return nil
}

// receiveLoop continuously receives and processes packets
func (c *Client) receiveLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := c.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Errorf("L2TP Client: Read error: %v", err)
			return
		}

		if err := c.handlePacket(buf[:n]); err != nil {
			log.Errorf("L2TP Client: Error handling packet: %v", err)
		}
	}
}

// handlePacket processes a received L2TP packet
func (c *Client) handlePacket(data []byte) error {
	header, offset, err := ParseL2TPHeader(data)
	if err != nil {
		return fmt.Errorf("failed to parse header: %w", err)
	}

	if header.IsControl() {
		// Handle control message
		return c.handleControlMessage(header, data[offset:])
	} else {
		// Handle data message (PPP payload)
		return c.handleDataMessage(data[offset:])
	}
}

// handleControlMessage processes control messages
func (c *Client) handleControlMessage(header *L2TPHeader, payload []byte) error {
	avps, err := ParseAVPs(payload)
	if err != nil {
		return fmt.Errorf("failed to parse AVPs: %w", err)
	}

	var messageType uint16
	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
			break
		}
	}

	log.Debugf("L2TP Client: Received control message type %d", messageType)

	// Update Nr
	if header.Flags&FlagSequence != 0 {
		c.mu.Lock()
		c.nr = header.Ns + 1
		c.mu.Unlock()
	}

	// Handle specific message types if needed
	switch messageType {
	case Hello:
		// Respond to Hello messages
		log.Debug("L2TP Client: Received Hello")
	case StopCCN:
		log.Info("L2TP Client: Received StopCCN, closing connection")
		c.Close()
	}

	return nil
}

// handleDataMessage processes data messages (PPP payload)
func (c *Client) handleDataMessage(payload []byte) error {
	if len(payload) < 2 {
		return fmt.Errorf("payload too short")
	}

	// PPP packet structure: Address(0xFF) + Control(0x03) + Protocol + Data
	// Some implementations may omit Address/Control

	var pppPayload []byte
	if payload[0] == 0xFF && len(payload) > 1 && payload[1] == 0x03 {
		// Address and Control present, skip them
		pppPayload = payload[2:]
	} else {
		// Address and Control omitted
		pppPayload = payload
	}

	// Extract protocol
	if len(pppPayload) < 2 {
		return fmt.Errorf("PPP payload too short")
	}

	protocol := binary.BigEndian.Uint16(pppPayload[0:2])
	data := pppPayload[2:]

	log.Debugf("L2TP Client: Received PPP packet, protocol: 0x%04x, length: %d", protocol, len(data))

	// Handle PPP protocol
	switch protocol {
	case 0xC021: // LCP (Link Control Protocol)
		log.Debug("L2TP Client: Received LCP packet")
		return c.handleLCP(data)
	case 0xC023: // PAP (Password Authentication Protocol)
		log.Debug("L2TP Client: Received PAP packet")
		return c.handlePAP(data)
	case 0xC223: // CHAP (Challenge Handshake Authentication Protocol)
		log.Debug("L2TP Client: Received CHAP packet")
		return c.handleCHAP(data)
	case 0x8021: // IPCP (IP Control Protocol)
		log.Debug("L2TP Client: Received IPCP packet")
		return c.handleIPCP(data)
	case 0x0021: // IPv4
		log.Debugf("L2TP Client: Received IPv4 packet, length: %d", len(data))
		return c.handleIPv4(data)
	default:
		log.Debugf("L2TP Client: Unhandled PPP protocol: 0x%04x", protocol)
	}

	return nil
}

// handleLCP handles LCP packets
func (c *Client) handleLCP(data []byte) error {
	// Simple LCP handling - just acknowledge
	// In a full implementation, we'd parse and respond appropriately
	log.Debug("L2TP Client: LCP packet received")
	return nil
}

// sendPAPRequest sends a PAP authentication request
func (c *Client) sendPAPRequest() error {
	c.mu.RLock()
	username := c.username
	password := c.password
	peerTunnelID := c.peerTunnelID
	peerSessionID := c.peerSessionID
	c.mu.RUnlock()

	if peerSessionID == 0 {
		return fmt.Errorf("session not established")
	}

	// Build PAP request
	// PAP format: Code(1) + ID(1) + Length(2) + PeerIDLength(1) + PeerID + PasswordLength(1) + Password
	papID := uint8(1)
	usernameBytes := []byte(username)
	passwordBytes := []byte(password)
	usernameLen := uint8(len(usernameBytes))
	passwordLen := uint8(len(passwordBytes))

	papLength := uint16(4 + 1 + len(usernameBytes) + 1 + len(passwordBytes))

	papData := make([]byte, papLength)
	papData[0] = 1                                      // Code: Authenticate-Request
	papData[1] = papID                                  // Identifier
	binary.BigEndian.PutUint16(papData[2:4], papLength) // Length
	papData[4] = usernameLen                            // Peer-ID Length
	copy(papData[5:], usernameBytes)                    // Peer-ID (username)
	papData[5+usernameLen] = passwordLen                // Password Length
	copy(papData[6+usernameLen:], passwordBytes)        // Password

	// Wrap in PPP frame
	// PPP format: Address(0xFF) + Control(0x03) + Protocol(0xC023 for PAP) + Data
	pppFrame := make([]byte, 4+len(papData))
	pppFrame[0] = 0xFF                                // Address
	pppFrame[1] = 0x03                                // Control
	binary.BigEndian.PutUint16(pppFrame[2:4], 0xC023) // Protocol: PAP
	copy(pppFrame[4:], papData)

	// Build L2TP data message
	header := &L2TPHeader{
		Flags:     L2TPVersion, // Data message
		TunnelID:  peerTunnelID,
		SessionID: peerSessionID,
	}

	headerData := header.Serialize()
	l2tpPacket := append(headerData, pppFrame...)

	_, err := c.conn.Write(l2tpPacket)
	if err != nil {
		return fmt.Errorf("failed to send PAP request: %w", err)
	}

	log.Infof("L2TP Client: Sent PAP request for user: %s", username)
	return nil
}

// handlePAP handles PAP authentication
func (c *Client) handlePAP(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("PAP packet too short: %d bytes", len(data))
	}

	code := data[0]
	identifier := data[1]
	length := binary.BigEndian.Uint16(data[2:4])

	log.Debugf("L2TP Client: PAP packet received - Code: %d, ID: %d, Length: %d", code, identifier, length)

	switch code {
	case 2: // PAP-Ack (Authentication successful)
		log.Info("L2TP Client: PAP authentication successful")
		c.mu.Lock()
		c.authenticated = true
		c.mu.Unlock()
	case 3: // PAP-Nak (Authentication failed)
		log.Warn("L2TP Client: PAP authentication failed")
		c.mu.Lock()
		c.authenticated = false
		c.mu.Unlock()
		return fmt.Errorf("PAP authentication failed")
	default:
		log.Debugf("L2TP Client: Unknown PAP code: %d", code)
	}

	return nil
}

// handleCHAP handles CHAP authentication
func (c *Client) handleCHAP(data []byte) error {
	log.Debug("L2TP Client: CHAP packet received")
	return nil
}

// handleIPCP handles IPCP negotiation
func (c *Client) handleIPCP(data []byte) error {
	log.Debug("L2TP Client: IPCP packet received")
	c.authenticated = true
	return nil
}

// handleIPv4 handles IPv4 packets
func (c *Client) handleIPv4(data []byte) error {
	// Call packet callback if set
	if c.onPacket != nil {
		c.onPacket(data)
	}

	// Inject into network stack if available
	if c.endpoint != nil {
		c.endpoint.InjectInbound(tcpip.NetworkProtocolNumber(0x0800), stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(data),
		}))
	}

	return nil
}

// InjectPacket sends an IP packet through the L2TP tunnel
func (c *Client) InjectPacket(packet []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.peerSessionID == 0 {
		return fmt.Errorf("session not established")
	}

	// Build L2TP data message
	// Data messages don't have sequence numbers or length field typically
	header := &L2TPHeader{
		Flags:     L2TPVersion, // Just version, no other flags for data
		TunnelID:  c.peerTunnelID,
		SessionID: c.peerSessionID,
	}

	headerData := header.Serialize()

	// PPP encapsulation: 0xFF 0x03 (Address/Control) + 0x0021 (IPv4) + packet
	pppPacket := make([]byte, 4+len(packet))
	pppPacket[0] = 0xFF // Address
	pppPacket[1] = 0x03 // Control
	pppPacket[2] = 0x00 // Protocol (IPv4) high byte
	pppPacket[3] = 0x21 // Protocol (IPv4) low byte
	copy(pppPacket[4:], packet)

	// Combine header and PPP packet
	l2tpPacket := append(headerData, pppPacket...)

	_, err := c.conn.Write(l2tpPacket)
	if err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}

	log.Debugf("L2TP Client: Sent IPv4 packet, length: %d", len(packet))
	return nil
}

// GetEndpoint returns the channel endpoint for network stack integration
func (c *Client) GetEndpoint() *channel.Endpoint {
	return c.endpoint
}

// GetTunnelID returns the client's tunnel ID
func (c *Client) GetTunnelID() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tunnelID
}

// GetPeerTunnelID returns the server's tunnel ID
func (c *Client) GetPeerTunnelID() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.peerTunnelID
}

// GetSessionID returns the client's session ID
func (c *Client) GetSessionID() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// GetPeerSessionID returns the server's session ID
func (c *Client) GetPeerSessionID() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.peerSessionID
}

// GetConn returns the UDP connection
func (c *Client) GetConn() *net.UDPConn {
	return c.conn
}

// GetRemoteAddr returns the remote UDP address
func (c *Client) GetRemoteAddr() *net.UDPAddr {
	return c.remoteAddr
}

// Close closes the L2TP client connection
func (c *Client) Close() error {
	c.cancel()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Build control messages

func (c *Client) buildSCCRQ() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  0,
		SessionID: 0,
		Ns:        c.ns,
		Nr:        c.nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, SCCRQ, true),
		CreateUint16AVP(AVPProtocolVersion, 0x0100, true),
		CreateStringAVP(AVPHostName, "yak-client", true),
		CreateUint16AVP(AVPAssignedTunnelID, c.tunnelID, true),
		CreateUint16AVP(AVPReceiveWindowSize, 4, true),
	}

	return c.buildControlMessage(header, avps)
}

func (c *Client) buildSCCCN() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  c.peerTunnelID,
		SessionID: 0,
		Ns:        c.ns,
		Nr:        c.nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, SCCCN, true),
	}

	return c.buildControlMessage(header, avps)
}

func (c *Client) buildICRQ() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  c.peerTunnelID,
		SessionID: 0,
		Ns:        c.ns,
		Nr:        c.nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, ICRQ, true),
		CreateUint16AVP(AVPAssignedSessionID, c.sessionID, true),
		CreateUint32AVP(AVPCallSerialNumber, 1, true),
	}

	return c.buildControlMessage(header, avps)
}

func (c *Client) buildICCN() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  c.peerTunnelID,
		SessionID: c.peerSessionID,
		Ns:        c.ns,
		Nr:        c.nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, ICCN, true),
		CreateUint32AVP(AVPTxConnectSpeed, 100000000, true),
		CreateUint32AVP(AVPFramingType, 3, true),
	}

	return c.buildControlMessage(header, avps)
}

func (c *Client) buildControlMessage(header *L2TPHeader, avps []AVP) []byte {
	var avpData []byte
	for _, avp := range avps {
		avpData = append(avpData, avp.Serialize()...)
	}

	headerData := header.Serialize()
	totalLength := uint16(len(headerData) + len(avpData))

	if header.Flags&FlagLength != 0 {
		// Update length field at offset 6
		binary.BigEndian.PutUint16(headerData[6:8], totalLength)
	}

	return append(headerData, avpData...)
}

// endpointReadLoop reads packets from the network stack endpoint and sends them through L2TP
func (c *Client) endpointReadLoop() {
	log.Info("L2TP Client: Starting endpoint read loop")
	defer log.Info("L2TP Client: Endpoint read loop stopped")

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Read packet from endpoint (this blocks until a packet is available)
		pkt := c.endpoint.ReadContext(c.ctx)
		if pkt == nil {
			// Context was cancelled or endpoint closed
			return
		}

		// Extract the IP packet data from all slices
		var ipPacket []byte
		for _, slice := range pkt.AsSlices() {
			ipPacket = append(ipPacket, slice...)
		}

		// Send the packet through L2TP tunnel
		if err := c.InjectPacket(ipPacket); err != nil {
			log.Errorf("L2TP Client: Failed to send packet from endpoint: %v", err)
		}

		// Release the packet buffer
		pkt.DecRef()
	}
}
