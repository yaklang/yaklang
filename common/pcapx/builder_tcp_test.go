package pcapx

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"testing"
)

func TestWithTCP_Ack(t *testing.T) {
	var packets, err = PacketBuilder(
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
		if ret.(*layers.TCP).ACK && string(ret.(*layers.TCP).Options[0].OptionData) == "\x04\x57" {
			t.Log("success")
		}
	}
}
