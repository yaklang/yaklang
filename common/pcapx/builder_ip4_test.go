package pcapx

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestSmoking_IP(t *testing.T) {
	packets, err := PacketBuilder(
		WithIPv4_SrcIP("1.1.1.1"),
		WithIPv4_DstIP("1.1.1.2"),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		log.Infof("error layer: %v", packet.ErrorLayer().Error())
		panic(packet.ErrorLayer().Error())
	}
	fmt.Println(packet.String())
	if packet.NetworkLayer().LayerType() != layers.LayerTypeIPv4 {
		t.Fatalf("expect ipv4 layer, got %v", packet.NetworkLayer().LayerType())
	}
}
