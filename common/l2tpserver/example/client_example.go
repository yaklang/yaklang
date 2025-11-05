package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/l2tpserver"
	"github.com/yaklang/yaklang/common/log"
)

func main() {
	var (
		serverAddr = flag.String("server", "127.0.0.1:1701", "L2TP server address")
		username   = flag.String("user", "test", "Username for authentication")
		password   = flag.String("pass", "test", "Password for authentication")
		timeout    = flag.Duration("timeout", 10*time.Second, "Connection timeout")
	)
	flag.Parse()

	fmt.Println("L2TP Client Example")
	fmt.Println("===================")
	fmt.Printf("Server: %s\n", *serverAddr)
	fmt.Printf("User: %s\n\n", *username)

	// Create and connect client
	client, err := l2tpserver.NewL2TPClient(
		*serverAddr,
		l2tpserver.WithUsername(*username),
		l2tpserver.WithPassword(*password),
		l2tpserver.WithClientTimeout(*timeout),
		l2tpserver.WithOnPacket(func(data []byte) {
			log.Infof("📦 Received packet from server: %d bytes", len(data))
			// Print first 32 bytes as hex
			if len(data) > 32 {
				log.Infof("   Data: %x...", data[:32])
			} else {
				log.Infof("   Data: %x", data)
			}
		}),
	)
	if err != nil {
		log.Fatalf("Failed to connect to L2TP server: %v", err)
	}
	defer client.Close()

	log.Info("✅ Successfully connected to L2TP server!")
	log.Infof("   Our Tunnel ID: %d", client.GetTunnelID())
	log.Infof("   Server Tunnel ID: %d", client.GetPeerTunnelID())
	log.Infof("   Our Session ID: %d", client.GetSessionID())
	log.Infof("   Server Session ID: %d", client.GetPeerSessionID())

	// Example: Send a test ICMP echo request packet
	log.Info("\n📤 Sending test ICMP echo request...")
	testPacket := buildICMPEchoRequest()
	err = client.InjectPacket(testPacket)
	if err != nil {
		log.Errorf("Failed to inject packet: %v", err)
	} else {
		log.Info("✅ Packet sent successfully")
	}

	// Wait for interrupt signal
	log.Info("\n⏳ Client running... Press Ctrl+C to exit")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("\n👋 Shutting down client...")
}

// buildICMPEchoRequest builds a simple ICMP echo request packet
func buildICMPEchoRequest() []byte {
	// IP Header (20 bytes) + ICMP Header (8 bytes) + Data (32 bytes)
	packet := make([]byte, 60)

	// IP Header
	packet[0] = 0x45                                                 // Version (4) + IHL (5)
	packet[1] = 0x00                                                 // DSCP + ECN
	packet[2] = 0x00                                                 // Total Length (high)
	packet[3] = 0x3C                                                 // Total Length (low) = 60
	packet[4] = 0x00                                                 // Identification (high)
	packet[5] = 0x00                                                 // Identification (low)
	packet[6] = 0x00                                                 // Flags + Fragment Offset (high)
	packet[7] = 0x00                                                 // Fragment Offset (low)
	packet[8] = 0x40                                                 // TTL = 64
	packet[9] = 0x01                                                 // Protocol = ICMP (1)
	packet[10] = 0x00                                                // Header Checksum (high) - will be calculated
	packet[11] = 0x00                                                // Header Checksum (low)
	packet[12], packet[13], packet[14], packet[15] = 192, 168, 1, 10 // Source IP: 192.168.1.10
	packet[16], packet[17], packet[18], packet[19] = 8, 8, 8, 8      // Dest IP: 8.8.8.8

	// Calculate IP header checksum
	ipChecksum := calculateChecksum(packet[:20])
	packet[10] = byte(ipChecksum >> 8)
	packet[11] = byte(ipChecksum)

	// ICMP Header
	packet[20] = 0x08 // Type = Echo Request
	packet[21] = 0x00 // Code = 0
	packet[22] = 0x00 // Checksum (high) - will be calculated
	packet[23] = 0x00 // Checksum (low)
	packet[24] = 0x00 // Identifier (high)
	packet[25] = 0x01 // Identifier (low) = 1
	packet[26] = 0x00 // Sequence Number (high)
	packet[27] = 0x01 // Sequence Number (low) = 1

	// ICMP Data (32 bytes)
	for i := 28; i < 60; i++ {
		packet[i] = byte(i - 28)
	}

	// Calculate ICMP checksum
	icmpChecksum := calculateChecksum(packet[20:])
	packet[22] = byte(icmpChecksum >> 8)
	packet[23] = byte(icmpChecksum)

	return packet
}

// calculateChecksum calculates the IP/ICMP checksum
func calculateChecksum(data []byte) uint16 {
	sum := uint32(0)
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum > 0xFFFF {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}
