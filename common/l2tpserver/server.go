package l2tpserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	DefaultL2TPPort      = 1701
	DefaultMTU           = 1500
	DefaultIdleTimeout   = 60 * time.Second
	DefaultHelloInterval = 30 * time.Second
)

// Server represents an L2TP server
type Server struct {
	mu            sync.RWMutex
	conn          *net.UDPConn
	tunnels       map[string]*Tunnel // key: remoteAddr.String()
	tunnelsByID   map[uint16]*Tunnel // key: tunnel ID
	nextTunnelID  uint16
	nextSessionID uint16
	ctx           context.Context
	cancel        context.CancelFunc

	// Configuration
	hostname   string
	vendorName string
	listenAddr string

	// Authentication
	authFunc func(username, password string) bool

	// Network stack integration
	endpoint *channel.Endpoint
	stack    *stack.Stack
	nicID    tcpip.NICID

	// IP address pool
	ipPool *IPPool

	// Packet callback
	onPacket func([]byte)
}

// Config holds server configuration
type Config struct {
	ListenAddr  string
	Hostname    string
	VendorName  string
	AuthFunc    func(username, password string) bool
	NetStack    *stack.Stack
	NICID       tcpip.NICID
	IPPoolStart net.IP
	IPPoolEnd   net.IP
	OnPacket    func([]byte)
}

// NewServer creates a new L2TP server
func NewServer(config *Config) (*Server, error) {
	if config.ListenAddr == "" {
		config.ListenAddr = fmt.Sprintf(":%d", DefaultL2TPPort)
	}
	if config.Hostname == "" {
		config.Hostname = "yaklang-l2tp"
	}
	if config.VendorName == "" {
		config.VendorName = "Yaklang"
	}
	if config.AuthFunc == nil {
		// Default: accept any username/password
		config.AuthFunc = func(username, password string) bool {
			log.Infof("Auth request: username=%s, password=%s", username, password)
			return true
		}
	}

	// Create IP pool
	ipPool := NewIPPool(config.IPPoolStart, config.IPPoolEnd)

	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		tunnels:       make(map[string]*Tunnel),
		tunnelsByID:   make(map[uint16]*Tunnel),
		nextTunnelID:  1,
		nextSessionID: 1,
		ctx:           ctx,
		cancel:        cancel,
		hostname:      config.Hostname,
		vendorName:    config.VendorName,
		listenAddr:    config.ListenAddr,
		authFunc:      config.AuthFunc,
		stack:         config.NetStack,
		nicID:         config.NICID,
		ipPool:        ipPool,
		onPacket:      config.OnPacket,
	}

	// Create channel endpoint for network stack integration
	if server.stack != nil {
		server.endpoint = channel.New(512, uint32(DefaultMTU), "")
	}

	return server, nil
}

// Start starts the L2TP server
func (s *Server) Start() error {
	addr, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return utils.Errorf("resolve UDP address failed: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return utils.Errorf("listen UDP failed: %v", err)
	}

	s.conn = conn
	log.Infof("L2TP server listening on %s", s.listenAddr)

	// Start packet receiver
	go s.receiveLoop()

	// Start cleanup goroutine
	go s.cleanupLoop()

	// Attach endpoint to stack if available
	if s.stack != nil && s.endpoint != nil {
		// The endpoint will be attached when creating/configuring the NIC
		// Start endpoint reader
		go s.endpointReadLoop()
	}

	return nil
}

// Stop stops the L2TP server
func (s *Server) Stop() error {
	s.cancel()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Close all tunnels
	for _, tunnel := range s.tunnels {
		tunnel.Close()
	}

	if s.conn != nil {
		s.conn.Close()
	}

	return nil
}

// GetAddr returns the server's listening address
func (s *Server) GetAddr() string {
	if s.conn != nil {
		return s.conn.LocalAddr().String()
	}
	return ""
}

// receiveLoop receives and processes UDP packets
func (s *Server) receiveLoop() {
	buf := make([]byte, 65536)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Errorf("Read from UDP failed: %v", err)
				continue
			}
		}

		// Process packet
		go s.handlePacket(buf[:n], remoteAddr)
	}
}

// handlePacket processes a received L2TP packet
func (s *Server) handlePacket(data []byte, remoteAddr *net.UDPAddr) {
	// Parse L2TP header
	header, offset, err := ParseL2TPHeader(data)
	if err != nil {
		log.Errorf("Parse L2TP header failed: %v", err)
		return
	}

	payload := data[offset:]

	if header.IsControl() {
		// Control message
		s.handleControlMessage(header, payload, remoteAddr)
	} else {
		// Data message
		s.handleDataMessage(header, payload, remoteAddr)
	}
}

// handleControlMessage processes a control message
func (s *Server) handleControlMessage(header *L2TPHeader, payload []byte, remoteAddr *net.UDPAddr) {
	// Parse AVPs
	avps, err := ParseAVPs(payload)
	if err != nil {
		log.Errorf("Parse AVPs failed: %v", err)
		return
	}

	// Get message type
	var messageType uint16
	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
			break
		}
	}

	log.Infof("Received control message type %d from %s (tunnel=%d, session=%d)",
		messageType, remoteAddr, header.TunnelID, header.SessionID)

	// Get or create tunnel
	addrKey := remoteAddr.String()
	tunnel := s.getTunnel(addrKey)

	// Update sequence number if present
	if header.HasSequence() {
		if tunnel != nil {
			tunnel.ValidateSequence(header.Ns)
		}
	}

	switch messageType {
	case SCCRQ:
		s.handleSCCRQ(header, avps, remoteAddr)
	case SCCCN:
		s.handleSCCCN(header, avps, remoteAddr)
	case ICRQ:
		s.handleICRQ(header, avps, remoteAddr)
	case ICCN:
		s.handleICCN(header, avps, remoteAddr)
	case CDN:
		s.handleCDN(header, avps, remoteAddr)
	case StopCCN:
		s.handleStopCCN(header, avps, remoteAddr)
	case Hello:
		s.handleHello(header, avps, remoteAddr)
	default:
		log.Warnf("Unknown control message type: %d", messageType)
	}
}

// handleSCCRQ handles Start-Control-Connection-Request
func (s *Server) handleSCCRQ(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling SCCRQ from %s", remoteAddr)

	// Extract peer tunnel ID and other info
	var peerTunnelID uint16
	var hostname string
	var vendorName string
	for _, avp := range avps {
		switch avp.Type {
		case AVPAssignedTunnelID:
			peerTunnelID, _ = avp.GetUint16()
		case AVPHostName:
			hostname = avp.GetString()
		case AVPVendorName:
			vendorName = avp.GetString()
		}
	}

	log.Debugf("Client connection request: addr=%s, hostname=%s, vendor=%s, peer_tunnel_id=%d",
		remoteAddr, hostname, vendorName, peerTunnelID)

	// Allocate our tunnel ID
	ourTunnelID := s.allocateTunnelID()

	// Create tunnel
	tunnel := NewTunnel(ourTunnelID, peerTunnelID, remoteAddr, s.ctx)

	s.mu.Lock()
	s.tunnels[remoteAddr.String()] = tunnel
	s.tunnelsByID[ourTunnelID] = tunnel
	s.mu.Unlock()

	log.Infof("Created tunnel: our_id=%d, peer_id=%d", ourTunnelID, peerTunnelID)

	// Send SCCRP
	s.sendSCCRP(tunnel)
}

// handleSCCCN handles Start-Control-Connection-Connected
func (s *Server) handleSCCCN(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling SCCCN from %s", remoteAddr)

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel != nil {
		tunnel.UpdateActivity()
		log.Infof("L2TP tunnel %d established with %s", tunnel.tunnelID, remoteAddr)
	}
}

// handleICRQ handles Incoming-Call-Request
func (s *Server) handleICRQ(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling ICRQ from %s", remoteAddr)

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel == nil {
		log.Errorf("No tunnel found for ICRQ from %s", remoteAddr)
		return
	}

	// Extract peer session ID
	var peerSessionID uint16
	for _, avp := range avps {
		if avp.Type == AVPAssignedSessionID {
			peerSessionID, _ = avp.GetUint16()
			break
		}
	}

	// Allocate our session ID
	ourSessionID := s.allocateSessionID()

	// Create session
	session := NewSession(ourSessionID, peerSessionID, tunnel)
	tunnel.AddSession(session)

	log.Infof("Created session: our_id=%d, peer_id=%d, tunnel_id=%d", ourSessionID, peerSessionID, tunnel.tunnelID)

	// Send ICRP
	s.sendICRP(tunnel, session)
}

// handleICCN handles Incoming-Call-Connected
func (s *Server) handleICCN(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling ICCN from %s (session=%d)", remoteAddr, header.SessionID)

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel == nil {
		return
	}

	session, ok := tunnel.GetSession(header.SessionID)
	if ok {
		session.UpdateActivity()
		log.Infof("Session %d established, ready for authentication", session.sessionID)
	}
}

// handleCDN handles Call-Disconnect-Notify
func (s *Server) handleCDN(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling CDN from %s (session=%d)", remoteAddr, header.SessionID)

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel != nil {
		session, ok := tunnel.GetSession(header.SessionID)
		if ok {
			clientIP := session.GetClientIP()
			log.Infof("Session %d disconnected, releasing IP %s", header.SessionID, clientIP)
			log.Debugf("Call disconnect: session=%d, tunnel=%d, client_ip=%s",
				header.SessionID, tunnel.tunnelID, clientIP)
		}
		tunnel.RemoveSession(header.SessionID)
	}
}

// handleStopCCN handles Stop-Control-Connection-Notification
func (s *Server) handleStopCCN(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	log.Infof("Handling StopCCN from %s", remoteAddr)

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel != nil {
		log.Infof("Closing tunnel %d from %s", tunnel.tunnelID, remoteAddr)
		log.Debugf("Tunnel shutdown: tunnel_id=%d, remote_addr=%s, active_sessions=%d",
			tunnel.tunnelID, remoteAddr, len(tunnel.sessions))
	}

	s.removeTunnel(remoteAddr.String())
}

// handleHello handles Hello message
func (s *Server) handleHello(header *L2TPHeader, avps []AVP, remoteAddr *net.UDPAddr) {
	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel != nil {
		tunnel.UpdateActivity()
	}
}

// handleDataMessage processes a data message
func (s *Server) handleDataMessage(header *L2TPHeader, payload []byte, remoteAddr *net.UDPAddr) {
	if header.SessionID == 0 {
		return // Invalid session ID
	}

	tunnel := s.getTunnel(remoteAddr.String())
	if tunnel == nil {
		return
	}

	session, ok := tunnel.GetSession(header.SessionID)
	if !ok {
		return
	}

	session.UpdateActivity()
	tunnel.UpdateActivity()

	// Process PPP frame
	s.handlePPPFrame(session, payload)
}

// sendControlMessage sends a control message
func (s *Server) sendControlMessage(tunnel *Tunnel, sessionID uint16, messageType uint16, avps []AVP) error {
	// Create header
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  tunnel.peerTunnelID,
		SessionID: sessionID,
		Ns:        tunnel.GetNextNs(),
		Nr:        tunnel.GetNr(),
	}

	// Add message type AVP
	allAVPs := []AVP{CreateUint16AVP(AVPMessageType, messageType, true)}
	allAVPs = append(allAVPs, avps...)

	// Serialize AVPs
	var avpData []byte
	for _, avp := range allAVPs {
		avpData = append(avpData, avp.Serialize()...)
	}

	// Serialize header
	headerData := header.Serialize()

	// Calculate total length
	totalLength := uint16(len(headerData) + len(avpData))

	// Update length field in header (it's at offset 6-7 if Length bit is set)
	if header.Flags&FlagLength != 0 {
		// Length field is at offset 6
		binary.BigEndian.PutUint16(headerData[6:8], totalLength)
	}

	// Combine header and AVPs
	packet := append(headerData, avpData...)

	// Send packet
	_, err := s.conn.WriteToUDP(packet, tunnel.remoteAddr)
	if err != nil {
		return utils.Errorf("write to UDP failed: %v", err)
	}

	log.Debugf("Sent control message type %d to %s (ns=%d, nr=%d)",
		messageType, tunnel.remoteAddr, header.Ns, header.Nr)

	return nil
}

// sendSCCRP sends Start-Control-Connection-Reply
func (s *Server) sendSCCRP(tunnel *Tunnel) error {
	avps := []AVP{
		CreateUint16AVP(AVPProtocolVersion, 0x0100, true),
		CreateUint32AVP(AVPFramingCapabilities, 0x00000003, true), // Both sync and async
		CreateUint32AVP(AVPBearerCapabilities, 0x00000003, true),  // Both digital and analog
		CreateUint16AVP(AVPFirmwareRevision, 0x0001, false),
		CreateStringAVP(AVPHostName, s.hostname, true),
		CreateStringAVP(AVPVendorName, s.vendorName, true),
		CreateUint16AVP(AVPAssignedTunnelID, tunnel.tunnelID, true),
		CreateUint16AVP(AVPReceiveWindowSize, 4, true),
	}

	return s.sendControlMessage(tunnel, 0, SCCRP, avps)
}

// sendICRP sends Incoming-Call-Reply
func (s *Server) sendICRP(tunnel *Tunnel, session *Session) error {
	avps := []AVP{
		CreateUint16AVP(AVPAssignedSessionID, session.sessionID, true),
	}

	return s.sendControlMessage(tunnel, session.peerSessionID, ICRP, avps)
}

// allocateTunnelID allocates a new tunnel ID
func (s *Server) allocateTunnelID() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		id := s.nextTunnelID
		s.nextTunnelID++
		if s.nextTunnelID == 0 {
			s.nextTunnelID = 1
		}

		if _, exists := s.tunnelsByID[id]; !exists {
			return id
		}
	}
}

// allocateSessionID allocates a new session ID
func (s *Server) allocateSessionID() uint16 {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextSessionID
	s.nextSessionID++
	if s.nextSessionID == 0 {
		s.nextSessionID = 1
	}
	return id
}

// getTunnel retrieves a tunnel by address key
func (s *Server) getTunnel(addrKey string) *Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tunnels[addrKey]
}

// removeTunnel removes a tunnel
func (s *Server) removeTunnel(addrKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tunnel, ok := s.tunnels[addrKey]; ok {
		delete(s.tunnelsByID, tunnel.tunnelID)
		delete(s.tunnels, addrKey)
		tunnel.Close()
		log.Infof("Removed tunnel %d", tunnel.tunnelID)
	}
}

// cleanupLoop periodically cleans up idle tunnels
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupIdleTunnels()
		}
	}
}

// cleanupIdleTunnels removes idle tunnels
func (s *Server) cleanupIdleTunnels() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var toRemove []string
	for addrKey, tunnel := range s.tunnels {
		if tunnel.IsIdle(DefaultIdleTimeout) {
			toRemove = append(toRemove, addrKey)
		}
	}

	for _, addrKey := range toRemove {
		if tunnel, ok := s.tunnels[addrKey]; ok {
			delete(s.tunnelsByID, tunnel.tunnelID)
			delete(s.tunnels, addrKey)
			tunnel.Close()
			log.Infof("Cleaned up idle tunnel %d", tunnel.tunnelID)
		}
	}
}

// endpointReadLoop reads packets from the endpoint and sends them to clients
func (s *Server) endpointReadLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		pkt := s.endpoint.ReadContext(s.ctx)
		if pkt == nil {
			return
		}

		// Extract destination IP to find the session
		// This is a simplified implementation
		// In production, you'd need proper routing

		pkt.DecRef()
	}
}

// InjectPacketToStack injects a packet into the network stack
func (s *Server) InjectPacketToStack(data []byte) error {
	if s.endpoint == nil {
		return fmt.Errorf("endpoint not initialized")
	}

	// Create packet buffer
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(data),
	})
	defer pkt.DecRef()

	// Inject into stack
	s.endpoint.InjectInbound(header.IPv4ProtocolNumber, pkt)

	return nil
}

// IPPool manages IP address allocation
type IPPool struct {
	mu        sync.Mutex
	start     net.IP
	end       net.IP
	allocated map[string]net.IP // session key -> IP
	available []net.IP
}

// NewIPPool creates a new IP pool
func NewIPPool(start, end net.IP) *IPPool {
	pool := &IPPool{
		start:     start,
		end:       end,
		allocated: make(map[string]net.IP),
		available: make([]net.IP, 0),
	}

	// Generate available IPs
	if start != nil && end != nil {
		startInt := ipToInt(start)
		endInt := ipToInt(end)

		for i := startInt; i <= endInt; i++ {
			pool.available = append(pool.available, intToIP(i))
		}
	} else {
		// Default pool: 172.16.0.2 - 172.16.0.254
		for i := 2; i < 255; i++ {
			pool.available = append(pool.available, net.IPv4(172, 16, 0, byte(i)))
		}
	}

	return pool
}

// Allocate allocates an IP address for a session
func (p *IPPool) Allocate(sessionKey string) (net.IP, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.available) == 0 {
		return nil, fmt.Errorf("no available IPs")
	}

	// Random selection
	idx := rand.Intn(len(p.available))
	ip := p.available[idx]

	// Remove from available
	p.available = append(p.available[:idx], p.available[idx+1:]...)

	// Add to allocated
	p.allocated[sessionKey] = ip

	return ip, nil
}

// Release releases an IP address
func (p *IPPool) Release(sessionKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ip, ok := p.allocated[sessionKey]; ok {
		delete(p.allocated, sessionKey)
		p.available = append(p.available, ip)
	}
}

func ipToInt(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func intToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}
