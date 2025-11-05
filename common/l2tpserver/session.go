package l2tpserver

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/vpnbrute/ppp"
)

// Tunnel represents an L2TP tunnel (control connection)
type Tunnel struct {
	mu           sync.RWMutex
	tunnelID     uint16 // Our tunnel ID
	peerTunnelID uint16 // Peer's tunnel ID
	ns           uint16 // Next sequence number to send
	nr           uint16 // Next expected sequence number
	remoteAddr   *net.UDPAddr
	sessions     map[uint16]*Session // Session ID -> Session
	lastActivity time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

// Session represents an L2TP session (call)
type Session struct {
	mu            sync.RWMutex
	sessionID     uint16 // Our session ID
	peerSessionID uint16 // Peer's session ID
	tunnel        *Tunnel
	pppAuth       *ppp.PPPAuth
	pppReady      chan struct{}
	authenticated bool
	clientIP      net.IP
	serverIP      net.IP
	lastActivity  time.Time
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewTunnel creates a new tunnel
func NewTunnel(tunnelID uint16, peerTunnelID uint16, remoteAddr *net.UDPAddr, ctx context.Context) *Tunnel {
	ctx, cancel := context.WithCancel(ctx)
	return &Tunnel{
		tunnelID:     tunnelID,
		peerTunnelID: peerTunnelID,
		ns:           0,
		nr:           0,
		remoteAddr:   remoteAddr,
		sessions:     make(map[uint16]*Session),
		lastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// NewSession creates a new session
func NewSession(sessionID uint16, peerSessionID uint16, tunnel *Tunnel) *Session {
	ctx, cancel := context.WithCancel(tunnel.ctx)
	return &Session{
		sessionID:     sessionID,
		peerSessionID: peerSessionID,
		tunnel:        tunnel,
		pppAuth:       ppp.GetDefaultPPPAuth(),
		pppReady:      make(chan struct{}),
		authenticated: false,
		lastActivity:  time.Now(),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// GetNextNs returns the next sequence number and increments it
func (t *Tunnel) GetNextNs() uint16 {
	t.mu.Lock()
	defer t.mu.Unlock()
	ns := t.ns
	t.ns++
	return ns
}

// GetNr returns the next expected sequence number
func (t *Tunnel) GetNr() uint16 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nr
}

// SetNr sets the next expected sequence number
func (t *Tunnel) SetNr(nr uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nr = nr
}

// ValidateSequence validates and updates sequence number
func (t *Tunnel) ValidateSequence(ns uint16) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Simple validation: accept if ns equals expected nr
	if ns == t.nr {
		t.nr++
		return true
	}

	// For now, we'll be lenient and accept any sequence
	// A production implementation should handle out-of-order packets
	log.Warnf("Sequence mismatch: expected %d, got %d", t.nr, ns)
	t.nr = ns + 1
	return true
}

// AddSession adds a session to the tunnel
func (t *Tunnel) AddSession(session *Session) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[session.sessionID] = session
	t.lastActivity = time.Now()
}

// GetSession retrieves a session by ID
func (t *Tunnel) GetSession(sessionID uint16) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	session, ok := t.sessions[sessionID]
	return session, ok
}

// RemoveSession removes a session from the tunnel
func (t *Tunnel) RemoveSession(sessionID uint16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if session, ok := t.sessions[sessionID]; ok {
		session.Close()
		delete(t.sessions, sessionID)
	}
}

// Close closes the tunnel and all its sessions
func (t *Tunnel) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, session := range t.sessions {
		session.Close()
	}
	t.sessions = make(map[uint16]*Session)
	t.cancel()
}

// UpdateActivity updates the last activity time
func (t *Tunnel) UpdateActivity() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActivity = time.Now()
}

// IsIdle checks if the tunnel has been idle for the given duration
func (t *Tunnel) IsIdle(timeout time.Duration) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Since(t.lastActivity) > timeout
}

// Close closes the session
func (s *Session) Close() {
	s.cancel()
}

// UpdateActivity updates the last activity time
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
}

// SetAuthenticated marks the session as authenticated
func (s *Session) SetAuthenticated(authenticated bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authenticated = authenticated
}

// IsAuthenticated returns whether the session is authenticated
func (s *Session) IsAuthenticated() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authenticated
}

// SetClientIP sets the client IP address
func (s *Session) SetClientIP(ip net.IP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientIP = ip
}

// GetClientIP gets the client IP address
func (s *Session) GetClientIP() net.IP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientIP
}
