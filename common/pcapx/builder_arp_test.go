package pcapx

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/yaklang/yaklang/common/log"
	"testing"
)

func TestSmoking_Arp(t *testing.T) {
	packets, err := PacketBuilder(
		WithArp_RequestAuto("8.8.8.8"),
	)
	if err != nil {
		panic(err)
	}
	packet := gopacket.NewPacket(packets, layers.LayerTypeEthernet, gopacket.Default)
	if packet.ErrorLayer() != nil {
		log.Infof("error layer: %v", packet.ErrorLayer().Error())
		panic(packet.ErrorLayer().Error())
	}
	if arp := packet.Layer(layers.LayerTypeARP); arp.LayerType() != layers.LayerTypeARP {
		panic("build arp error")
	}
	fmt.Println(packet.String())
}
