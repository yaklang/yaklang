package pcapx

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"testing"
)

func TestWithICMP_Sequence(t *testing.T) {
	var packets, err = PacketBuilder(
		WithIPv4_DstIP("1.1.1.1"),
		WithIPv4_SrcIP("1.1.1.2"),
		WithICMP_Type(layers.ICMPv4TypeEchoRequest, nil),
		WithPayload([]byte("hello yakit pcapx world")),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		panic(packet.ErrorLayer().Error())
	}
	fmt.Println(packet.String())
	if ret := packet.Layer(layers.LayerTypeICMPv4); ret == nil {
		t.Fatal("expect ipv4 icmp layer, not found ")
	}
}
