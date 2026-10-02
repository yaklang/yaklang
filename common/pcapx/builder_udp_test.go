package pcapx

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"testing"
)

func TestSmoking_UDP(t *testing.T) {
	var packets, err = PacketBuilder(
		// Packet serialization tests use explicit link addresses, without host routing or ARP.
		WithEthernet_SrcMac("02:00:00:00:00:01"),
		WithEthernet_DstMac("02:00:00:00:00:02"),
		WithIPv4_SrcIP("1.1.1.1"),
		WithIPv4_DstIP("1.1.1.2"),
		WithUDP_SrcPort(80),
		WithUDP_DstPort(80),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		panic(packet.ErrorLayer().Error())
	}
	fmt.Println(packet.String())
	if ret := packet.Layer(layers.LayerTypeUDP); ret == nil {
		t.Fatal("expect ipv4 udp layer, not found ")
	} else {
		udp := ret.(*layers.UDP)
		if udp.SrcPort != layers.UDPPort(80) || udp.DstPort != layers.UDPPort(80) {
			t.Fatalf("UDP port mismatch: %+v", udp)
		}
	}
}
