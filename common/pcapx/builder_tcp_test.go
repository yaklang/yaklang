package pcapx

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"testing"
)

func TestSmoking_TCP(t *testing.T) {
	var packets, err = PacketBuilder(
		// Packet serialization tests use explicit link addresses, without host routing or ARP.
		WithEthernet_SrcMac("02:00:00:00:00:01"),
		WithEthernet_DstMac("02:00:00:00:00:02"),
		WithIPv4_SrcIP("1.1.1.1"),
		WithIPv4_DstIP("1.1.1.2"),
		WithTCP_SrcPort(80),
		WithTCP_DstPort(80),
		WithTCP_Flags("ack"),
		WithTCP_OptionMSS(1111),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		panic(packet.ErrorLayer().Error())
	}
	fmt.Println(packet.String())
	if ret := packet.Layer(layers.LayerTypeTCP); ret == nil {
		t.Fatal("expect ipv4 tcp layer, not found ")
	} else {
		tcp := ret.(*layers.TCP)
		if !tcp.ACK || len(tcp.Options) == 0 || tcp.Options[0].OptionType != layers.TCPOptionKindMSS || string(tcp.Options[0].OptionData) != "\x04\x57" {
			t.Fatalf("TCP ACK/MSS mismatch: %+v", tcp)
		}
	}
}
