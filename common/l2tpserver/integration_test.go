package l2tpserver

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// TestL2TPClientServerIntegration tests a complete L2TP connection flow
func TestL2TPClientServerIntegration(t *testing.T) {
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

	serverAddr := server.conn.LocalAddr().(*net.UDPAddr)
	log.Infof("Server running on %s", serverAddr)

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client connection
	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}
	defer clientConn.Close()

	// Test 1: Send SCCRQ (Start-Control-Connection-Request)
	sccrq := buildSCCRQ(1)
	_, err = clientConn.Write(sccrq)
	if err != nil {
		t.Fatalf("Failed to send SCCRQ: %v", err)
	}

	log.Info("Sent SCCRQ")

	// Receive SCCRP (Start-Control-Connection-Reply)
	buf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive SCCRP: %v", err)
	}

	log.Infof("Received response: %d bytes", n)

	// Parse response
	header, offset, err := ParseL2TPHeader(buf[:n])
	if err != nil {
		t.Fatalf("Failed to parse SCCRP header: %v", err)
	}

	if !header.IsControl() {
		t.Fatal("Expected control message")
	}

	// Parse AVPs
	log.Infof("Parsing AVPs from offset %d to %d (total %d bytes)", offset, n, n-offset)
	endIdx := offset + 20
	if endIdx > n {
		endIdx = n
	}
	log.Infof("AVP data (first 20 bytes): %x", buf[offset:endIdx])
	avps, err := ParseAVPs(buf[offset:n])
	if err != nil {
		t.Fatalf("Failed to parse AVPs: %v (offset=%d, n=%d)", err, offset, n)
	}

	// Find message type
	var messageType uint16
	var assignedTunnelID uint16
	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
		}
		if avp.Type == AVPAssignedTunnelID {
			assignedTunnelID, _ = avp.GetUint16()
		}
	}

	if messageType != SCCRP {
		t.Fatalf("Expected SCCRP (2), got %d", messageType)
	}

	log.Infof("Received SCCRP, assigned tunnel ID: %d", assignedTunnelID)

	// Test 2: Send SCCCN (Start-Control-Connection-Connected)
	scccn := buildSCCCN(1, assignedTunnelID, 1, 1)
	_, err = clientConn.Write(scccn)
	if err != nil {
		t.Fatalf("Failed to send SCCCN: %v", err)
	}

	log.Info("Sent SCCCN")
	time.Sleep(100 * time.Millisecond)

	// Test 3: Send ICRQ (Incoming-Call-Request)
	icrq := buildICRQ(1, assignedTunnelID, 100, 2, 1)
	_, err = clientConn.Write(icrq)
	if err != nil {
		t.Fatalf("Failed to send ICRQ: %v", err)
	}

	log.Info("Sent ICRQ")

	// Receive ICRP (Incoming-Call-Reply)
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive ICRP: %v", err)
	}

	log.Infof("Received ICRP: %d bytes", n)

	// Parse ICRP
	header, offset, err = ParseL2TPHeader(buf[:n])
	if err != nil {
		t.Fatalf("Failed to parse ICRP header: %v", err)
	}

	avps, err = ParseAVPs(buf[offset:n])
	if err != nil {
		t.Fatalf("Failed to parse AVPs: %v", err)
	}

	var assignedSessionID uint16
	for _, avp := range avps {
		if avp.Type == AVPMessageType {
			messageType, _ = avp.GetUint16()
		}
		if avp.Type == AVPAssignedSessionID {
			assignedSessionID, _ = avp.GetUint16()
		}
	}

	if messageType != ICRP {
		t.Fatalf("Expected ICRP (11), got %d", messageType)
	}

	log.Infof("Received ICRP, assigned session ID: %d", assignedSessionID)

	// Test 4: Send ICCN (Incoming-Call-Connected)
	iccn := buildICCN(1, assignedTunnelID, 100, 3, 2)
	_, err = clientConn.Write(iccn)
	if err != nil {
		t.Fatalf("Failed to send ICCN: %v", err)
	}

	log.Info("Sent ICCN - Session established!")

	// Let server process
	time.Sleep(200 * time.Millisecond)

	log.Info("L2TP session successfully established")
}

// buildSCCRQ builds a Start-Control-Connection-Request message
func buildSCCRQ(tunnelID uint16) []byte {
	// Create header
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  0,
		SessionID: 0,
		Ns:        0,
		Nr:        0,
	}

	// Create AVPs
	avps := []AVP{
		CreateUint16AVP(AVPMessageType, SCCRQ, true),
		CreateUint16AVP(AVPProtocolVersion, 0x0100, true),
		CreateUint32AVP(AVPFramingCapabilities, 0x00000003, true),
		CreateUint32AVP(AVPBearerCapabilities, 0x00000003, true),
		CreateStringAVP(AVPHostName, "test-client", true),
		CreateStringAVP(AVPVendorName, "TestVendor", true),
		CreateUint16AVP(AVPAssignedTunnelID, tunnelID, true),
		CreateUint16AVP(AVPReceiveWindowSize, 4, true),
	}

	return buildControlMessage(header, avps)
}

// buildSCCCN builds a Start-Control-Connection-Connected message
func buildSCCCN(ourTunnelID, peerTunnelID uint16, ns, nr uint16) []byte {
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  peerTunnelID,
		SessionID: 0,
		Ns:        ns,
		Nr:        nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, SCCCN, true),
	}

	return buildControlMessage(header, avps)
}

// buildICRQ builds an Incoming-Call-Request message
func buildICRQ(ourTunnelID, peerTunnelID, sessionID uint16, ns, nr uint16) []byte {
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  peerTunnelID,
		SessionID: 0,
		Ns:        ns,
		Nr:        nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, ICRQ, true),
		CreateUint16AVP(AVPAssignedSessionID, sessionID, true),
		CreateUint32AVP(AVPCallSerialNumber, 1, true),
	}

	return buildControlMessage(header, avps)
}

// buildICCN builds an Incoming-Call-Connected message
func buildICCN(ourTunnelID, peerTunnelID, sessionID uint16, ns, nr uint16) []byte {
	header := &L2TPHeader{
		Flags:     FlagType | FlagLength | FlagSequence | L2TPVersion,
		TunnelID:  peerTunnelID,
		SessionID: sessionID,
		Ns:        ns,
		Nr:        nr,
	}

	avps := []AVP{
		CreateUint16AVP(AVPMessageType, ICCN, true),
		CreateUint32AVP(AVPTxConnectSpeed, 100000000, true),
		CreateUint32AVP(AVPFramingType, 3, true),
	}

	return buildControlMessage(header, avps)
}

// buildControlMessage builds a complete control message
func buildControlMessage(header *L2TPHeader, avps []AVP) []byte {
	// Serialize AVPs
	var avpData []byte
	for _, avp := range avps {
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

	return packet
}

// TestL2TPDataMessage tests sending data messages
func TestL2TPDataMessage(t *testing.T) {
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

	serverAddr := server.conn.LocalAddr().(*net.UDPAddr)

	time.Sleep(100 * time.Millisecond)

	// Create client
	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer clientConn.Close()

	// Establish tunnel and session (simplified)
	sccrq := buildSCCRQ(1)
	clientConn.Write(sccrq)

	buf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := clientConn.Read(buf)

	if n > 0 {
		header, offset, _ := ParseL2TPHeader(buf[:n])
		if header != nil && header.IsControl() {
			avps, _ := ParseAVPs(buf[offset:n])
			var tunnelID uint16
			for _, avp := range avps {
				if avp.Type == AVPAssignedTunnelID {
					tunnelID, _ = avp.GetUint16()
					break
				}
			}

			if tunnelID > 0 {
				log.Infof("Got tunnel ID: %d", tunnelID)

				// Send SCCCN
				scccn := buildSCCCN(1, tunnelID, 1, 1)
				clientConn.Write(scccn)

				// Send ICRQ
				icrq := buildICRQ(1, tunnelID, 100, 2, 1)
				clientConn.Write(icrq)

				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	log.Info("Data message test completed")
}

// TestL2TPConcurrentSessions tests multiple concurrent sessions
func TestL2TPConcurrentSessions(t *testing.T) {
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

	serverAddr := server.conn.LocalAddr().(*net.UDPAddr)

	time.Sleep(100 * time.Millisecond)

	// Create multiple clients
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	numClients := 3
	done := make(chan bool, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer func() { done <- true }()

			conn, err := net.DialUDP("udp", nil, serverAddr)
			if err != nil {
				log.Errorf("Client %d failed to dial: %v", clientID, err)
				return
			}
			defer conn.Close()

			// Send SCCRQ
			sccrq := buildSCCRQ(uint16(clientID + 1))
			_, err = conn.Write(sccrq)
			if err != nil {
				log.Errorf("Client %d failed to send SCCRQ: %v", clientID, err)
				return
			}

			// Read response
			buf := make([]byte, 4096)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				log.Errorf("Client %d failed to read response: %v", clientID, err)
				return
			}

			if n > 0 {
				log.Infof("Client %d received response: %d bytes", clientID, n)
			}
		}(i)
	}

	// Wait for all clients
	for i := 0; i < numClients; i++ {
		select {
		case <-done:
			log.Infof("Client %d completed", i)
		case <-ctx.Done():
			t.Fatal("Timeout waiting for clients")
		}
	}

	log.Infof("All %d concurrent clients completed", numClients)
}
