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
	if len(packet) == 0 {
		t.Fatal("assembleSynPacket returned empty packet")
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
