package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/yaklang/yaklang/common/l2tpserver"
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

func main() {
	// Example 1: Basic L2TP server without network stack integration
	if len(os.Args) > 1 && os.Args[1] == "basic" {
		runBasicServer()
		return
	}

	// Example 2: L2TP server with network stack integration
	runServerWithNetStack()
}

func runBasicServer() {
	log.Info("Starting basic L2TP server...")

	server, err := l2tpserver.StartL2TPServer(
		l2tpserver.WithListenAddr(":1701"),
		l2tpserver.WithHostname("yaklang-l2tp-server"),
		l2tpserver.WithVendorName("Yaklang"),
		l2tpserver.WithAuthFunc(func(username, password string) bool {
			log.Infof("Auth attempt: username=%s, password=%s", username, password)
			// Accept test/123456 for testing
			return username == "test" && password == "123456"
		}),
		l2tpserver.WithIPPool(
			net.IPv4(172, 16, 0, 10),
			net.IPv4(172, 16, 0, 100),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Stop()

	log.Info("Basic L2TP server started on :1701")
	log.Info("Test credentials: username=test, password=123456")
	log.Info("Client IP pool: 172.16.0.10 - 172.16.0.100")

	waitForSignal()
}

func runServerWithNetStack() {
	log.Info("Starting L2TP server with network stack integration...")

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

	// Create channel endpoint
	ep := channel.New(512, 1500, "")
	nicID := tcpip.NICID(1)

	// Create NIC
	if err := s.CreateNIC(nicID, ep); err != nil {
		log.Fatalf("CreateNIC failed: %v", err)
	}

	// Configure IP address for the VPN gateway
	addr := tcpip.AddrFrom4([4]byte{172, 16, 0, 1})
	protocolAddr := tcpip.ProtocolAddress{
		Protocol: header.IPv4ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   addr,
			PrefixLen: 24,
		},
	}

	if err := s.AddProtocolAddress(nicID, protocolAddr, stack.AddressProperties{}); err != nil {
		log.Fatalf("AddProtocolAddress failed: %v", err)
	}

	// Set route table
	s.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         nicID,
		},
	})

	// Create L2TP server with network stack
	server, err := l2tpserver.StartL2TPServer(
		l2tpserver.WithListenAddr(":1701"),
		l2tpserver.WithHostname("yaklang-l2tp-netstack"),
		l2tpserver.WithVendorName("Yaklang"),
		l2tpserver.WithAuthFunc(func(username, password string) bool {
			log.Infof("Auth attempt: username=%s, password=%s", username, password)
			// Accept any credentials for demo
			return true
		}),
		l2tpserver.WithNetStack(s, nicID),
		l2tpserver.WithIPPool(
			net.IPv4(172, 16, 0, 10),
			net.IPv4(172, 16, 0, 100),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Stop()

	log.Info("L2TP server with network stack started on :1701")
	log.Info("Gateway IP: 172.16.0.1")
	log.Info("Client IP pool: 172.16.0.10 - 172.16.0.100")
	log.Info("All authentication attempts will be accepted")
	log.Info("")
	log.Info("To connect using L2TP client (example):")
	log.Info("  - Server: localhost or your server IP")
	log.Info("  - Username: any")
	log.Info("  - Password: any")

	waitForSignal()
}

func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down...")
}
