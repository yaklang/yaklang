package synscanx

import (
	"context"
	"net"
	"testing"
)

func TestAssembleSynPacket_PointToPointInterfaceDoesNotRequireARP(t *testing.T) {
	scanner := &Scannerx{
		ctx: context.Background(),
		config: &SynxConfig{
			Iface: &net.Interface{
				Name:  "utun6",
				Flags: net.FlagUp | net.FlagRunning | net.FlagPointToPoint,
			},
			SourceIP: net.ParseIP("10.10.16.42"),
		},
	}
	scanner.ifaceIPNetV4 = &net.IPNet{
		IP:   net.ParseIP("10.10.16.42").To4(),
		Mask: net.CIDRMask(23, 32),
	}
	scanner.ifaceUpdated = true

	packet, err := scanner.assembleSynPacket("10.129.220.92", 80)
	if err != nil {
		t.Fatalf("assembleSynPacket returned error: %v", err)
	}
	if len(packet) < 20 || packet[0]>>4 != 4 {
		t.Fatalf("point-to-point SYN must be a raw IPv4 packet, got %x", packet)
	}
}

func TestAssembleSynPacket_NoHardwareAddressIsRawIPv4(t *testing.T) {
	scanner := &Scannerx{
		ctx: context.Background(),
		config: &SynxConfig{
			Iface: &net.Interface{
				Name:  "tun0",
				Flags: net.FlagUp | net.FlagRunning,
			},
			SourceIP: net.ParseIP("172.18.0.1"),
		},
	}
	packet, err := scanner.assembleSynPacket("1.1.1.1", 443)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) < 20 || packet[0]>>4 != 4 {
		t.Fatalf("expected raw IPv4, got %x", packet)
	}
}

func TestAssembleArpPacket_PointToPointInterfaceUnsupported(t *testing.T) {
	scanner := &Scannerx{
		config: &SynxConfig{
			Iface: &net.Interface{
				Name:  "utun6",
				Flags: net.FlagUp | net.FlagRunning | net.FlagPointToPoint,
			},
			SourceIP: net.ParseIP("10.10.16.42"),
		},
	}

	if _, err := scanner.assembleArpPacket("10.129.220.92"); err == nil {
		t.Fatal("expected assembleArpPacket to reject point-to-point interfaces")
	}
}
