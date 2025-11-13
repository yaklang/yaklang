package l2tpserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/header"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/network/ipv4"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/icmp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/tcp"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/transport/udp"
)

// TestL2TPServerBasic tests basic server creation and start
func TestL2TPServerBasic(t *testing.T) {
	server, err := NewL2TPServer(
		WithListenAddr(":0"), // Random port
		WithHostname("test-server"),
		WithVendorName("Test"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	defer server.Stop()

	// Server should be running
	if server.conn == nil {
		t.Fatal("Server connection is nil")
	}

	log.Infof("Server started on %s", server.conn.LocalAddr())
}

// TestL2TPServerWithNetStack tests server with network stack integration
func TestL2TPServerWithNetStack(t *testing.T) {
	// Create a test network stack
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

	// Create a channel endpoint
	ep := channel.New(256, 1500, "")

	// Create NIC
	nicID := tcpip.NICID(1)
	if err := s.CreateNIC(nicID, ep); err != nil {
		t.Fatalf("CreateNIC failed: %v", err)
	}

	// Add address
	addr := tcpip.AddrFrom4([4]byte{172, 16, 0, 1})
	protocolAddr := tcpip.ProtocolAddress{
		Protocol: header.IPv4ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   addr,
			PrefixLen: 24,
		},
	}
	if err := s.AddProtocolAddress(nicID, protocolAddr, stack.AddressProperties{}); err != nil {
		t.Fatalf("AddProtocolAddress failed: %v", err)
	}

	// Create L2TP server with stack
	server, err := NewL2TPServer(
		WithListenAddr(":0"),
		WithHostname("test-server-with-stack"),
		WithNetStack(s, nicID),
		WithIPPool(net.IPv4(172, 16, 0, 10), net.IPv4(172, 16, 0, 20)),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	defer server.Stop()

	log.Infof("Server with network stack started on %s", server.conn.LocalAddr())

	// Let it run for a moment
	time.Sleep(1 * time.Second)
}

// TestL2TPProtocol tests L2TP protocol parsing
func TestL2TPProtocol(t *testing.T) {
	// Test control message header
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  1,
		SessionID: 0,
		Ns:        0,
		Nr:        0,
	}

	data := header.Serialize()
	if len(data) < 6 {
		t.Fatal("Serialized header too short")
	}

	// Parse it back
	parsed, offset, err := ParseL2TPHeader(data)
	if err != nil {
		t.Fatalf("Failed to parse header: %v", err)
	}

	if !parsed.IsControl() {
		t.Error("Header should be control message")
	}

	if parsed.TunnelID != 1 {
		t.Errorf("Expected tunnel ID 1, got %d", parsed.TunnelID)
	}

	log.Infof("Parsed header: offset=%d, tunnel=%d, session=%d", offset, parsed.TunnelID, parsed.SessionID)
}

// TestAVPParsing tests AVP parsing and serialization
func TestAVPParsing(t *testing.T) {
	// Create AVP
	avp := CreateUint16AVP(AVPMessageType, SCCRQ, true)

	data := avp.Serialize()
	if len(data) == 0 {
		t.Fatal("Serialized AVP is empty")
	}

	// Parse it back
	parsed, size, err := ParseAVP(data)
	if err != nil {
		t.Fatalf("Failed to parse AVP: %v", err)
	}

	if size != len(data) {
		t.Errorf("Size mismatch: expected %d, got %d", len(data), size)
	}

	value, err := parsed.GetUint16()
	if err != nil {
		t.Fatalf("Failed to get uint16 value: %v", err)
	}

	if value != SCCRQ {
		t.Errorf("Expected value %d, got %d", SCCRQ, value)
	}

	log.Infof("Parsed AVP: type=%d, value=%d, mandatory=%v", parsed.Type, value, parsed.Mandatory)
}

// TestIPPool tests IP address pool allocation
func TestIPPool(t *testing.T) {
	pool := NewIPPool(net.IPv4(192, 168, 1, 10), net.IPv4(192, 168, 1, 20))

	// Allocate IP
	ip1, err := pool.Allocate("session1")
	if err != nil {
		t.Fatalf("Failed to allocate IP: %v", err)
	}

	log.Infof("Allocated IP: %s", ip1)

	// Allocate another
	ip2, err := pool.Allocate("session2")
	if err != nil {
		t.Fatalf("Failed to allocate second IP: %v", err)
	}

	if ip1.Equal(ip2) {
		t.Error("Two allocated IPs should be different")
	}

	// Release first IP
	pool.Release("session1")

	// Allocate again should work
	ip3, err := pool.Allocate("session3")
	if err != nil {
		t.Fatalf("Failed to allocate after release: %v", err)
	}

	log.Infof("Allocated IPs: %s, %s, %s", ip1, ip2, ip3)
}

// TestTunnelSession tests tunnel and session management
func TestTunnelSession(t *testing.T) {
	ctx := context.Background()
	remoteAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1234")

	tunnel := NewTunnel(1, 100, remoteAddr, ctx)

	if tunnel.tunnelID != 1 {
		t.Errorf("Expected tunnel ID 1, got %d", tunnel.tunnelID)
	}

	if tunnel.peerTunnelID != 100 {
		t.Errorf("Expected peer tunnel ID 100, got %d", tunnel.peerTunnelID)
	}

	// Create session
	session := NewSession(10, 200, tunnel)
	tunnel.AddSession(session)

	// Retrieve session
	retrieved, ok := tunnel.GetSession(10)
	if !ok {
		t.Fatal("Failed to retrieve session")
	}

	if retrieved.sessionID != 10 {
		t.Errorf("Expected session ID 10, got %d", retrieved.sessionID)
	}

	if retrieved.peerSessionID != 200 {
		t.Errorf("Expected peer session ID 200, got %d", retrieved.peerSessionID)
	}

	// Remove session
	tunnel.RemoveSession(10)

	_, ok = tunnel.GetSession(10)
	if ok {
		t.Error("Session should have been removed")
	}

	log.Info("Tunnel and session management test passed")
}

// TestAuthFunction tests custom authentication
func TestAuthFunction(t *testing.T) {
	authCalled := false
	var capturedUsername, capturedPassword string

	server, err := NewL2TPServer(
		WithListenAddr(":0"),
		WithAuthFunc(func(username, password string) bool {
			authCalled = true
			capturedUsername = username
			capturedPassword = password
			return username == "testuser" && password == "testpass"
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Manually call auth function to test it
	result := server.authFunc("testuser", "testpass")
	if !result {
		t.Error("Authentication should succeed for correct credentials")
	}

	if !authCalled {
		t.Error("Auth function was not called")
	}

	if capturedUsername != "testuser" || capturedPassword != "testpass" {
		t.Errorf("Captured credentials don't match: %s/%s", capturedUsername, capturedPassword)
	}

	// Test wrong credentials
	result = server.authFunc("wronguser", "wrongpass")
	if result {
		t.Error("Authentication should fail for incorrect credentials")
	}

	log.Info("Authentication function test passed")
}
