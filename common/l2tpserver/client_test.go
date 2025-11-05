package l2tpserver

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// TestL2TPClient tests the L2TP client connection to a server
func TestL2TPClient(t *testing.T) {
	// Start server
	server, err := NewL2TPServer(
		WithListenAddr("127.0.0.1:0"),
		WithHostname("test-server"),
		WithAuthFunc(func(username, password string) bool {
			return username == "testuser" && password == "testpass"
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	serverAddr := server.GetAddr()
	log.Infof("Server running on %s", serverAddr)

	time.Sleep(100 * time.Millisecond)

	// Create client and connect
	client, err := NewL2TPClient(
		serverAddr,
		WithUsername("testuser"),
		WithPassword("testpass"),
		WithClientTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	log.Infof("Client connected successfully")
	log.Infof("Tunnel ID: %d, Peer Tunnel ID: %d", client.GetTunnelID(), client.GetPeerTunnelID())
	log.Infof("Session ID: %d, Peer Session ID: %d", client.GetSessionID(), client.GetPeerSessionID())

	// Verify connection
	if client.GetPeerTunnelID() == 0 {
		t.Fatal("Peer tunnel ID should not be 0")
	}
	if client.GetPeerSessionID() == 0 {
		t.Fatal("Peer session ID should not be 0")
	}

	log.Info("L2TP client test completed successfully")
}

// TestL2TPClientPacketExchange tests sending and receiving packets
func TestL2TPClientPacketExchange(t *testing.T) {
	// Start server
	server, err := NewL2TPServer(
		WithListenAddr("127.0.0.1:0"),
		WithHostname("test-server"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	serverAddr := server.GetAddr()
	time.Sleep(100 * time.Millisecond)

	// Create client with packet callback
	clientReceivedPackets := make(chan []byte, 10)
	client, err := NewL2TPClient(
		serverAddr,
		WithUsername("user"),
		WithPassword("pass"),
		WithClientTimeout(5*time.Second),
		WithOnPacket(func(data []byte) {
			log.Infof("Client received packet: %d bytes", len(data))
			packetCopy := make([]byte, len(data))
			copy(packetCopy, data)
			clientReceivedPackets <- packetCopy
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	log.Info("Client connected, testing packet injection")

	// Give some time for session to be fully established
	time.Sleep(200 * time.Millisecond)

	// Test sending a packet from client to server
	testPacket := []byte{
		0x45, 0x00, 0x00, 0x54, // IP header
		0x00, 0x00, 0x40, 0x00,
		0x40, 0x01, 0x00, 0x00,
		0xC0, 0xA8, 0x01, 0x0A, // Source IP: 192.168.1.10
		0xC0, 0xA8, 0x01, 0x01, // Dest IP: 192.168.1.1
		// ... ICMP echo request data
		0x08, 0x00, 0xF7, 0xFF,
		0x00, 0x00, 0x00, 0x00,
	}

	err = client.InjectPacket(testPacket)
	if err != nil {
		t.Fatalf("Failed to inject packet: %v", err)
	}

	log.Info("Packet sent successfully")

	// Wait a bit to ensure packet processing
	time.Sleep(500 * time.Millisecond)

	log.Info("L2TP client packet exchange test completed")
}

// TestL2TPClientMultiple tests multiple concurrent clients
func TestL2TPClientMultiple(t *testing.T) {
	// Start server
	server, err := NewL2TPServer(
		WithListenAddr("127.0.0.1:0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	serverAddr := server.GetAddr()
	time.Sleep(100 * time.Millisecond)

	// Create multiple clients
	numClients := 3
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		client, err := NewL2TPClient(
			serverAddr,
			WithUsername("user"),
			WithPassword("pass"),
			WithClientTimeout(5*time.Second),
		)
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}
		clients[i] = client
		log.Infof("Client %d connected: tunnel=%d, session=%d",
			i, client.GetTunnelID(), client.GetSessionID())
	}

	// Cleanup
	for i, client := range clients {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client %d: %v", i, err)
		}
	}

	log.Infof("All %d clients connected and closed successfully", numClients)
}
